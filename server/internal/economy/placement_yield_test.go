package economy

import "testing"

// TestPlacementYield_GrainPlateausAtCap is a pure-Go (no DB) proof of the
// plan's cap-safety requirement (megaron_plan_grain_cap.md §Säkerhet): grain's
// yield must plateau at cap × rate, exactly like every other good, once
// placed exceeds cap — Place() is supposed to make "placed > cap" unreachable
// in practice, but placementYield defends anyway (a stale read must never
// let yield keep climbing past what the hex can physically hold).
//
// Form A (megaron_byggnadsniva_produktion.md): grain's rate×placed shape is
// no longer a separate branch in placementYield — callers reproduce it by
// passing capL1=1 (hexGoodCaps pins grain's capL1 to 1 unconditionally), so
// rate/1×placed = rate×placed. This test passes capL1=1 directly to prove
// the formula still gives grain's exact old shape.
func TestPlacementYield_GrainPlateausAtCap(t *testing.T) {
	const rate, capL1, cap = 43.2, 1, 4

	// Below cap: grain's shape is rate × placed (not rate/cap × placed).
	if got, want := placementYield(GoodGrain, rate, capL1, cap, 1), rate*1; got != want {
		t.Errorf("placed=1: got %v, want %v", got, want)
	}
	if got, want := placementYield(GoodGrain, rate, capL1, cap, 4), rate*4; got != want {
		t.Errorf("placed=cap(4): got %v, want %v", got, want)
	}

	// At and beyond cap, yield must plateau — never exceed rate × cap.
	atCap := placementYield(GoodGrain, rate, capL1, cap, 4)
	beyondCap := placementYield(GoodGrain, rate, capL1, cap, 32) // Timothy's own "32 gubbar" figure
	if beyondCap != atCap {
		t.Errorf("placed=32 (beyond cap 4) yield = %v, want it clamped to the at-cap yield %v", beyondCap, atCap)
	}
	if beyondCap != rate*cap {
		t.Errorf("placed=32 yield = %v, want exactly rate×cap = %v", beyondCap, rate*cap)
	}
}

// TestPlacementYield_ZeroCapMeansZeroYield covers the cap<=0/capL1<=0 guard
// for every good, grain included — a good with no P3 hexCapacityRule entry
// for this terrain must yield 0, not divide by zero or fall through to an
// uncapped rate×placed formula.
func TestPlacementYield_ZeroCapMeansZeroYield(t *testing.T) {
	if got := placementYield(GoodGrain, 43.2, 1, 0, 5); got != 0 {
		t.Errorf("grain with cap=0 = %v, want 0", got)
	}
	if got := placementYield("fish", 10, 4, 0, 5); got != 0 {
		t.Errorf("fish with cap=0 = %v, want 0", got)
	}
	if got := placementYield("fish", 10, 0, 4, 5); got != 0 {
		t.Errorf("fish with capL1=0 = %v, want 0", got)
	}
}

// TestPlacementYield_NonGrainDividesByCapL1NotCap is the regression guard for
// every other good's Form A formula: rate/capL1 × min(placed,cap). At
// capL1==cap (building at level 1, or no level-scaling building at all) this
// is identical to the old rate/cap × placed shape; this test also proves the
// level-upgrade case (capL1 < cap) now yields MORE for the same rate, not
// the same — the exact bug megaron_byggnadsniva_produktion.md documents.
func TestPlacementYield_NonGrainDividesByCapL1NotCap(t *testing.T) {
	// capL1 == cap (unlevelled / level-1 case): unchanged from before.
	got := placementYield("fish", 40.0, 4, 4, 2)
	want := (40.0 / 4.0) * 2.0
	if got != want {
		t.Errorf("fish placed=2/capL1=cap=4: got %v, want %v", got, want)
	}
	// Beyond cap also plateaus for non-grain goods (pre-existing behaviour).
	if got := placementYield("fish", 40.0, 4, 4, 32); got != 40.0 {
		t.Errorf("fish placed=32/capL1=cap=4: got %v, want the at-cap yield 40.0", got)
	}

	// capL1 < cap (an upgraded building): more gubbar fit (cap), same rate
	// per gubbe (still divided by the FROZEN capL1) — max output must rise.
	const silverRate = 28.799999999999997 // live production_rules value
	l1Max := placementYield("silver", silverRate, 5, 5, 5) // level 1: capL1==cap==5
	l3Max := placementYield("silver", silverRate, 5, 9, 9) // level 3: capL1 frozen at 5, cap grown to 9
	if l3Max <= l1Max {
		t.Errorf("level-3 max yield (%v) must exceed level-1 max yield (%v) — Form A's whole point", l3Max, l1Max)
	}
	if want := silverRate; l1Max != want {
		t.Errorf("level-1 max yield = %v, want unchanged rate %v", l1Max, want)
	}
	if want := silverRate / 5 * 9; l3Max != want {
		t.Errorf("level-3 max yield = %v, want rate/capL1×cap = %v", l3Max, want)
	}
}
