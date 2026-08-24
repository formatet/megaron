package economy

import (
	"math"
	"testing"
)

// TestPlacementYield_GrainPlateausAtCap is a pure-Go (no DB) proof of the
// plan's cap-safety requirement (megaron_plan_grain_cap.md §Säkerhet): grain's
// yield must plateau at cap × rate, exactly like every other good, once
// placed exceeds cap — Place() is supposed to make "placed > cap" unreachable
// in practice, but placementYield defends anyway (a stale read must never
// let yield keep climbing past what the hex can physically hold).
//
// Form B (megaron_plan_byggnadsniva_takt.md): grain passes capL1=1 (the RATE
// denominator, unchanged from Form A) but placeCap=cap (the REAL,
// level-actual physical ceiling — Form B's grain exception, see
// placementYield's doc comment) and mult=1 (the multiplier axis stays closed
// for grain). rate/1 × 1 × placed = rate × placed reproduces grain's exact
// pre-Form-B shape.
func TestPlacementYield_GrainPlateausAtCap(t *testing.T) {
	const rate, capL1, cap, mult = 43.2, 1, 4, 1.0

	// Below cap: grain's shape is rate × placed (not rate/cap × placed).
	if got, want := placementYield(GoodGrain, rate, capL1, cap, mult, 1), rate*1; got != want {
		t.Errorf("placed=1: got %v, want %v", got, want)
	}
	if got, want := placementYield(GoodGrain, rate, capL1, cap, mult, 4), rate*4; got != want {
		t.Errorf("placed=cap(4): got %v, want %v", got, want)
	}

	// At and beyond cap, yield must plateau — never exceed rate × cap.
	atCap := placementYield(GoodGrain, rate, capL1, cap, mult, 4)
	beyondCap := placementYield(GoodGrain, rate, capL1, cap, mult, 32) // Timothy's own "32 gubbar" figure
	if beyondCap != atCap {
		t.Errorf("placed=32 (beyond cap 4) yield = %v, want it clamped to the at-cap yield %v", beyondCap, atCap)
	}
	if beyondCap != rate*cap {
		t.Errorf("placed=32 yield = %v, want exactly rate×cap = %v", beyondCap, rate*cap)
	}
}

// TestPlacementYield_ZeroCapMeansZeroYield covers the capL1<=0/placeCap<=0
// guard for every good, grain included — a good with no P3 hexCapacityRule
// entry for this terrain must yield 0, not divide by zero or fall through to
// an uncapped rate×placed formula.
func TestPlacementYield_ZeroCapMeansZeroYield(t *testing.T) {
	if got := placementYield(GoodGrain, 43.2, 1, 0, 1.0, 5); got != 0 {
		t.Errorf("grain with placeCap=0 = %v, want 0", got)
	}
	if got := placementYield("fish", 10, 4, 0, 1.0, 5); got != 0 {
		t.Errorf("fish with placeCap=0 = %v, want 0", got)
	}
	if got := placementYield("fish", 10, 0, 4, 1.0, 5); got != 0 {
		t.Errorf("fish with capL1=0 = %v, want 0", got)
	}
}

// TestPlacementYield_NonGrainDividesByCapL1AndScalesByMult is the regression
// guard for every other good's Form B formula: rate/capL1 × mult ×
// min(placed,placeCap). At mult==1 (building at level 1, or no
// level-scaling building at all) and placeCap==capL1, this is identical to
// Form A's rate/capL1 × min(placed,cap) shape (since placed never legally
// exceeds capL1==cap there); this test also proves the level-upgrade case
// (mult>1, placeCap still capL1) now yields MORE for the same rate AND
// FEWER gubbar than Form A needed — the exact axis-move
// megaron_plan_byggnadsniva_takt.md is about.
func TestPlacementYield_NonGrainDividesByCapL1AndScalesByMult(t *testing.T) {
	// mult==1, placeCap==capL1 (unlevelled / level-1 case): unchanged from
	// Form A.
	got := placementYield("fish", 40.0, 4, 4, 1.0, 2)
	want := (40.0 / 4.0) * 2.0
	if got != want {
		t.Errorf("fish placed=2/capL1=placeCap=4/mult=1: got %v, want %v", got, want)
	}
	// Beyond placeCap also plateaus for non-grain goods (pre-existing
	// behaviour, now against placeCap instead of the level-grown cap).
	if got := placementYield("fish", 40.0, 4, 4, 1.0, 32); got != 40.0 {
		t.Errorf("fish placed=32/capL1=placeCap=4/mult=1: got %v, want the at-cap yield 40.0", got)
	}

	// Form B's axis move: an upgraded silver_mine (level 3) keeps
	// placeCap==capL1==5 (headcount FROZEN, not grown to 9) but mult=9/5=1.8
	// (the level's effect, now on the rate). Staffing only capL1(5) gubbar —
	// not the old level-grown cap(9) — must already reach the SAME max Form A
	// needed 9 gubbar for.
	const silverRate = 28.799999999999997                              // live production_rules value
	l1Max := placementYield("silver", silverRate, 5, 5, 1.0, 5)        // level 1: mult=1, 5 gubbar
	l3MaxAtCapL1 := placementYield("silver", silverRate, 5, 5, 1.8, 5) // level 3: mult=1.8, STILL only 5 gubbar
	if l3MaxAtCapL1 <= l1Max {
		t.Errorf("level-3 max yield at capL1 headcount (%v) must exceed level-1 max yield (%v) — Form B's whole point", l3MaxAtCapL1, l1Max)
	}
	if want := silverRate; l1Max != want {
		t.Errorf("level-1 max yield = %v, want unchanged rate %v", l1Max, want)
	}
	// Toleransjämför — se kommentaren i hex_good_caps_form_b_test.go: samma tal,
	// annan multiplikationsordning, inte bitidentiskt.
	if want := silverRate / 5 * 9; math.Abs(l3MaxAtCapL1-want) >= 1e-9 {
		t.Errorf("level-3 max yield at capL1 headcount = %v, want rate/capL1×cap = %v (Form A's own max, reached with FEWER gubbar)", l3MaxAtCapL1, want)
	}

	// Placing beyond capL1 (up to the OLD level-grown cap 9) must not add
	// anything further — the defensive clip now bites at capL1, not cap.
	l3MaxOverstaffed := placementYield("silver", silverRate, 5, 5, 1.8, 9)
	if l3MaxOverstaffed != l3MaxAtCapL1 {
		t.Errorf("level-3 yield with 9 gubbar (only 5 fit) = %v, want it clamped to the capL1 max %v", l3MaxOverstaffed, l3MaxAtCapL1)
	}
}
