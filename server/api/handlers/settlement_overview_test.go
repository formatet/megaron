package handlers

// Tests for SettlementsOverview (megaron_plan_oversiktsendpoint.md, 2026-09-06):
// GET /worlds/{worldID}/settlements/overview replaces the old N+1
// (/provinces/{id} once per owned settlement, from economy.js's
// loadEconomyGoods) with a single aggregate call.
//
//  1. TestSettlementsOverview_OnlyOwnSettlements: two settlements owned by the
//     caller and one owned by another Wanax → only the caller's two come back
//     (FOW/ownership, CLAUDE.md "Trade & messenger layer").
//  2. TestSettlementsOverviewParity_MatchesProvinceGet: the overview row for a
//     settlement carries EXACTLY the same food/granary fields as
//     ProvinceHandler.Get's "settlement" object for that same settlement — the
//     slice's invariant, because both read settlementFoodSummary.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func settlementsOverviewTestPool(t *testing.T) *pgxpool.Pool {
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

// settlementsOverviewTestWorld archives any leftover active test world (the
// one_active_world partial unique index — settled() needs current_world_tick())
// and creates a fresh active one.
func settlementsOverviewTestWorld(t *testing.T, pool *pgxpool.Pool, ctx context.Context) uuid.UUID {
	t.Helper()
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 1000) RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})
	return worldID
}

// settlementsOverviewSeedSettlement creates a province + settlement owned by
// ownerID, with a grain (and fish) settlement_goods row so grain_prod_rate /
// coverage aren't all-zero degenerate cases.
func settlementsOverviewSeedSettlement(t *testing.T, pool *pgxpool.Pool, ctx context.Context,
	worldID, ownerID uuid.UUID, name string, isCapital bool, mapQ, mapR int,
) uuid.UUID {
	t.Helper()
	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, 'plains') RETURNING id`,
		worldID, mapQ, mapR,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, $3, 'achaean', $4, 'capital', $5, 800) RETURNING id`,
		worldID, provinceID, name, ownerID, isCapital,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement %q: %v", name, err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'grain', 500, 12.5, 5000, 1000), ($1, 'fish', 100, 2.0, 2000, 1000)`,
		settlementID,
	); err != nil {
		t.Fatalf("seed settlement_goods for %q: %v", name, err)
	}
	return settlementID
}

