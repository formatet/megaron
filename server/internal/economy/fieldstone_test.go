package economy

import (
	"math"
	"testing"
)

// Fältsten (mig 156, megaron_plan_byggkostnader steg 0): every land terrain
// without quarry-grade stone gives half a standard rate, and hexRules agrees.
// hexRules names the rule, production_rules carries the rate — this pins that
// both halves exist; the rate is read from the real table (a rule without its
// row yields 0 and would pass a pure-Go check).
func TestFieldStone_EveryLandTerrainHasRuleAndRate(t *testing.T) {
	pool := openScaleTestPool(t)
	defer pool.Close()

	byKey := map[[3]string]float64{}
	for _, r := range loadProductionRules(t, pool) {
		byKey[[3]string{r.good, r.terrain, r.building}] = r.rate
	}
	for _, terrain := range []string{"plains", "river_valley", "river_delta", "forest_olive_grove",
		"forest_cedar", "scrub_maquis", "semi_desert", "mountain_red"} {
		rate := byKey[[3]string{"stone", terrain, ""}]
		if math.Abs(rate-0.5) > 1e-9 {
			t.Errorf("%s: fieldstone production_rules rate = %v, want 0.5 (mig 156)", terrain, rate)
		}
		places, mult := hexGoodPlaces(terrain, false, false, false, map[string]int{}, "stone")
		if places != 1 || mult != 1.0 {
			t.Errorf("%s: unbuilt stone = %d x %.1f, want 1 x 1.0", terrain, places, mult)
		}
		if !RuleBuildingSupportsHex("stonequarry", terrain, false, false, false) {
			t.Errorf("%s: stonequarry may not stand here", terrain)
		}
	}
	// Hills and limestone keep their own, higher rates (tolerance: mig 155 divided them).
	hills, lime := byKey[[3]string{"stone", "hills", ""}], byKey[[3]string{"stone", "mountain_limestone", ""}]
	if math.Abs(hills-1.0) > 1e-6 || math.Abs(lime-2.0) > 1e-6 {
		t.Errorf("hills/limestone stone rates = %v/%v, want 1/2", hills, lime)
	}
}
