package economy

// megaron_byggnadsniva_produktion.md, Form A (Timothy 2026-08-22): rate has no
// level term while cap grows with level via WorkplaceSlots — so an upgraded
// production building yields the SAME max output as level 1, for more gubbar
// and more silver/upkeep. This is the rött-före/grönt-efter proof for the fix:
// on unfixed code, TestRecomputeProduction_BuildingLevelIncreasesYield_HexGated
// FAILS (L3 == L1's max yield, the bug); after Form A it PASSES (L3 > L1).

import (
	"context"
	"math"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
)

// TestRecomputeProduction_BuildingLevelIncreasesYield_HexGated exercises the
// real pipeline (LoadHexProductionOptions -> hexGoodCaps -> placementYield ->
// RecomputeProduction) for a HexOption-gated good: cedar via lumbermill on
// forest_cedar terrain. terrainCapacityTable["forest_cedar"] =
// {"cedar", capNoBuilding:1, capWithBuilding:2, "lumbermill"}; production_rules
// carries forest_cedar/NULL/cedar=72 (always) + forest_cedar/lumbermill/cedar=144
// (once built) = 216 rate_per_tick once the lumbermill exists, unchanged by
// level. Only cap grows with level (WorkplaceSlots["lumbermill"] = {0,2,4,6}):
//
//	L1 cap = 2+2=4   L3 cap = 2+6=8
//
// A settlement is built once per level, its ONE ring hex staffed to that
// level's own cap (full staffing = the ceiling the plan measures), and
// RecomputeProduction's resulting cedar rate read back.
func TestRecomputeProduction_BuildingLevelIncreasesYield_HexGated(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 100

	rateAtLevel := func(level int) (rate float64, cap int) {
		settlementID := seedFullRingFixture(t, tick, 500, "forest_cedar")
		hex := hexgrid.Ring(hexgrid.Coord{Q: 0, R: 0}, hexgrid.CatchmentRadius)[0]
		if _, err := pool.Exec(ctx,
			`INSERT INTO buildings (settlement_id, building_type, level, hex_q, hex_r) VALUES ($1, 'lumbermill', $2, $3, $4)`,
			settlementID, level, hex.Q, hex.R,
		); err != nil {
			t.Fatalf("seed lumbermill level %d: %v", level, err)
		}
		// hexCapacityRule{"cedar",1,2,"lumbermill"}.capWithBuilding=2, plus
		// this level's own WorkplaceSlots on top (capOf in placement_yield.go).
		// The lumbermill stands on THIS hex (megaron_plan_byggnad_pa_hex.md §A) —
		// staffing must target the same one it raised the cap on.
		cap = 1 + BuildingExtraPlaces
		for i := 0; i < cap; i++ {
			placeHexGubbe(t, pool, settlementID, i+1, hex, "cedar")
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		defer tx.Rollback(ctx)
		if err := RecomputeProduction(ctx, tx, settlementID); err != nil {
			t.Fatalf("RecomputeProduction level %d: %v", level, err)
		}
		if err := tx.QueryRow(ctx,
			`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'cedar'`,
			settlementID,
		).Scan(&rate); err != nil {
			t.Fatalf("read cedar rate level %d: %v", level, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
		return rate, cap
	}

	l1Rate, l1Cap := rateAtLevel(1)
	l3Rate, l3Cap := rateAtLevel(3)

	t.Logf("lumbermill L1: cap=%d fully-staffed cedar rate=%v", l1Cap, l1Rate)
	t.Logf("lumbermill L3: cap=%d fully-staffed cedar rate=%v", l3Cap, l3Rate)

	if l1Rate <= 0 {
		t.Fatalf("a fully-staffed level-1 lumbermill must produce SOME cedar, got %v", l1Rate)
	}
	if l3Rate <= l1Rate {
		t.Errorf("byggnadsnivå-bugg (megaron_byggnadsniva_produktion.md): en L3-byggnad ska ge MER "+
			"vid full bemanning än L1, inte samma. L1(cap=%d)=%v, L3(cap=%d)=%v",
			l1Cap, l1Rate, l3Cap, l3Rate)
	}
}

// TestRecomputeProduction_BuildingLevelIncreasesYield_Stonequarry: stone on a
// hills hex with a stonequarry standing on it (byggnadsregeln — the quarry works
// its hex, it has no workplace inside the building any more): full crew of
// 2+4 places, and the level raises the rate per worker.
func TestRecomputeProduction_BuildingLevelIncreasesYield_Stonequarry(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 100

	rateAtLevel := func(level int) (rate float64, cap int) {
		settlementID := seedFullRingFixture(t, tick, 500, "hills")
		hex := hexgrid.Ring(hexgrid.Coord{Q: 0, R: 0}, hexgrid.CatchmentRadius)[0]
		if _, err := pool.Exec(ctx,
			`INSERT INTO buildings (settlement_id, building_type, level, hex_q, hex_r) VALUES ($1, 'stonequarry', $2, $3, $4)`,
			settlementID, level, hex.Q, hex.R,
		); err != nil {
			t.Fatalf("seed stonequarry level %d: %v", level, err)
		}
		cap = 2 + BuildingExtraPlaces
		for i := 0; i < cap; i++ {
			placeHexGubbe(t, pool, settlementID, i+1, hex, "stone")
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		defer tx.Rollback(ctx)
		if err := RecomputeProduction(ctx, tx, settlementID); err != nil {
			t.Fatalf("RecomputeProduction level %d: %v", level, err)
		}
		if err := tx.QueryRow(ctx,
			`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'stone'`,
			settlementID,
		).Scan(&rate); err != nil {
			t.Fatalf("read stone rate level %d: %v", level, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
		return rate, cap
	}

	l1Rate, _ := rateAtLevel(1)
	l3Rate, _ := rateAtLevel(3)
	if want := 6 * 1.0 * 1.7; math.Abs(l1Rate-want) > 1e-6 {
		t.Errorf("stonequarry L1 on hills, full crew: stone rate %.4f, want %.4f (6 x 1.0 x 1.7)", l1Rate, want)
	}
	if want := 6 * 1.0 * 3.1; math.Abs(l3Rate-want) > 1e-6 {
		t.Errorf("stonequarry L3 on hills, full crew: stone rate %.4f, want %.4f (6 x 1.0 x 3.1)", l3Rate, want)
	}
}
