package handlers

// HTTP-level tests for S1c (megaron_plan_foda_konsistens.md §Steg "S1c"):
// player-initiated livestock slaughter for immediate population growth —
// the herd's strongest sink. DB integration tests (real Postgres, gated by
// DATABASE_URL, same pattern as settlement_placement_test.go).

import (
	"context"
	"net/http"
	"testing"

	"formatet/megaron/server/internal/economy"
)

// TestSlaughterLivestock_GainsPopulationAndConsumesOneAnimal is S1c's core
// rött-före test (megaron_plan_foda_konsistens.md §Rött-före): a settlement
// at 193 population with one livestock in stock can slaughter it and land
// on 203 population, the herd down by exactly one whole animal. 193→203
// crosses the 200 threshold, so the newly-born second gubbe must also be
// auto-placed through the same P4 hook population growth already uses
// (economy.PlaceNextGubbeOnBestFoodHex, kharis/tick.go applyDecay).
func TestSlaughterLivestock_GainsPopulationAndConsumesOneAnimal(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})
	pool := p10TestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE settlements SET population = 193 WHERE id = $1`, f.settlementID); err != nil {
		t.Fatalf("set population: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, $2, 1, 0, 10, 0)
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET amount = 1, rate = 0, cap = 10, calc_tick = 0`,
		f.settlementID, economy.GoodLivestock,
	); err != nil {
		t.Fatalf("seed livestock: %v", err)
	}

	code, resp := f.do(t, http.MethodPost, f.slaughterPath(), nil)
	if code != http.StatusOK {
		t.Fatalf("slaughter = %d: %v", code, resp)
	}
	if int(resp["population"].(float64)) != 203 {
		t.Errorf("response population = %v, want 203", resp["population"])
	}
	if int(resp["gubbar_placed"].(float64)) != 1 {
		t.Errorf("gubbar_placed = %v, want 1 (193->203 crosses the 200 threshold)", resp["gubbar_placed"])
	}

	var pop int
	if err := pool.QueryRow(ctx, `SELECT population FROM settlements WHERE id = $1`, f.settlementID).Scan(&pop); err != nil {
		t.Fatalf("read population: %v", err)
	}
	if pop != 203 {
		t.Errorf("db population = %d, want 203", pop)
	}

	var livestock float64
	if err := pool.QueryRow(ctx,
		`SELECT settled(amount, rate, calc_tick) FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
		f.settlementID, economy.GoodLivestock,
	).Scan(&livestock); err != nil {
		t.Fatalf("read livestock: %v", err)
	}
	if livestock != 0 {
		t.Errorf("livestock = %v, want 0 (herd down by exactly one whole animal)", livestock)
	}

	var placedRows int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM settlement_placement WHERE settlement_id = $1 AND gubbe_ordinal = 2`,
		f.settlementID,
	).Scan(&placedRows); err != nil {
		t.Fatalf("read placements: %v", err)
	}
	if placedRows != 1 {
		t.Errorf("expected the crossing gubbe (#2) to be auto-placed, found %d rows", placedRows)
	}
}

// TestSlaughterLivestock_RejectsWithoutLivestock: a settlement holding no
// livestock must be refused cleanly, not silently no-op or grant population
// anyway.
func TestSlaughterLivestock_RejectsWithoutLivestock(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})

	code, resp := f.do(t, http.MethodPost, f.slaughterPath(), nil)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("slaughter without livestock = %d: %v, want 422", code, resp)
	}
}
