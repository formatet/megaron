package handlers

// Build-verb acceptance tests for megaron_plan_byggnad_pa_hex.md §A:
// farm/mine/lumbermill/stonequarry are hex-bound (province.HexBoundBuildings)
// — POST .../build now requires hex_q/hex_r for them, forbids it for city
// buildings, and validates the CHOSEN hex (catchment membership, terrain/
// deposit match) rather than the catchment in aggregate.
//
// Real Postgres, gated by DATABASE_URL — same harness as
// province_mine_catchment_test.go (recruitShipTestPool).

import (
	"context"
	"net/http"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/notify"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// buildHexFixture is setupMineGateFixture's sibling: a capital at (0,0) with
// ring hex (1,0) seeded plains (grain-capable) and ring hex (0,1) seeded
// deep_sea (not grain-capable, and outside the "coastal_sea" fish gate too) —
// enough to exercise farm's terrain/deposit gate without dragging in the
// mine-specific deposit machinery province_mine_catchment_test.go covers.
func buildHexFixture(t *testing.T) *mineGateFixture {
	t.Helper()
	pool := recruitShipTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-buildhex-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 100) RETURNING id`,
		"test-buildhex-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	accessToken, _, err := authSvc.Register(ctx, "buildhex-"+uuid.New().String(), "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	// Centre (not a production tile) + two ring hexes at distance 1: one
	// plains (grain-capable), one deep_sea (not).
	for _, tile := range []struct {
		q, r    int
		terrain string
	}{
		{0, 0, "plains"},
		{1, 0, "plains"},
		{0, 1, "deep_sea"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, tile.q, tile.r, tile.terrain,
		); err != nil {
			t.Fatalf("seed tile (%d,%d): %v", tile.q, tile.r, err)
		}
	}

	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Georgopolis', 'achaean', $3, 'capital', true, 500) RETURNING id`,
		worldID, provinceID, playerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}
	for good, amount := range map[string]float64{"timber": 200, "stone": 200, "cedar": 50} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
			 VALUES ($1, $2, $3, 0, $3, 0)`,
			settlementID, good, amount,
		); err != nil {
			t.Fatalf("seed %s: %v", good, err)
		}
	}
	if err := economy.RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("initial RecomputeProduction: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)
	hub := notify.New()
	hub.SetPool(pool)
	ph := NewProvinceHandler(pool, scheduler, clk, economy.SitosConfig{}, eventStore, hub)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/provinces/{provinceID}/build", ph.Build)

	return &mineGateFixture{pool: pool, worldID: worldID, provinceID: provinceID, settlementID: settlementID, accessToken: accessToken, router: r}
}

// TestBuildFarm_MissingHexRejected: a hex-bound type without hex_q/hex_r is a
// 400, not a 422 — the request itself is malformed, not merely invalid for
// this settlement.
func TestBuildFarm_MissingHexRejected(t *testing.T) {
	f := buildHexFixture(t)
	code, resp := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "farm"})
	if code != http.StatusBadRequest {
		t.Fatalf("build farm without hex_q/hex_r = %d: %v, want 400", code, resp)
	}
}

// TestBuildMarket_HexProvidedRejected: a CITY building must not accept a hex
// — the inverse of the farm case above.
func TestBuildMarket_HexProvidedRejected(t *testing.T) {
	f := buildHexFixture(t)
	code, resp := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "market", "hex_q": 1, "hex_r": 0})
	if code != http.StatusBadRequest {
		t.Fatalf("build market WITH hex_q/hex_r = %d: %v, want 400", code, resp)
	}
}

// TestBuildFarm_OutsideCatchmentRejected is acceptance criterion 4's "utanför
// catchment" case.
func TestBuildFarm_OutsideCatchmentRejected(t *testing.T) {
	f := buildHexFixture(t)
	far := hexgrid.Coord{Q: 100, R: 100}
	code, resp := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "farm", "hex_q": far.Q, "hex_r": far.R})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("build farm outside catchment = %d: %v, want 422", code, resp)
	}
	errMsg, _ := resp["error"].(string)
	if errMsg == "" {
		t.Errorf("expected an error message naming the catchment condition, got none")
	}
}

// TestBuildFarm_WrongTerrainRejected is acceptance criterion 4's "fel
// terräng" case: a catchment hex that exists but cannot grow grain
// (deep_sea) must be rejected, naming the terrain/deposit condition.
func TestBuildFarm_WrongTerrainRejected(t *testing.T) {
	f := buildHexFixture(t)
	code, resp := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "farm", "hex_q": 0, "hex_r": 1})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("build farm on deep_sea = %d: %v, want 422", code, resp)
	}
	errMsg, _ := resp["error"].(string)
	if errMsg == "" {
		t.Errorf("expected an error message naming the terrain/deposit condition, got none")
	}
}

// TestBuildFarm_OnValidPlainsHexSucceeds is the positive control for the
// three rejection tests above, and acceptance criterion 5's HTTP half: a
// plains ring hex within the catchment is a valid farm site.
func TestBuildFarm_OnValidPlainsHexSucceeds(t *testing.T) {
	f := buildHexFixture(t)
	code, resp := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "farm", "hex_q": 1, "hex_r": 0})
	if code != http.StatusCreated {
		t.Fatalf("build farm on valid plains ring hex = %d: %v, want 201", code, resp)
	}
}
