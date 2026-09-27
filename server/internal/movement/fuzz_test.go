package movement

import (
	"math/rand"
	"reflect"
	"testing"
)

// newRNG returns a deterministic generator for a given seed — used by both
// the plain seeded loop in TestT2 and the two Fuzz functions below, which
// fuzz the seed rather than hand-decoding a byte slice into a Move.
func newRNG(seed int64) *rand.Rand {
	return rand.New(rand.NewSource(seed))
}

// randomMove builds a structurally valid Move (Validate() passes) from rng.
// Hex identity doesn't affect Boundaries/PositionAt/Presence math at all, so
// hexes are just distinct placeholders.
func randomMove(rng *rand.Rand) Move {
	start := int64(rng.Intn(2000) - 1000)
	length := int64(rng.Intn(2000) + 1)
	end := start + length

	numCosts := rng.Intn(15) + 1
	costs := make([]int64, numCosts)
	for i := range costs {
		costs[i] = int64(rng.Intn(100000) + 1)
	}
	hexes := make([]Hex, numCosts+1)
	for i := range hexes {
		hexes[i] = Hex{Q: rng.Intn(2000) - 1000, R: rng.Intn(2000) - 1000}
	}

	return Move{
		StartTick: int(start),
		EndTick:   int(end),
		Hexes:     hexes,
		Costs:     costs,
	}
}

// mergeAdjacent merges consecutive same-hex, contiguous intervals
// (prev.Out == next.In) into one. Used to compare Presence(a,b)+Presence(b,c)
// against Presence(a,c): splitting a hex's occupancy exactly at b produces
// two adjacent pieces where Presence(a,c) produces one ("sammanfogat" in the
// plan — joined, not just concatenated).
func mergeAdjacent(ivs []Interval) []Interval {
	if len(ivs) == 0 {
		return ivs
	}
	out := []Interval{ivs[0]}
	for _, iv := range ivs[1:] {
		last := &out[len(out)-1]
		if last.Hex == iv.Hex && last.Out == iv.In {
			last.Out = iv.Out
			continue
		}
		out = append(out, iv)
	}
	return out
}

// FuzzBoundaries checks R2's invariants for random valid Moves: monotonic
// boundaries, exact endpoints, PositionAt always on the path, determinism.
func FuzzBoundaries(f *testing.F) {
	f.Add(int64(1))
	f.Add(int64(42))
	f.Add(int64(0))
	f.Fuzz(func(t *testing.T, seed int64) {
		m := randomMove(newRNG(seed))
		if err := m.Validate(); err != nil {
			t.Skip() // e.g. the D*T overflow guard rejected this seed's scale — not a bug
		}

		bounds, err := Boundaries(m)
		if err != nil {
			t.Fatalf("Boundaries: %v", err)
		}

		for i := 1; i < len(bounds); i++ {
			if bounds[i] < bounds[i-1] {
				t.Fatalf("bounds not monotonic: %v", bounds)
			}
		}
		if want := Milli(m.StartTick) * millisPerTick; bounds[0] != want {
			t.Fatalf("bounds[0] = %d, want %d", bounds[0], want)
		}
		if want := Milli(m.EndTick) * millisPerTick; bounds[len(bounds)-1] != want {
			t.Fatalf("bounds[last] = %d, want %d", bounds[len(bounds)-1], want)
		}

		bounds2, err := Boundaries(m)
		if err != nil || !reflect.DeepEqual(bounds, bounds2) {
			t.Fatalf("Boundaries is not deterministic: %v vs %v (err=%v)", bounds, bounds2, err)
		}

		valid := make(map[Hex]bool, len(m.Hexes))
		for _, h := range m.Hexes {
			valid[h] = true
		}
		for _, probe := range []Milli{bounds[0] - 1, bounds[0], bounds[len(bounds)-1], bounds[len(bounds)-1] + 1} {
			pos, err := PositionAt(m, probe)
			if err != nil {
				t.Fatalf("PositionAt(%d): %v", probe, err)
			}
			if !valid[pos] {
				t.Fatalf("PositionAt(%d) = %+v, not one of the path's hexes %v", probe, pos, m.Hexes)
			}
		}
	})
}

// FuzzPresence checks that Presence(a,c) equals Presence(a,b) joined with
// Presence(b,c) for a <= b <= c, and that Presence is deterministic.
func FuzzPresence(f *testing.F) {
	f.Add(int64(1), int64(-500), int64(3000), int64(7000))
	f.Add(int64(42), int64(0), int64(500), int64(500))
	f.Fuzz(func(t *testing.T, seed, aOff, bOff, cOff int64) {
		rng := newRNG(seed)
		m := randomMove(rng)
		if err := m.Validate(); err != nil {
			t.Skip()
		}

		lo := int64(m.StartTick)*1000 - 5000
		hi := int64(m.EndTick)*1000 + 5000
		span := hi - lo
		if span <= 0 {
			t.Skip()
		}
		pick := func(off int64) Milli {
			v := ((off % span) + span) % span
			return Milli(lo + v)
		}
		a, b, c := pick(aOff), pick(bOff), pick(cOff)
		if a > b {
			a, b = b, a
		}
		if b > c {
			b, c = c, b
		}
		if a > b {
			a, b = b, a
		}

		pAC, err := Presence(m, a, c)
		if err != nil {
			t.Fatalf("Presence(a,c): %v", err)
		}
		pAB, err := Presence(m, a, b)
		if err != nil {
			t.Fatalf("Presence(a,b): %v", err)
		}
		pBC, err := Presence(m, b, c)
		if err != nil {
			t.Fatalf("Presence(b,c): %v", err)
		}
		combined := mergeAdjacent(append(append([]Interval{}, pAB...), pBC...))
		if !reflect.DeepEqual(pAC, combined) {
			t.Fatalf("Presence(a,c) = %+v, want Presence(a,b)+Presence(b,c) joined = %+v (a=%d b=%d c=%d)", pAC, combined, a, b, c)
		}

		pAC2, err := Presence(m, a, c)
		if err != nil || !reflect.DeepEqual(pAC, pAC2) {
			t.Fatalf("Presence is not deterministic: %+v vs %+v (err=%v)", pAC, pAC2, err)
		}
	})
}
