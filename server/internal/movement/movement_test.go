package movement

import (
	"reflect"
	"testing"
)

// findHex returns the Interval for hex h in ivs, or fails the test if absent.
func findHex(t *testing.T, ivs []Interval, h Hex) Interval {
	t.Helper()
	for _, iv := range ivs {
		if iv.Hex == h {
			return iv
		}
	}
	t.Fatalf("hex %+v not found in %+v", h, ivs)
	return Interval{}
}

// T1 (R1 terrain, megaron_plan_rorelsekarnan.md "Vilken hex äger vilket
// intervall", Timothy 2026-09-27 val A): plains(0.75) -> ford(2.5) ->
// plains(0.75). The cost of stepping into a hex IS the time spent standing
// in it, so the ford (the expensive hex to enter) gets the long interval —
// not the hex before it (that was the first version's bug, corrected here).
func TestT1_R1_Terrain(t *testing.T) {
	m := Move{
		StartTick: 0, EndTick: 10,
		Hexes: []Hex{{0, 0}, {1, 0}, {2, 0}}, // plains, ford, plains
		Costs: []int64{2500, 750},            // milli-ticks: cost to enter ford, cost to enter target plains
	}
	bounds, err := Boundaries(m)
	if err != nil {
		t.Fatalf("Boundaries: %v", err)
	}
	want := []Milli{0, 7692, 10000}
	if !reflect.DeepEqual(bounds, want) {
		t.Fatalf("Boundaries = %v, want %v", bounds, want)
	}

	pres, err := Presence(m, 0, 10000)
	if err != nil {
		t.Fatalf("Presence: %v", err)
	}

	// The start hex has no transit interval within the move's own window:
	// it is left instantly at b_0 = Start*1000.
	for _, iv := range pres {
		if iv.Hex == m.Hexes[0] {
			t.Fatalf("start hex %+v should have no presence in [Start*1000, End*1000), got %+v", m.Hexes[0], iv)
		}
	}

	ford := findHex(t, pres, Hex{1, 0})
	if ford.In != 0 || ford.Out != 7692 {
		t.Fatalf("ford interval = [%d,%d), want [0,7692)", ford.In, ford.Out)
	}
	target := findHex(t, pres, Hex{2, 0})
	if target.In != 7692 || target.Out != 10000 {
		t.Fatalf("target interval = [%d,%d), want [7692,10000)", target.In, target.Out)
	}

	fordDwell := ford.Out - ford.In             // 7692
	targetPaidPortion := target.Out - target.In // 2308: the part paid for before the executed arrival
	ratio := float64(fordDwell) / float64(targetPaidPortion)
	if ratio <= 3.0 || ratio >= 3.5 {
		t.Fatalf("ford should dwell just over 3x the target's paid portion, got ratio %.4f (%d vs %d)", ratio, fordDwell, targetPaidPortion)
	}
}

// T2 (R2 endpoints): b_0 = Start*1000 and b_n = End*1000 exactly, for many
// randomly generated valid Moves.
func TestT2_R2_Endpoints(t *testing.T) {
	for seed := int64(0); seed < 200; seed++ {
		m := randomMove(newRNG(seed))
		bounds, err := Boundaries(m)
		if err != nil {
			t.Fatalf("seed %d: Boundaries: %v", seed, err)
		}
		wantFirst := Milli(m.StartTick) * millisPerTick
		wantLast := Milli(m.EndTick) * millisPerTick
		if bounds[0] != wantFirst {
			t.Fatalf("seed %d: bounds[0] = %d, want %d", seed, bounds[0], wantFirst)
		}
		if bounds[len(bounds)-1] != wantLast {
			t.Fatalf("seed %d: bounds[last] = %d, want %d", seed, bounds[len(bounds)-1], wantLast)
		}
	}
}

