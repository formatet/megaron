package economy

import "testing"

// TestPlacementYield_GrainPlateausAtCap is a pure-Go (no DB) proof of the
// plan's cap-safety requirement (megaron_plan_grain_cap.md §Säkerhet): grain's
// yield must plateau at cap × rate, exactly like every other good, once
// placed exceeds cap — Place() is supposed to make "placed > cap" unreachable
// in practice, but placementYield defends anyway (a stale read must never
// let yield keep climbing past what the hex can physically hold).
func TestPlacementYield_GrainPlateausAtCap(t *testing.T) {
	const rate, cap = 43.2, 4

	// Below cap: grain's shape is rate × placed (not rate/cap × placed).
	if got, want := placementYield(GoodGrain, rate, cap, 1), rate*1; got != want {
		t.Errorf("placed=1: got %v, want %v", got, want)
	}
	if got, want := placementYield(GoodGrain, rate, cap, 4), rate*4; got != want {
		t.Errorf("placed=cap(4): got %v, want %v", got, want)
	}

	// At and beyond cap, yield must plateau — never exceed rate × cap.
	atCap := placementYield(GoodGrain, rate, cap, 4)
	beyondCap := placementYield(GoodGrain, rate, cap, 32) // Timothy's own "32 gubbar" figure
	if beyondCap != atCap {
		t.Errorf("placed=32 (beyond cap 4) yield = %v, want it clamped to the at-cap yield %v", beyondCap, atCap)
	}
	if beyondCap != rate*cap {
		t.Errorf("placed=32 yield = %v, want exactly rate×cap = %v", beyondCap, rate*cap)
	}
}

// TestPlacementYield_ZeroCapMeansZeroYield covers the cap<=0 guard for every
// good, grain included — a good with no P3 hexCapacityRule entry for this
// terrain must yield 0, not divide by zero or fall through to an uncapped
// rate×placed formula.
func TestPlacementYield_ZeroCapMeansZeroYield(t *testing.T) {
	if got := placementYield(GoodGrain, 43.2, 0, 5); got != 0 {
		t.Errorf("grain with cap=0 = %v, want 0", got)
	}
	if got := placementYield("fish", 10, 0, 5); got != 0 {
		t.Errorf("fish with cap=0 = %v, want 0", got)
	}
}

// TestPlacementYield_NonGrainStillDividesByCap is the regression guard for
// every other good's unchanged formula (rate/cap × placed) — this slice only
// touches grain's exemption, not the shared clamp logic every other good
// already relied on.
func TestPlacementYield_NonGrainStillDividesByCap(t *testing.T) {
	got := placementYield("fish", 40.0, 4, 2)
	want := (40.0 / 4.0) * 2.0
	if got != want {
		t.Errorf("fish placed=2/cap=4: got %v, want %v", got, want)
	}
	// Beyond cap also plateaus for non-grain goods (pre-existing behaviour).
	if got := placementYield("fish", 40.0, 4, 32); got != 40.0 {
		t.Errorf("fish placed=32/cap=4: got %v, want the at-cap yield 40.0", got)
	}
}
