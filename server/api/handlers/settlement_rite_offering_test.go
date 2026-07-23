package handlers

// Regression test for Fas 1g: the Oracle rite must debit its material offering
// atomically regardless of outcome, and the RNG must be rolled exactly once in
// the handler (Fas 2.3 invariant: the event stores the outcome, never
// "roll_pending"). Reading Rite (api/handlers/settlement.go) shows the
// offering deduction happening unconditionally BEFORE the success roll, in
// the same transaction that commits regardless of outcome — this test
// verifies that structural claim end-to-end against a real DB rather than by
// re-reading the source, and that every response carries a discrete
// success:true/false rather than any pending/deferred state.
//
// Kharis seed updated for the 2026-07-09 0-100 rescale (FAS 0/1): kharis=50
// gives ~50% success (riteSuccessChance = kharis/100 + offerMod, offerMod=0 at
// the default offer_multiplier), the same "see both outcomes across repeated
// casts" property the old 150/2000 ("Suspicious" tier, 60%) seed exercised.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func riteTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestRiteOffering_DeductedRegardlessOfOutcome(t *testing.T) {
	pool := riteTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	username := "priest-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	// kharis=50 (0-100 scale) → riteSuccessChance ≈ 0.50 at the default
	// offer_multiplier (offerMod=0) — a mid-range chance that reliably produces
	// both outcomes across repeated casts, same role the old 150/2000 seed played.
	if _, err := pool.Exec(ctx,
		`INSERT INTO player_world_records (player_id, world_id, kharis_amount, kharis_rate)
		 VALUES ($1, $2, 50, 0)`,
		playerID, worldID,
	); err != nil {
		t.Fatalf("seed player_world_records: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	sh := NewSettlementHandler(pool, events.NewStore(pool), events.NewScheduler(pool, clk), clk)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/settlements/{settlementID}/rite", sh.Rite)

	const prayer = "akhaier_oracle_deposits" // Offering: oil 20, wine 10 (MinKharis 100)
	const oilStart = 100.0
	const wineStart = 100.0

	var successes, misses int
	// 15 independent casts (fresh settlement each time to dodge the per-temple
	// cooldown) at chance=60%: P(15/15 identical outcomes) ≈ 0.6^15+0.4^15 <
	// 0.05% — deducting on every single cast, regardless of which way any one
	// roll landed, is the actual invariant under test; observing both outcomes
	// is just corroborating evidence the roll isn't hardcoded.
	for i := 0; i < 15; i++ {
		var provinceID uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
			worldID, i,
		).Scan(&provinceID); err != nil {
			t.Fatalf("create province %d: %v", i, err)
		}
		var settlementID uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
			 VALUES ($1, $2, $3, 'akhaier', $4, 'capital', true) RETURNING id`,
			worldID, provinceID, "Oracleton"+uuid.New().String(), playerID,
		).Scan(&settlementID); err != nil {
			t.Fatalf("create settlement %d: %v", i, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'temple', 1)`,
			settlementID,
		); err != nil {
			t.Fatalf("create temple %d: %v", i, err)
		}
		for _, g := range []struct {
			key    string
			amount float64
		}{{"oil", oilStart}, {"wine", wineStart}} {
			if _, err := pool.Exec(ctx,
				`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
				 VALUES ($1, $2, $3, 0, 1000, 0)`,
				settlementID, g.key, g.amount,
			); err != nil {
				t.Fatalf("seed %s for settlement %d: %v", g.key, i, err)
			}
		}

		body, _ := json.Marshal(map[string]string{"prayer": prayer})
		req := httptest.NewRequest(http.MethodPost,
			"/worlds/"+worldID.String()+"/settlements/"+settlementID.String()+"/rite", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+accessToken)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("cast %d: Rite = %d: %s", i, rec.Code, rec.Body.String())
		}
		var resp struct {
			Success *bool `json:"success"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("cast %d: parse response: %v", i, err)
		}
		if resp.Success == nil {
			t.Fatalf("cast %d: response has no discrete success field — outcome not externalized (Fas 2.3)", i)
		}
		if *resp.Success {
			successes++
		} else {
			misses++
		}

		var oilLeft, wineLeft float64
		if err := pool.QueryRow(ctx,
			`SELECT settled(amount, rate, calc_tick) FROM settlement_goods WHERE settlement_id=$1 AND good_key='oil'`,
			settlementID,
		).Scan(&oilLeft); err != nil {
			t.Fatalf("cast %d: read oil: %v", i, err)
		}
		if err := pool.QueryRow(ctx,
			`SELECT settled(amount, rate, calc_tick) FROM settlement_goods WHERE settlement_id=$1 AND good_key='wine'`,
			settlementID,
		).Scan(&wineLeft); err != nil {
			t.Fatalf("cast %d: read wine: %v", i, err)
		}
		if oilLeft != oilStart-20 {
			t.Errorf("cast %d (success=%v): oil = %.1f, want %.1f (offering must be deducted regardless of outcome)",
				i, *resp.Success, oilLeft, oilStart-20)
		}
		if wineLeft != wineStart-10 {
			t.Errorf("cast %d (success=%v): wine = %.1f, want %.1f (offering must be deducted regardless of outcome)",
				i, *resp.Success, wineLeft, wineStart-10)
		}
	}

	if successes == 0 || misses == 0 {
		t.Logf("warning: only observed successes=%d misses=%d across 15 casts at chance=60%% — statistically unlucky but not itself a failure (offering deduction was still verified every time)", successes, misses)
	}
}
