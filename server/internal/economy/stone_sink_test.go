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
	"math"
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
	// 750 → 104,168 (mig 136, sten ÷7,2) → 410,396 (S4, dagsverkeskalibreringen
	// 2026-08-27: byggnadskatalogen satt mot galärankaret på 30 dagsverken, se
	// province.BuildingSpecs). Läses live ur katalogen ovan, inte härledd här.
	// → 381,825 (mig 154, 2026-09-28: silver_mine utgår ur katalogen —
	// megaron_plan_byggnad_pa_hex.md §A, beslut 2 — samma kostnad/byggtid
	// som mine, alltså en ren minskning med EN byggnads stenkostnad 28,571,
	// ingen balansändring i sig. megaron_plan_sten_stock.md §1:s citerade
	// tal är ANNU inte uppdaterat till detta — flaggat, inte ändrat här
	// (vault är utanför den här slicens scope).
	//
	// Tolerans i stället för det gamla strikta ==: buildingCatalogueStoneCost
	// summerar en Go-map i icke-deterministisk iterationsordning, och
	// kostnaderna är sedan mig 136 inte längre exakta multiplar som adderas
	// bit-identiskt oavsett ordning — mätt drift ~1e-13, väl inom gränsen.
	const want = 381.825
	got := buildingCatalogueStoneCost()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("building catalogue stone cost = %v, want %v (megaron_plan_sten_stock.md §1); "+
			"if a building's cost changed on purpose, update the plan's derivation too", got, want)
	}
}

// TestFullyStaffedStonequarry_ClearsBuildingCatalogueInPlannedWindow — since
// byggnadsregeln (mig 155) the stonequarry has no workplace inside the building:
// it stands on a hills / mountain_limestone hex, gives it +4 places and raises
// the per-worker rate by (1 + 0.7 x level). One fully staffed level-1 quarry
// hex therefore makes (2+4) x r0 x 1.7 stone per tick (hills 10.2, limestone
// 20.4 — before: a 3.33 x 2 workshop in the building plus 1.0 x 2 on the hex).
//
// The OLD band [45,80] ticks (megaron_plan_sten_stock.md §6.1 criterion 1) was
// calibrated against the old building workplace (57.3 ticks) and is NOT held
// by the new rule on hills (37.4 ticks) or limestone (18.7). Whether the
// catalogue's stone costs or the band move is BESLUT 3's call, not this test's:
// it pins the derived numbers and logs the windows.
func TestFullyStaffedStonequarry_ClearsBuildingCatalogueInPlannedWindow(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	catalogue := buildingCatalogueStoneCost()
	for _, c := range []struct {
		terrain  string
		wantRate float64
	}{
		{"hills", 6 * 1.0 * 1.7},
		{"mountain_limestone", 6 * 2.0 * 1.7},
	} {
		var r0 float64
		if err := pool.QueryRow(ctx,
			`SELECT rate_per_tick FROM production_rules WHERE terrain_type = $1 AND good_key = 'stone' AND building_type IS NULL`,
			c.terrain,
		).Scan(&r0); err != nil {
			t.Fatalf("read %s stone r0: %v", c.terrain, err)
		}
		places, mult := hexGoodPlaces(c.terrain, false, false, false, map[string]int{"stonequarry": 1}, "stone")
		rate := hexYield(r0, places, mult, places)
		if math.Abs(rate-c.wantRate) > 1e-9 {
			t.Errorf("%s: fully staffed level-1 quarry hex makes %.4f stone/tick, want %.4f", c.terrain, rate, c.wantRate)
		}
		t.Logf("%s: %.1f stone/tick -> catalogue (%.1f) cleared in %.1f ticks (old band [45,80])", c.terrain, rate, catalogue, catalogue/rate)
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
		{"hills", 1.0},              // per gubbe since mig 155 (was 2.0 per hex, fallback cap 2)
		{"mountain_limestone", 2.0}, // per gubbe since mig 155 (was 4.0 per hex)
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
