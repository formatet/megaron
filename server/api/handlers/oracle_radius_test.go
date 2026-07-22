package handlers

// Regression test for P1b (soak 2026-07-18): the oracle rite
// (applyOracleRevealDeposits, settlement.go) only searched within oracleRadius
// hexes of the casting settlement. At the old radius (8) a tin deposit sitting
// 15 hexes away — entirely plausible on a sparse, mostly-sea map — was
// permanently undiscoverable, since the oracle is the ONLY discovery surface
// for deposits outside a settlement's own 7-hex catchment.
//
// This test seeds a single tin-bearing site 15 hexes from the caster's
// settlement (nothing closer exists in the DB at all) and casts the oracle
// prayer with kharis=100 (riteSuccessChance = 100%, so success is
// deterministic — no repeated-cast loop needed). It also casts from a
// NON-capital settlement to verify the soak's "only the capital can cast"
// claim is stale: Rite gates on ownership, not is_capital.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
)

func TestOracleRadius_RevealsTinSiteFifteenHexesAwayFromNonCapitalSettlement(t *testing.T) {
	pool := riteTestPool(t) // helper from settlement_rite_offering_test.go
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
	username := "oracle-radius-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	// kharis=100 -> riteSuccessChance = 100% (kharis/100 + offerMod(0)) so the
	// cast succeeds deterministically; the thing under test is the radius, not
	// the success roll.
	if _, err := pool.Exec(ctx,
		`INSERT INTO player_world_records (player_id, world_id, kharis_amount, kharis_rate)
		 VALUES ($1, $2, 100, 0)`,
		playerID, worldID,
	); err != nil {
		t.Fatalf("seed player_world_records: %v", err)
	}

	// Caster's own settlement at origin (0,0) — deliberately NOT the capital,
	// to verify a non-capital settlement can cast (the soak's "not your
	// settlement" claim was found stale in code review: Rite gates on
	// owner_id only).
	var originProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&originProvinceID); err != nil {
		t.Fatalf("create origin province: %v", err)
	}
	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Non-Capital Colony', 'akhaier', $3, 'colony', false) RETURNING id`,
		worldID, originProvinceID, playerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create non-capital settlement: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'temple', 1)`,
		settlementID,
	); err != nil {
		t.Fatalf("create temple: %v", err)
	}
	for _, g := range []struct {
		key    string
		amount float64
	}{{"oil", 100}, {"wine", 100}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
			 VALUES ($1, $2, $3, 0, 1000, 0)`,
			settlementID, g.key, g.amount,
		); err != nil {
			t.Fatalf("seed %s: %v", g.key, err)
		}
	}

	// Buildable site at (15,0) — hex-distance 15 from origin (0,0), i.e.
	// outside the old radius (8) but inside the widened one (20). Its
	// neighbour at (16,-1) carries the tin deposit; the site itself must be
	// colonisable terrain (not sea/impassable-mountain/semi_desert).
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain, copper_deposit, tin_deposit, silver_deposit)
		 VALUES ($1, 15, 0, 'hills', false, false, false)`,
		worldID,
	); err != nil {
		t.Fatalf("seed candidate site tile: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain, copper_deposit, tin_deposit, silver_deposit)
		 VALUES ($1, 16, -1, 'mountain_limestone', false, true, false)`,
		worldID,
	); err != nil {
		t.Fatalf("seed tin deposit tile: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	sh := NewSettlementHandler(pool, events.NewStore(pool), events.NewScheduler(pool, clk), clk)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/settlements/{settlementID}/rite", sh.Rite)

	body, _ := json.Marshal(map[string]string{"prayer": "akhaier_oracle_deposits"})
	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+worldID.String()+"/settlements/"+settlementID.String()+"/rite", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Rite = %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Success *bool `json:"success"`
		Effect  struct {
			Reveals []struct {
				Q   int    `json:"q"`
				R   int    `json:"r"`
				Ore string `json:"ore"`
			} `json:"reveals"`
		} `json:"effect"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp.Success == nil || !*resp.Success {
		t.Fatalf("cast should succeed deterministically at kharis=100, got success=%v message=%q",
			resp.Success, resp.Message)
	}
	if len(resp.Effect.Reveals) == 0 {
		t.Fatalf("expected the oracle to reveal the tin site at distance 15 (widened radius) — got no reveals, message=%q",
			resp.Message)
	}
	found := false
	for _, rv := range resp.Effect.Reveals {
		if rv.Q == 15 && rv.R == 0 && rv.Ore == "tin" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected reveal (q=15,r=0,ore=tin) among %+v — oracle radius must reach at least 15 hexes",
			resp.Effect.Reveals)
	}
}