func settlementsOverviewRegisterPlayer(t *testing.T, authSvc *auth.Service, ctx context.Context, username string) (uuid.UUID, string) {
	t.Helper()
	accessToken, _, err := authSvc.Register(ctx, username, "x")
	if err != nil {
		t.Fatalf("register player %q: %v", username, err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	return claims.PlayerID, accessToken
}

func TestSettlementsOverview_OnlyOwnSettlements(t *testing.T) {
	pool := settlementsOverviewTestPool(t)
	ctx := context.Background()
	worldID := settlementsOverviewTestWorld(t, pool, ctx)

	authSvc := auth.NewService(pool, "test-secret")
	ownerAID, tokenA := settlementsOverviewRegisterPlayer(t, authSvc, ctx, "wanax-a-"+uuid.New().String())
	ownerBID, _ := settlementsOverviewRegisterPlayer(t, authSvc, ctx, "wanax-b-"+uuid.New().String())

	petrasID := settlementsOverviewSeedSettlement(t, pool, ctx, worldID, ownerAID, "Petras", true, 0, 0)
	zakrosID := settlementsOverviewSeedSettlement(t, pool, ctx, worldID, ownerAID, "Zakros", false, 1, 0)
	_ = settlementsOverviewSeedSettlement(t, pool, ctx, worldID, ownerBID, "RivalCity", true, 2, 0)

	sitosCfg := economy.SitosConfig{
		SubsistenceGoods: []string{"grain", "fish"},
		LowDays:          3,
		HighDays:         10,
		GranaryCapDays:   14,
	}
	clk := clock.NewTestClock(time.Now())
	sh := NewSettlementHandler(pool, nil, nil, clk, sitosCfg)

	r := chi.NewRouter()
	r.With(auth.Middleware(authSvc)).Get("/worlds/{worldID}/settlements/overview", sh.SettlementsOverview)

	req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/settlements/overview", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("SettlementsOverview = %d: %s", rec.Code, rec.Body.String())
	}

	var result []struct {
		ID                 uuid.UUID `json:"id"`
		Name               string    `json:"name"`
		IsCapital          bool      `json:"is_capital"`
		Population         int       `json:"population"`
		GrainProdRate      float64   `json:"grain_prod_rate"`
		GrainConsumRate    float64   `json:"grain_consum_rate"`
		FoodSelfSufficient bool      `json:"food_self_sufficient"`
		Sitos              struct {
			CoverageTicks  float64 `json:"coverage_ticks"`
			LowTicks       float64 `json:"low_ticks"`
			HighTicks      float64 `json:"high_ticks"`
			GranaryTotal   float64 `json:"granary_total"`
			FoodNetPerTick float64 `json:"food_net_per_tick"`
		} `json:"sitos"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("parse response: %v (body: %s)", err, rec.Body.String())
	}

	if len(result) != 2 {
		t.Fatalf("got %d settlements, want 2 (only ownerA's) — body: %s", len(result), rec.Body.String())
	}
	seen := map[uuid.UUID]bool{}
	for _, row := range result {
		seen[row.ID] = true
		if row.Population != 800 {
			t.Errorf("settlement %s: population = %d, want 800", row.Name, row.Population)
		}
		if row.GrainProdRate != 12.5 {
			t.Errorf("settlement %s: grain_prod_rate = %.2f, want 12.5", row.Name, row.GrainProdRate)
		}
		if row.Sitos.LowTicks != 3 || row.Sitos.HighTicks != 10 {
			t.Errorf("settlement %s: sitos low/high = %.0f/%.0f, want 3/10", row.Name, row.Sitos.LowTicks, row.Sitos.HighTicks)
		}
	}
	if !seen[petrasID] {
		t.Errorf("Petras (%s) missing from result", petrasID)
	}
	if !seen[zakrosID] {
		t.Errorf("Zakros (%s) missing from result", zakrosID)
	}
	for _, row := range result {
		if row.Name == "RivalCity" {
			t.Errorf("another Wanax's settlement (RivalCity) leaked into the overview — FOW/ownership broken")
		}
	}
}

// TestSettlementsOverviewParity_MatchesProvinceGet is the slice's invariant
// (megaron_plan_oversiktsendpoint.md acceptance criterion 5): the overview
// row must carry EXACTLY what ProvinceHandler.Get computes for the same
// settlement, because both call settlementFoodSummary — never a second
// formula. Mutation-tested manually: temporarily hardcoding a field in
// SettlementsOverview (e.g. GrainProdRate: 0) made this test fail, confirming
// it actually exercises the shared path rather than passing vacuously.
func TestSettlementsOverviewParity_MatchesProvinceGet(t *testing.T) {
	pool := settlementsOverviewTestPool(t)
	ctx := context.Background()
	worldID := settlementsOverviewTestWorld(t, pool, ctx)

	authSvc := auth.NewService(pool, "test-secret")
	ownerID, token := settlementsOverviewRegisterPlayer(t, authSvc, ctx, "wanax-parity-"+uuid.New().String())
	settlementID := settlementsOverviewSeedSettlement(t, pool, ctx, worldID, ownerID, "Parityton", true, 0, 0)

	sitosCfg := economy.SitosConfig{
		SubsistenceGoods: []string{"grain", "fish"},
		LowDays:          3,
		HighDays:         10,
		GranaryCapDays:   14,
	}
	clk := clock.NewTestClock(time.Now())
	ph := NewProvinceHandler(pool, nil, clk, sitosCfg, nil, nil)
	sh := NewSettlementHandler(pool, nil, nil, clk, sitosCfg)

	r := chi.NewRouter()
	r.Get("/worlds/{worldID}/provinces/{provinceID}", ph.Get)
	r.With(auth.Middleware(authSvc)).Get("/worlds/{worldID}/settlements/overview", sh.SettlementsOverview)

	// Resolve the settlement's province ID for the /provinces/{id} call.
	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT province_id FROM settlements WHERE id = $1`, settlementID).Scan(&provinceID); err != nil {
		t.Fatalf("resolve province ID: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/provinces/"+provinceID.String(), nil)
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("provinces Get = %d: %s", getRec.Code, getRec.Body.String())
	}
	var getResp struct {
		Settlement struct {
			Population         int     `json:"population"`
			GrainProdRate      float64 `json:"grain_prod_rate"`
			GrainConsumRate    float64 `json:"grain_consum_rate"`
			FoodSelfSufficient bool    `json:"food_self_sufficient"`
			Sitos              struct {
				CoverageTicks  float64 `json:"coverage_ticks"`
				LowTicks       float64 `json:"low_ticks"`
				HighTicks      float64 `json:"high_ticks"`
				GranaryTotal   float64 `json:"granary_total"`
				FoodNetPerTick float64 `json:"food_net_per_tick"`
			} `json:"sitos"`
		} `json:"settlement"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("parse provinces Get response: %v", err)
	}

	ovReq := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/settlements/overview", nil)
	ovReq.Header.Set("Authorization", "Bearer "+token)
	ovRec := httptest.NewRecorder()
	r.ServeHTTP(ovRec, ovReq)
	if ovRec.Code != http.StatusOK {
		t.Fatalf("SettlementsOverview = %d: %s", ovRec.Code, ovRec.Body.String())
	}
	var ovResult []struct {
		ID                 uuid.UUID `json:"id"`
		Population         int       `json:"population"`
		GrainProdRate      float64   `json:"grain_prod_rate"`
		GrainConsumRate    float64   `json:"grain_consum_rate"`
		FoodSelfSufficient bool      `json:"food_self_sufficient"`
		Sitos              struct {
			CoverageTicks  float64 `json:"coverage_ticks"`
			LowTicks       float64 `json:"low_ticks"`
			HighTicks      float64 `json:"high_ticks"`
			GranaryTotal   float64 `json:"granary_total"`
			FoodNetPerTick float64 `json:"food_net_per_tick"`
		} `json:"sitos"`
	}
	if err := json.Unmarshal(ovRec.Body.Bytes(), &ovResult); err != nil {
		t.Fatalf("parse overview response: %v", err)
	}
	var ov *struct {
		ID                 uuid.UUID `json:"id"`
		Population         int       `json:"population"`
		GrainProdRate      float64   `json:"grain_prod_rate"`
		GrainConsumRate    float64   `json:"grain_consum_rate"`
		FoodSelfSufficient bool      `json:"food_self_sufficient"`
		Sitos              struct {
			CoverageTicks  float64 `json:"coverage_ticks"`
			LowTicks       float64 `json:"low_ticks"`
			HighTicks      float64 `json:"high_ticks"`
			GranaryTotal   float64 `json:"granary_total"`
			FoodNetPerTick float64 `json:"food_net_per_tick"`
		} `json:"sitos"`
	}
	for i := range ovResult {
		if ovResult[i].ID == settlementID {
			ov = &ovResult[i]
			break
		}
	}
	if ov == nil {
		t.Fatalf("settlement %s missing from overview response: %s", settlementID, ovRec.Body.String())
	}

	if ov.Population != getResp.Settlement.Population {
		t.Errorf("population parity broken: overview=%d province=%d", ov.Population, getResp.Settlement.Population)
	}
	if ov.GrainProdRate != getResp.Settlement.GrainProdRate {
		t.Errorf("grain_prod_rate parity broken: overview=%.4f province=%.4f", ov.GrainProdRate, getResp.Settlement.GrainProdRate)
	}
	if ov.GrainConsumRate != getResp.Settlement.GrainConsumRate {
		t.Errorf("grain_consum_rate parity broken: overview=%.4f province=%.4f", ov.GrainConsumRate, getResp.Settlement.GrainConsumRate)
	}
	if ov.FoodSelfSufficient != getResp.Settlement.FoodSelfSufficient {
		t.Errorf("food_self_sufficient parity broken: overview=%v province=%v", ov.FoodSelfSufficient, getResp.Settlement.FoodSelfSufficient)
	}
	if ov.Sitos.CoverageTicks != getResp.Settlement.Sitos.CoverageTicks {
		t.Errorf("sitos.coverage_ticks parity broken: overview=%.4f province=%.4f", ov.Sitos.CoverageTicks, getResp.Settlement.Sitos.CoverageTicks)
	}
	if ov.Sitos.LowTicks != getResp.Settlement.Sitos.LowTicks {
		t.Errorf("sitos.low_ticks parity broken: overview=%.4f province=%.4f", ov.Sitos.LowTicks, getResp.Settlement.Sitos.LowTicks)
	}
	if ov.Sitos.HighTicks != getResp.Settlement.Sitos.HighTicks {
		t.Errorf("sitos.high_ticks parity broken: overview=%.4f province=%.4f", ov.Sitos.HighTicks, getResp.Settlement.Sitos.HighTicks)
	}
	if ov.Sitos.GranaryTotal != getResp.Settlement.Sitos.GranaryTotal {
		t.Errorf("sitos.granary_total parity broken: overview=%.4f province=%.4f", ov.Sitos.GranaryTotal, getResp.Settlement.Sitos.GranaryTotal)
	}
	if ov.Sitos.FoodNetPerTick != getResp.Settlement.Sitos.FoodNetPerTick {
		t.Errorf("sitos.food_net_per_tick parity broken: overview=%.4f province=%.4f", ov.Sitos.FoodNetPerTick, getResp.Settlement.Sitos.FoodNetPerTick)
	}
}
