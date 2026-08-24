package economy

import (
	"math"
	"testing"
)

// TestHexGoodCaps_SilverMineFormB is the pure-Go (no DB) reproduction of
// megaron_plan_byggnadsniva_takt.md's worked silver_mine example, following
// on from Form A (megaron_byggnadsniva_produktion.md, formerly proven by
// TestHexGoodCaps_SilverMineFormA in this same file). Form A moved the
// level's effect into the CEILING (cap grows with level, mult didn't exist,
// headcount for max output grew 5/7/9). Form B moves it into the RATE
// instead (mult grows with level, headcount for max output stays 5/5/5).
//
// Framgångskriterium 1 (megaron_plan_byggnadsniva_takt.md §6): max output per
// level is UNCHANGED from Form A's own already-deployed, calibrated table —
// 28.8 / 40.3 / 51.8.
// Framgångskriterium 2: the headcount needed to reach that max is capL1 (5)
// at EVERY level, not the level-grown cap (5/7/9).
func TestHexGoodCaps_SilverMineFormB(t *testing.T) {
	const rate = 28.799999999999997 // mountain_limestone/silver_mine/silver, live DB value

	atLevel := func(level int) (cap, capL1 int, mult float64, maxAtCapL1, maxOverstaffed float64) {
		caps, capsL1, m, placeCap := hexGoodCaps("mountain_limestone", false, false, true /* silverDep */, map[string]int{"silver_mine": level})
		cap = caps["silver"]
		capL1 = capsL1["silver"]
		mult = m["silver"]
		maxAtCapL1 = placementYield("silver", rate, capL1, placeCap["silver"], mult, capL1)
		maxOverstaffed = placementYield("silver", rate, capL1, placeCap["silver"], mult, cap) // beyond capL1, must plateau — see below
		return
	}

	l1Cap, l1CapL1, l1Mult, l1Max, l1MaxOver := atLevel(1)
	l2Cap, l2CapL1, l2Mult, l2Max, l2MaxOver := atLevel(2)
	l3Cap, l3CapL1, l3Mult, l3Max, l3MaxOver := atLevel(3)

	t.Logf("L1: cap=%d capL1=%d mult=%v max=%v", l1Cap, l1CapL1, l1Mult, l1Max)
	t.Logf("L2: cap=%d capL1=%d mult=%v max=%v", l2Cap, l2CapL1, l2Mult, l2Max)
	t.Logf("L3: cap=%d capL1=%d mult=%v max=%v", l3Cap, l3CapL1, l3Mult, l3Max)

	// The physical ceiling (cap) is unchanged from P3/Form A — still shown as
	// the hex's theoretical worker capacity, even though headcount no longer
	// needs to reach it.
	if l1Cap != 5 || l2Cap != 7 || l3Cap != 9 {
		t.Fatalf("cap staircase drifted from the vault's worked example: L1=%d L2=%d L3=%d, want 5/7/9", l1Cap, l2Cap, l3Cap)
	}

	// Framgångskriterium 2: capL1 (the REAL number of gubbar that must be
	// placed to reach max output) is pinned to 5 at every level.
	if l1CapL1 != 5 || l2CapL1 != 5 || l3CapL1 != 5 {
		t.Errorf("capL1 must stay frozen at the level-1 cap (5) regardless of actual level: L1=%d L2=%d L3=%d", l1CapL1, l2CapL1, l3CapL1)
	}

	// Framgångskriterium 1: max output matches Form A's already-deployed,
	// calibrated table exactly (28.8/40.3/51.8) — now reached with only
	// capL1 gubbar, not cap.
	wantL1, wantL2, wantL3 := rate, rate/5*7, rate/5*9
	// Toleransjämför, aldrig !=. (rate/capL1)*mult*placed och rate/capL1*cap är
	// algebraiskt samma tal men INTE bitidentiska i float64 — de multiplicerar i
	// olika ordning, så L2 landar på 40.32 mot 40.31999999999999. Ett == här
	// mäter avrundningsordning, inte den ekonomiska invarianten.
	closeEnough := func(got, want float64) bool { return math.Abs(got-want) < 1e-9 }
	if !closeEnough(l1Max, wantL1) {
		t.Errorf("L1 max output = %v, want %v (unchanged)", l1Max, wantL1)
	}
	if !closeEnough(l2Max, wantL2) {
		t.Errorf("L2 max output = %v, want %v (Form A's own table value, 40.3)", l2Max, wantL2)
	}
	if !closeEnough(l3Max, wantL3) {
		t.Errorf("L3 max output = %v, want %v (Form A's own table value, 51.8)", l3Max, wantL3)
	}
	if l2Max <= l1Max || l3Max <= l2Max {
		t.Errorf("byggnadsnivå-bugg: max output must still strictly increase with level, got L1=%v L2=%v L3=%v", l1Max, l2Max, l3Max)
	}

	// Placing MORE than capL1 gubbar (up to the old, level-grown cap) must
	// plateau at the exact same max — placementYield's defensive clip.
	if l1MaxOver != l1Max || l2MaxOver != l2Max || l3MaxOver != l3Max {
		t.Errorf("over-staffing beyond capL1 must plateau at the same max: L1 %v/%v L2 %v/%v L3 %v/%v",
			l1MaxOver, l1Max, l2MaxOver, l2Max, l3MaxOver, l3Max)
	}
}