// T3 (several hexes in one tick): a cheap five-hex path crossed within a
// single tick. All boundaries fall in [Start*1000, End*1000]. Presence is
// queried with a small margin on each side (rather than exactly
// [Start*1000, End*1000)) so that the start hex's sliver before b_0 and the
// target's sliver after b_3 both show up too — under the corrected model
// (T1/"val A") those two hexes have zero width strictly inside the move's
// own window, since the start hex is left instantly at b_0 and the target is
// already reached at b_3, before the tick's own end (megaron_plan_
// rorelsekarnan.md, "Vilken hex äger vilket intervall").
func TestT3_MultipleHexesPerTick(t *testing.T) {
	m := Move{
		StartTick: 0, EndTick: 1,
		Hexes: []Hex{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}},
		Costs: []int64{1, 1, 1, 1}, // cheap and equal
	}
	bounds, err := Boundaries(m)
	if err != nil {
		t.Fatalf("Boundaries: %v", err)
	}
	for i, b := range bounds {
		if b < 0 || b > 1000 {
			t.Fatalf("bounds[%d] = %d, want within [0,1000]", i, b)
		}
	}

	pres, err := Presence(m, -1, 1001)
	if err != nil {
		t.Fatalf("Presence: %v", err)
	}
	if len(pres) != len(m.Hexes) {
		t.Fatalf("Presence returned %d hexes, want all %d: %+v", len(pres), len(m.Hexes), pres)
	}
	seen := map[Hex]bool{}
	for _, iv := range pres {
		seen[iv.Hex] = true
	}
	for _, h := range m.Hexes {
		if !seen[h] {
			t.Fatalf("hex %+v missing from Presence result %+v", h, pres)
		}
	}
}

// T4 (PositionAt at the boundary): at exact b_i (i<n), the actor has already
// moved on to Hexes[i+1] (half-open). At exact b_0 it has left the start hex.
func TestT4_PositionAtBoundary(t *testing.T) {
	m := Move{
		StartTick: 0, EndTick: 10,
		Hexes: []Hex{{0, 0}, {1, 0}, {2, 0}},
		Costs: []int64{2500, 750},
	}
	bounds, err := Boundaries(m) // [0, 7692, 10000]
	if err != nil {
		t.Fatalf("Boundaries: %v", err)
	}

	// Before b_0: still at the start hex.
	got, err := PositionAt(m, bounds[0]-1)
	if err != nil {
		t.Fatalf("PositionAt: %v", err)
	}
	if got != m.Hexes[0] {
		t.Fatalf("PositionAt(b_0-1) = %+v, want start hex %+v", got, m.Hexes[0])
	}

	// At exact b_0: already left the start hex, now in Hexes[1].
	got, err = PositionAt(m, bounds[0])
	if err != nil {
		t.Fatalf("PositionAt: %v", err)
	}
	if got != m.Hexes[1] {
		t.Fatalf("PositionAt(b_0) = %+v, want Hexes[1] %+v", got, m.Hexes[1])
	}

	// At exact b_1 (i=1 < n=2): in Hexes[2] (the target).
	got, err = PositionAt(m, bounds[1])
	if err != nil {
		t.Fatalf("PositionAt: %v", err)
	}
	if got != m.Hexes[2] {
		t.Fatalf("PositionAt(b_1) = %+v, want Hexes[2] %+v", got, m.Hexes[2])
	}

	// Long after arrival: still at the target.
	got, err = PositionAt(m, bounds[len(bounds)-1]+100000)
	if err != nil {
		t.Fatalf("PositionAt: %v", err)
	}
	if got != m.Hexes[2] {
		t.Fatalf("PositionAt(far future) = %+v, want target %+v", got, m.Hexes[2])
	}
}

