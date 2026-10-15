package province

// Vägar V3 (megaron_plan_vagar.md §2): the war chariot's own terrain table.
// Expectations are hard-coded from the plan, not read back from the table.

import "testing"

func TestChariotTerrainTable(t *testing.T) {
	rows := []struct {
		terrain string
		chariot float64
		foot    float64
	}{
		{"plains", 0.67, 1},
		{"river_valley", 0.67, 1},
		{"river_delta", 0.67, 1},
		{"forest_olive_grove", 2, 1.5},
		{"scrub_maquis", 2, 1.5},
		{"hills", 3, 2},
		{"forest_cedar", 3, 2},
		{"semi_desert", 3, 3},
		{"river_ford", 2.5, 2.5}, // not in the plan's table: priced as foot
	}
	for _, r := range rows {
		if got := TerrainMoveTicksFor(r.terrain, CategoryWarChariot); !near(got, r.chariot) {
			t.Errorf("chariot on %s = %v, want %v", r.terrain, got, r.chariot)
		}
		if got := TerrainMoveTicksFor(r.terrain, "land"); !near(got, r.foot) {
			t.Errorf("foot on %s = %v, want %v (foot soldiers must be unchanged)", r.terrain, got, r.foot)
		}
		if got := moveHoursFor(r.terrain, CategoryWarChariot); !near(got, r.chariot) {
			t.Errorf("moveHoursFor chariot on %s = %v, want %v", r.terrain, got, r.chariot)
		}
	}
}

func TestChariotMountainsStayClosed(t *testing.T) {
	for _, m := range []string{"mountain_limestone", "mountain_red", "river", "coastal_sea", "deep_sea"} {
		if isPassable(m, CategoryWarChariot) {
			t.Errorf("%s must stay closed to chariots", m)
		}
	}
	tiles := map[[2]int]string{{0, 0}: "plains", {1, 0}: "mountain_red", {2, 0}: "plains"}
	if _, _, ok := findPath(tiles, MapPosition{}, MapPosition{Q: 2}, CategoryWarChariot); ok {
		t.Error("a chariot must not path through a mountain")
	}
}

// On a road the chariot takes a flat 0.4 per hex on every terrain (Timothy 2026-10-15).
func TestChariotRoadOnTopOfOwnTable(t *testing.T) {
	a, b := MapPosition{}, MapPosition{Q: 1}
	rs := roadsOf([2]MapPosition{a, b})
	if got := EdgeCost(a, b, "plains", "plains", CategoryWarChariot, rs); !near(got, 0.4) {
		t.Errorf("chariot on road = %v, want 0.4", got)
	}
	if got := EdgeCost(a, b, "hills", "hills", CategoryWarChariot, rs); !near(got, 0.4) {
		t.Errorf("chariot on hill road = %v, want 0.4", got)
	}
	if got := EdgeCost(a, b, "plains", "river_ford", CategoryWarChariot, rs); !near(got, 2.5) {
		t.Errorf("road never reaches a ford, got %v", got)
	}
}

// The chariot prefers a longer flat route to a short climb through the grove; the foot
// soldier takes the grove: 3 flat steps (2.01) beat 2 grove steps (2.67) for the
// chariot, while foot cost is 2.5 vs 3.
func TestChariotPrefersFlatDetour(t *testing.T) {
	tiles := map[[2]int]string{
		{0, 0}: "plains", {1, 0}: "forest_olive_grove", {2, 0}: "plains",
		{1, -1}: "plains", {2, -1}: "plains",
	}
	o, tg := MapPosition{}, MapPosition{Q: 2}
	fp, fc, _ := findPath(tiles, o, tg, "land")
	if len(fp) != 3 || !near(fc, 2.5) {
		t.Fatalf("foot: path %v cost %v, want the direct olive route at 2.5", fp, fc)
	}
	cp, cc, ok := findPath(tiles, o, tg, CategoryWarChariot)
	if !ok || len(cp) != 4 || !near(cc, 3*0.67) {
		t.Fatalf("chariot: path %v cost %v, want the 4-hex flat detour at %v", cp, cc, 3*0.67)
	}
}