// TestHexGoodCaps_GrainCapL1PinnedTo1 proves grain's capL1 stays 1 at every
// farm level (plains) — the mechanism that lets placementYield reproduce
// grain's original rate×placed shape (via mult=1, placeCap=the real cap) —
// see TestHexGoodCaps_MultAndPlaceCapPinnedForGrain below for the two other
// legs of the same invariant.
func TestHexGoodCaps_GrainCapL1PinnedTo1(t *testing.T) {
	for _, level := range []int{0, 1, 2, 3} {
		_, capsL1, _, _ := hexGoodCaps("plains", false, false, false, map[string]int{"farm": level})
		if got := capsL1["grain"]; got != 1 {
			t.Errorf("farm level %d: grain capL1 = %d, want 1", level, got)
		}
	}
}

// TestHexGoodCaps_MultAndPlaceCapPinnedForGrain is the plan's own required
// rött-före-worthy proof (megaron_plan_byggnadsniva_takt.md §4): grain's
// multiplier axis must be closed, and its placement ceiling must stay the
// REAL level-actual cap, not the frozen capL1=1 fiction.
//
// This is NOT a constant-compared-to-itself test: at farm level 3 the NAIVE
// ratio cap/capL1 is 12/1 = 12.0, a real, non-trivial number — proving
// hexGoodCaps' explicit override actually does something. Likewise
// placeCap["grain"] must track the real, level-grown cap (4/8/10/12 —
// megaron_plan_grain_cap.md), not collapse to capL1=1 the way every other
// good's placeCap now does.
func TestHexGoodCaps_MultAndPlaceCapPinnedForGrain(t *testing.T) {
	wantCap := map[int]int{0: 4, 1: 8, 2: 10, 3: 12} // megaron_plan_grain_cap.md's locked staircase
	for _, level := range []int{0, 1, 2, 3} {
		caps, _, mult, placeCap := hexGoodCaps("plains", false, false, false, map[string]int{"farm": level})
		if got := caps["grain"]; got != wantCap[level] {
			t.Fatalf("farm level %d: real grain cap = %d, want %d (grain-cap plan drifted, not this slice's business to fix)", level, got, wantCap[level])
		}
		if got := mult["grain"]; got != 1.0 {
			t.Errorf("farm level %d: grain mult = %v, want 1.0 (naive cap/capL1 would be %v — the multiplier axis must be closed for grain)",
				level, got, float64(wantCap[level])/1.0)
		}
		if got := placeCap["grain"]; got != wantCap[level] {
			t.Errorf("farm level %d: grain placeCap = %d, want %d (the REAL cap, not capL1=1)", level, got, wantCap[level])
		}
	}
}

// TestHexGoodCaps_LevelOneCapEqualsCapL1 proves the "L1 unchanged" half of
// the plan's contract for every building-gated hex rule, not just silver —
// at the actual building's level 1, cap and capL1 must be identical (so
// mult=1.0 and placementYield's formula reduces to the pre-Form-A formula
// exactly).
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
		caps, capsL1, mult, _ := hexGoodCaps(c.terrain, false, false, false, map[string]int{c.building: 1})
		if c.good == GoodGrain {
			continue // grain's capL1 is deliberately always 1, checked separately above
		}
		if caps[c.good] != capsL1[c.good] {
			t.Errorf("%s/%s level 1: cap=%d capL1=%d, want them equal", c.terrain, c.good, caps[c.good], capsL1[c.good])
		}
		if got := mult[c.good]; got != 1.0 {
			t.Errorf("%s/%s level 1: mult=%v, want 1.0", c.terrain, c.good, got)
		}
	}
}
