package economy

// megaron_byggnadsniva_produktion.md, Form A (Timothy 2026-08-22): rate has no
// level term while cap grows with level via WorkplaceSlots — so an upgraded
// production building yields the SAME max output as level 1, for more gubbar
// and more silver/upkeep. This is the rött-före/grönt-efter proof for the fix:
// on unfixed code, TestRecomputeProduction_BuildingLevelIncreasesYield_HexGated
// FAILS (L3 == L1's max yield, the bug); after Form A it PASSES (L3 > L1).

import (
	"context"
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
		if _, err := pool.Exec(ctx,
			`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'lumbermill', $2)`,
			settlementID, level,
		); err != nil {
			t.Fatalf("seed lumbermill level %d: %v", level, err)
		}
		// hexCapacityRule{"cedar",1,2,"lumbermill"}.capWithBuilding=2, plus
		// this level's own WorkplaceSlots on top (capOf in placement_yield.go).
		cap = 2 + WorkplaceSlots("lumbermill", level)
		hex := hexgrid.Ring(hexgrid.Coord{Q: 0, R: 0}, hexgrid.CatchmentRadius)[0]
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

// TestRecomputeProduction_BuildingLevelIncreasesYield_BuildingGated is the
// BuildingOption sibling of the test above — a good with NO hex/terrain term
// at all, gated purely by a building's own WorkplaceSlots (P2): stone via
// stonequarry (production_rules: NULL terrain, building=stonequarry, rate=576,
// unchanged by level; WorkplaceSlots["stonequarry"] = {0,2,4,6}). Before Form
// A this surface had the SAME bug in its purest form: BuildingOption.CapPerGood
// WAS WorkplaceSlots(buildingType, level) with no capNoBuilding floor at all,
// so rate/cap × placed reduced to exactly rate at every level, trivially.
func TestRecomputeProduction_BuildingLevelIncreasesYield_BuildingGated(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 100

	rateAtLevel := func(level int) (rate float64, cap int) {
		settlementID := seedFullRingFixture(t, tick, 500, "plains")
		if _, err := pool.Exec(ctx,
			`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'stonequarry', $2)`,
			settlementID, level,
		); err != nil {
			t.Fatalf("seed stonequarry level %d: %v", level, err)
		}
		cap = WorkplaceSlots("stonequarry", level)
		for i := 0; i < cap; i++ {
			placeBuildingGubbe(t, pool, settlementID, i+1, "stonequarry", "stone")
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

	l1Rate, l1Cap := rateAtLevel(1)
	l3Rate, l3Cap := rateAtLevel(3)

	t.Logf("stonequarry L1: cap=%d fully-staffed stone rate=%v", l1Cap, l1Rate)
	t.Logf("stonequarry L3: cap=%d fully-staffed stone rate=%v", l3Cap, l3Rate)

	if l1Rate <= 0 {
		t.Fatalf("a fully-staffed level-1 stonequarry must produce SOME stone, got %v", l1Rate)
	}
	if l3Rate <= l1Rate {
		t.Errorf("byggnadsnivå-bugg (BuildingOption path): L3 stonequarry ska ge MER vid full bemanning än L1. "+
			"L1(cap=%d)=%v, L3(cap=%d)=%v", l1Cap, l1Rate, l3Cap, l3Rate)
	}
}
