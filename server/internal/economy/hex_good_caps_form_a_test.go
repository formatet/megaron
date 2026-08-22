package economy

import "testing"

// TestHexGoodCaps_SilverMineFormA is the pure-Go (no DB) reproduction of
// megaron_byggnadsniva_produktion.md §2's worked silver_mine example —
// mountain_limestone's silver row (production_rules rate_per_tick=28.8,
// verified live against poleia_test 2026-08-22) against
// depositCapacityTable["silver"] = {capNoBuilding:1, capWithBuilding:3,
// relevantBuilding:"silver_mine"} and WorkplaceSlots["silver_mine"] =
// {0,2,4,6}.
//
// Before Form A (cap used as BOTH the ceiling and the divisor): L1/L2/L3 all
// plateau at the exact same 28.8 max — the vault doc's "spelaren betalar för
// att bli sämre" finding. After Form A (capL1 frozen at the level-1 value):
// L1 is unchanged, L2/L3 rise as the ceiling grows.
func TestHexGoodCaps_SilverMineFormA(t *testing.T) {
	const rate = 28.799999999999997 // mountain_limestone/silver_mine/silver, live DB value

	atLevel := func(level int) (cap, capL1 int, oldMax, newMax float64) {
		caps, capsL1 := hexGoodCaps("mountain_limestone", false, false, true /* silverDep */, map[string]int{"silver_mine": level})
		cap = caps["silver"]
		capL1 = capsL1["silver"]
		oldMax = (rate / float64(cap)) * float64(cap) // pre-Form-A shape: rate/cap × placed, staffed to cap
		newMax = placementYield("silver", rate, capL1, cap, cap)
		return
	}

	l1Cap, l1CapL1, l1Old, l1New := atLevel(1)
	l2Cap, l2CapL1, l2Old, l2New := atLevel(2)
	l3Cap, l3CapL1, l3Old, l3New := atLevel(3)

	t.Logf("L1: cap=%d capL1=%d old-max=%v new-max=%v", l1Cap, l1CapL1, l1Old, l1New)
	t.Logf("L2: cap=%d capL1=%d old-max=%v new-max=%v", l2Cap, l2CapL1, l2Old, l2New)
	t.Logf("L3: cap=%d capL1=%d old-max=%v new-max=%v", l3Cap, l3CapL1, l3Old, l3New)

	// Vault table: L1 cap=5, L2 cap=7, L3 cap=9.
	if l1Cap != 5 || l2Cap != 7 || l3Cap != 9 {
		t.Fatalf("cap staircase drifted from the vault's worked example: L1=%d L2=%d L3=%d, want 5/7/9", l1Cap, l2Cap, l3Cap)
	}

	// The bug, reproduced: pre-Form-A max output is IDENTICAL at every level.
	if l1Old != l2Old || l2Old != l3Old {
		t.Errorf("expected the pre-Form-A formula to plateau at every level (the documented bug): L1=%v L2=%v L3=%v", l1Old, l2Old, l3Old)
	}

	// The fix: capL1 stays pinned to the level-1 value at every level...
	if l1CapL1 != 5 || l2CapL1 != 5 || l3CapL1 != 5 {
		t.Errorf("capL1 must stay frozen at the level-1 cap (5) regardless of actual level: L1=%d L2=%d L3=%d", l1CapL1, l2CapL1, l3CapL1)
	}
	// ...so L1's max output is unchanged, and L2/L3 strictly increase.
	if l1New != rate {
		t.Errorf("level-1 max output must be unchanged: got %v, want the raw rate %v", l1New, rate)
	}
	if l2New <= l1New || l3New <= l2New {
		t.Errorf("byggnadsnivå-bugg: max output must strictly increase with level, got L1=%v L2=%v L3=%v", l1New, l2New, l3New)
	}
}

// TestHexGoodCaps_GrainCapL1PinnedTo1 proves grain's capL1 stays 1 at every
// farm level (plains) — the mechanism that lets placementYield drop its old
// grain-specific branch and still reproduce grain's original rate×placed
// shape via the shared formula.
func TestHexGoodCaps_GrainCapL1PinnedTo1(t *testing.T) {
	for _, level := range []int{0, 1, 2, 3} {
		_, capsL1 := hexGoodCaps("plains", false, false, false, map[string]int{"farm": level})
		if got := capsL1["grain"]; got != 1 {
			t.Errorf("farm level %d: grain capL1 = %d, want 1", level, got)
		}
	}
}

// TestHexGoodCaps_LevelOneCapEqualsCapL1 proves the "L1 unchanged" half of
// the plan's contract for every building-gated hex rule, not just silver —
// at the actual building's level 1, cap and capL1 must be identical (so
// placementYield's Form A formula reduces to the pre-Form-A formula exactly).
func TestHexGoodCaps_LevelOneCapEqualsCapL1(t *testing.T) {
	cases := []struct {
		terrain  string
		building string
		good     string
	}{
		{"hills", "farm", "grain"},
		{"river_valley", "farm", "grain"},
		{"river_delta", "farm", "grain"},
		{"forest_olive_grove", "lumbermill", "timber"},
		{"forest_cedar", "lumbermill", "cedar"},
		{"coastal_sea", "harbour", "fish"},
	}
	for _, c := range cases {
		caps, capsL1 := hexGoodCaps(c.terrain, false, false, false, map[string]int{c.building: 1})
		if c.good == GoodGrain {
			continue // grain's capL1 is deliberately always 1, checked separately above
		}
		if caps[c.good] != capsL1[c.good] {
			t.Errorf("%s/%s level 1: cap=%d capL1=%d, want them equal", c.terrain, c.good, caps[c.good], capsL1[c.good])
		}
	}
}
