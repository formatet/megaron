package handlers

// Proof test for megaron_plan_tysta_forluster.md §Hål 2: Ticklog's flow
// derivation treated settlement_goods.rate as gross, so a settlement with a
// positive NET grain rate (the stored rate is already netted against the
// population's own consumption — province.go's Sitos-box comment, ~line 655)
// landed entirely in `production`, leaving `consumption["grain"]` absent.
// fmtFlows then printed "Kons: —" (cmd/keryx/cmd_ticklog.go) while the city's
// granary was actually draining relative to gross production — `consumption`
// can only ever hold net-negative goods, which is an underskott row, not a
// consumption row. Fixed by reusing the same honest cons=full-demand,
// prod=netto+demand reconstruction already established for the Sitos box.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ticklogGrainTestPool(t *testing.T) *pgxpool.Pool {
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

func TestTicklog_GrainConsumptionShownEvenWhenNetRateIsPositive(t *testing.T) {
	pool := ticklogGrainTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 0) RETURNING id`,
		"test-ticklog-grain-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"ticklog-grain-"+uuid.New().String(), "ticklog-grain-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}

	// Population 3000 → grain demand = 3000 * 0.5/tick = 1500/tick
	// (economy.GrainConsumptionPerCitizenPerTick). Plan's own worked example
	// ("producerar 2768 grain brutto och äter 1500" → rate=+1268): gross
	// production here is netGrainRate(1268) + demand(1500) = 2768.
	const population = 3000
	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Ticklogton', 'achaean', $3, 'capital', true, $4) RETURNING id`,
		worldID, provinceID, ownerID, population,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}
	const netGrainRate = 1268.0
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'grain', 50000, $2, 1000000, 0)`,
		settlementID, netGrainRate,
	); err != nil {
		t.Fatalf("seed grain settlement_goods row: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	ph := NewProvinceHandler(pool, nil, clk, economy.SitosConfig{}, nil, nil)

	r := chi.NewRouter()
	r.Get("/worlds/{worldID}/provinces/{provinceID}", ph.Get)
	r.Get("/worlds/{worldID}/provinces/{provinceID}/ticklog", ph.Ticklog)

	// keryx status's own net rate — the "two surfaces, one truth" half of the
	// plan's acceptance criterion.
	getReq := httptest.NewRequest(http.MethodGet,
		"/worlds/"+worldID.String()+"/provinces/"+provinceID.String(), nil)
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("Get = %d: %s", getRec.Code, getRec.Body.String())
	}
	var getResp struct {
		Settlement struct {
			Resources map[string]struct {
				Rate float64 `json:"rate"`
			} `json:"resources"`
		} `json:"settlement"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("parse Get response: %v", err)
	}
	statusNet := getResp.Settlement.Resources["grain"].Rate
	if statusNet != netGrainRate {
		t.Fatalf("status grain rate = %v, want %v (fixture didn't seed what the test assumes)", statusNet, netGrainRate)
	}

	tickReq := httptest.NewRequest(http.MethodGet,
		"/worlds/"+worldID.String()+"/provinces/"+provinceID.String()+"/ticklog?last=1", nil)
	tickRec := httptest.NewRecorder()
	r.ServeHTTP(tickRec, tickReq)
	if tickRec.Code != http.StatusOK {
		t.Fatalf("Ticklog = %d: %s", tickRec.Code, tickRec.Body.String())
	}
	var tickResp struct {
		Ticks []struct {
			Production  map[string]float64 `json:"production"`
			Consumption map[string]float64 `json:"consumption"`
		} `json:"ticks"`
	}
	if err := json.Unmarshal(tickRec.Body.Bytes(), &tickResp); err != nil {
		t.Fatalf("parse Ticklog response: %v", err)
	}
	if len(tickResp.Ticks) == 0 {
		t.Fatalf("ticklog returned no ticks")
	}
	row := tickResp.Ticks[0]

	cons, hasCons := row.Consumption["grain"]
	if !hasCons || cons <= 0 {
		t.Fatalf("ticklog Kons for grain = %v (present=%v), want > 0 — Hål 2's bug: a positive net rate must not hide consumption behind \"—\"", cons, hasCons)
	}
	prod, hasProd := row.Production["grain"]
	if !hasProd || prod <= 0 {
		t.Fatalf("ticklog Prod for grain = %v (present=%v), want > 0", prod, hasProd)
	}
	if got := prod - cons; got != netGrainRate {
		t.Errorf("Prod(%v) - Kons(%v) = %v, want %v (netto must stay exact)", prod, cons, got, netGrainRate)
	}
	if got := prod - cons; got != statusNet {
		t.Errorf("Prod - Kons (%v) != keryx status's own net rate (%v) — two surfaces must show the same truth", got, statusNet)
	}
}