// T5 (R3 opposite-direction swap): A: 1->2, B: 2->1. Early, late and exactly
// simultaneous crossings all count as contact, plus a crossing through a
// genuine zero-length hex.
func TestT5_OppositeSwap(t *testing.T) {
	h1, h2 := Hex{1, 0}, Hex{2, 0}

	t.Run("early: A crosses before B", func(t *testing.T) {
		a := []Interval{{h1, 0, 100}, {h2, 100, 500}}
		b := []Interval{{h2, 0, 200}, {h1, 200, 500}}
		hex, at, ok := FirstContact(a, b)
		if !ok || hex != h2 || at != 100 {
			t.Fatalf("FirstContact = (%+v,%d,%v), want (%+v,100,true)", hex, at, ok, h2)
		}
	})

	t.Run("late: B crosses before A", func(t *testing.T) {
		a := []Interval{{h1, 0, 200}, {h2, 200, 500}}
		b := []Interval{{h2, 0, 100}, {h1, 100, 500}}
		hex, at, ok := FirstContact(a, b)
		if !ok || hex != h1 || at != 100 {
			t.Fatalf("FirstContact = (%+v,%d,%v), want (%+v,100,true)", hex, at, ok, h1)
		}
	})

	t.Run("exact: same instant", func(t *testing.T) {
		a := []Interval{{h1, 0, 150}, {h2, 150, 500}}
		b := []Interval{{h2, 0, 150}, {h1, 150, 500}}
		hex, at, ok := FirstContact(a, b)
		if !ok || hex != h1 || at != 150 {
			t.Fatalf("FirstContact = (%+v,%d,%v), want (%+v,150,true)", hex, at, ok, h1)
		}
	})

	t.Run("via a genuine zero-length hex", func(t *testing.T) {
		// A very cheap middle hex M on a short trip collapses to [b,b): a
		// Move with three hexes where the middle one's boundaries coincide.
		m := Hex{9, 9}
		aMove := Move{StartTick: 0, EndTick: 1, Hexes: []Hex{h1, m, h2}, Costs: []int64{1, 999999}}
		bMove := Move{StartTick: 0, EndTick: 1, Hexes: []Hex{h2, m, h1}, Costs: []int64{1, 999999}}

		aBounds, err := Boundaries(aMove)
		if err != nil {
			t.Fatalf("Boundaries(aMove): %v", err)
		}
		if aBounds[0] != aBounds[1] {
			t.Fatalf("precondition failed: expected M's interval to be zero-length, bounds=%v", aBounds)
		}

		aPres, err := Presence(aMove, -500, 1000)
		if err != nil {
			t.Fatalf("Presence(aMove): %v", err)
		}
		bPres, err := Presence(bMove, -500, 1000)
		if err != nil {
			t.Fatalf("Presence(bMove): %v", err)
		}
		for _, iv := range aPres {
			if iv.Hex == m {
				t.Fatalf("zero-length hex M should not appear in Presence, got %+v", aPres)
			}
		}

		hex, at, ok := FirstContact(aPres, bPres)
		if !ok || hex != h1 || at != 0 {
			t.Fatalf("FirstContact = (%+v,%d,%v), want (%+v,0,true); aPres=%+v bPres=%+v", hex, at, ok, h1, aPres, bPres)
		}
	})
}

// T6 (R3 no contact): B follows A in the same direction. A leaves hex 2 at
// 400, B enters hex 2 at exactly 400 — half-open, so no contact, and it is
// not an opposite-direction swap. Separated, non-overlapping times in the
// same direction also give no contact.
func TestT6_SameDirectionNoContact(t *testing.T) {
	h1, h2, h3 := Hex{1, 0}, Hex{2, 0}, Hex{3, 0}

	t.Run("touching at the half-open boundary", func(t *testing.T) {
		// B follows A with a constant lag, one hex-duration behind at every
		// step, so it never shares a hex with A — it only ever touches the
		// boundary A just left (e.g. hex 2 at exactly 400).
		a := []Interval{{h1, 0, 200}, {h2, 200, 400}, {h3, 400, 600}}
		b := []Interval{{h1, 200, 400}, {h2, 400, 600}, {h3, 600, 800}}
		_, _, ok := FirstContact(a, b)
		if ok {
			t.Fatalf("FirstContact should report no contact for a same-direction touch")
		}
	})

	t.Run("no overlap at all", func(t *testing.T) {
		a := []Interval{{h1, 0, 100}, {h2, 100, 200}}
		b := []Interval{{h1, 300, 400}, {h2, 400, 500}}
		_, _, ok := FirstContact(a, b)
		if ok {
			t.Fatalf("FirstContact should report no contact for non-overlapping same-direction travel")
		}
	})
}

