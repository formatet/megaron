package economy

// Migration 129 (megaron_plan_sten_stock.md): stone's ONLY sink is one-time
// build costs (verified against internal/combat/upkeep.go and
// internal/economy/recipe.go — no upkeep, no recipe, no recurring drain).
// Before 129 a fully staffed level-1 stonequarry cleared the whole building
// catalogue in ~1.3 ticks, making stone allocation a non-decision. This file
// proves the behavioural bar in the plan's §6.1: 12-20 ticks, not 1-2.
//
// Rött-före (arbetssätt §3): at migration 128 (pre-129), stonequarry's rate
// was 576/tick, so 750/576 ≈ 1.3 ticks — OUTSIDE [12,20]. Verified by hand
// against poleia_test_ordning (still on 128) before writing 129:
//
//	$ docker exec thalassa-postgres-1 psql -U poleia -d poleia_test_ordning \
//	    -c "SELECT rate_per_tick FROM production_rules WHERE building_type='stonequarry' AND good_key='stone'"
//	 rate_per_tick
//	---------------
//	           576
//
// 750/576 = 1.302, confirming the test as written would fail against the
// unmigrated database — the bound is not vacuously true.

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/province"
)

// buildingCatalogueStoneCost sums BuildingSpecs' stone cost across the whole
// catalogue in code (not a literal 750), so the bound tracks the catalogue
// when a building is added or its cost changes. Deliberately excludes
// WallLevelSpecs — walls are a repeatable sink Timothy chose to leave out of
// the "clears the catalogue" framing (megaron_plan_sten_stock.md §1: "each
// building in the catalogue, once each").
func buildingCatalogueStoneCost() float64 {
	total := 0.0
	for _, spec := range province.BuildingSpecs {
		total += spec.Costs["stone"]
	}
	return total
}

// TestBuildingCatalogueStoneCost_MatchesPlannedFigure pins the ~750 figure
// the plan's derivation and migration 129's comment both cite, so a silent
// drift in BuildingSpecs is caught here rather than only showing up as a
// changed tick count in the test below.
func TestBuildingCatalogueStoneCost_MatchesPlannedFigure(t *testing.T) {
	got := buildingCatalogueStoneCost()
	if got != 750 {
		t.Fatalf("building catalogue stone cost = %v, want 750 (megaron_plan_sten_stock.md §1); "+
			"if a building's cost changed on purpose, update the plan's derivation too", got)
	}
}

// TestFullyStaffedStonequarry_ClearsBuildingCatalogueInPlannedWindow is the
// plan's §6.1 criterion 1: a fully staffed (capL1=2 gubbar) level-1
// stonequarry produces at production_rules.rate_per_tick directly (placed ==
// capL1 => placementYield == rate), so ticks-to-clear = catalogue / rate.
func TestFullyStaffedStonequarry_ClearsBuildingCatalogueInPlannedWindow(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	var rate float64
	if err := pool.QueryRow(ctx,
		`SELECT rate_per_tick FROM production_rules WHERE building_type = 'stonequarry' AND good_key = 'stone'`,
	).Scan(&rate); err != nil {
		t.Fatalf("read stonequarry stone rate_per_tick: %v", err)
	}
	if rate <= 0 {
		t.Fatalf("stonequarry stone rate_per_tick must be positive, got %v", rate)
	}

	catalogue := buildingCatalogueStoneCost()
	ticks := catalogue / rate

	const minTicks, maxTicks = 12.0, 20.0
	if ticks < minTicks || ticks > maxTicks {
		t.Errorf("a fully staffed level-1 stonequarry clears the %v-stone catalogue in %.2f ticks "+
			"(rate=%v/tick), want [%v,%v] per megaron_plan_sten_stock.md §6.1 criterion 1",
			catalogue, ticks, rate, minTicks, maxTicks)
	}
}

// TestStoneTerrainBaselines_Unchanged is criterion 2: migration 129 must not
// touch the bare-terrain baselines, exactly as 079 left them.
func TestStoneTerrainBaselines_Unchanged(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	cases := []struct {
		terrain string
		want    float64
	}{
		{"hills", 14.4},
		{"mountain_limestone", 28.8},
	}
	for _, c := range cases {
		var got float64
		if err := pool.QueryRow(ctx,
			`SELECT rate_per_tick FROM production_rules WHERE terrain_type = $1 AND good_key = 'stone' AND building_type IS NULL`,
			c.terrain,
		).Scan(&got); err != nil {
			t.Fatalf("read %s stone baseline: %v", c.terrain, err)
		}
		if diff := got - c.want; diff > 1e-6 || diff < -1e-6 {
			t.Errorf("%s stone baseline = %v, want %v (terrain baselines must survive migration 129 untouched)",
				c.terrain, got, c.want)
		}
	}
}