// T7 (rounding trap): three waypoints, queried at 80% of the journey. This
// documents the new, exact answer — it does not replace
// province.InterpolateAlongPath (round-based) or province.InterpolatePosition
// (floor-based) here, that is a later slice.
func TestT7_RoundingTrap(t *testing.T) {
	m := Move{
		StartTick: 0, EndTick: 10,
		Hexes: []Hex{{0, 0}, {1, 0}, {2, 0}},
		Costs: []int64{9000, 1000}, // Hexes[1]'s entry cost dominates the journey
	}
	t80 := Milli(8000) // 80% of the 10000-Milli journey
	got, err := PositionAt(m, t80)
	if err != nil {
		t.Fatalf("PositionAt: %v", err)
	}
	t.Logf("T7: PositionAt(m, 8000) = %+v", got)

	// Old province.InterpolateAlongPath: idx = round(0.8*(3-1)) = round(1.6) = 2 -> the TARGET hex.
	// Old province.InterpolatePosition:  idx = floor(0.8*(3-1)) = floor(1.6) = 1 -> Hexes[1], same as
	// below, but for the wrong reason: both ignore terrain cost entirely and distribute position evenly
	// by index (megaron_rorelseregler.md §Varför, "positionen fördelas jämnt per hex"). This answer is
	// exact because Hexes[1]'s entry cost (9000 of 10000) genuinely dominates the journey.
	want := Hex{1, 0}
	if got != want {
		t.Fatalf("PositionAt(80%%) = %+v, want %+v", got, want)
	}
}

// T8 (R5 example, geometry only): FirstContact(A,B) at 0.300 and
// FirstContact(B,C) at 0.700. Proves times are computed correctly; chain
// invalidation itself is slice 3.
func TestT8_R5ChainGeometry(t *testing.T) {
	x, y := Hex{0, 0}, Hex{1, 0}
	a := []Interval{{x, 0, 500}}
	b := []Interval{{x, 300, 600}, {y, 600, 1200}}
	c := []Interval{{y, 700, 1000}}

	hex, at, ok := FirstContact(a, b)
	if !ok || hex != x || at != 300 {
		t.Fatalf("FirstContact(A,B) = (%+v,%d,%v), want (%+v,300,true)", hex, at, ok, x)
	}
	hex, at, ok = FirstContact(b, c)
	if !ok || hex != y || at != 700 {
		t.Fatalf("FirstContact(B,C) = (%+v,%d,%v), want (%+v,700,true)", hex, at, ok, y)
	}
}

// Validate error-path coverage: shape and overflow guards.
func TestValidate(t *testing.T) {
	valid := Move{StartTick: 0, EndTick: 1, Hexes: []Hex{{0, 0}, {1, 0}}, Costs: []int64{1}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid Move rejected: %v", err)
	}

	cases := []struct {
		name string
		m    Move
	}{
		{"too few hexes", Move{StartTick: 0, EndTick: 1, Hexes: []Hex{{0, 0}}, Costs: nil}},
		{"cost length mismatch", Move{StartTick: 0, EndTick: 1, Hexes: []Hex{{0, 0}, {1, 0}, {2, 0}}, Costs: []int64{1}}},
		{"EndTick not after StartTick", Move{StartTick: 5, EndTick: 5, Hexes: []Hex{{0, 0}, {1, 0}}, Costs: []int64{1}}},
		{"non-positive cost", Move{StartTick: 0, EndTick: 1, Hexes: []Hex{{0, 0}, {1, 0}}, Costs: []int64{0}}},
		{"D*T overflow", Move{StartTick: 0, EndTick: 1 << 40, Hexes: []Hex{{0, 0}, {1, 0}}, Costs: []int64{1 << 40}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.m.Validate(); err == nil {
				t.Fatalf("expected an error, got nil")
			}
		})
	}
}
