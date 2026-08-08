package hexgrid

import "testing"

func TestRingOrdinal_MatchesRingOrder(t *testing.T) {
	center := Coord{10, -4}
	ring := Ring(center, CatchmentRadius)
	if len(ring) != 18 {
		t.Fatalf("Ring(radius=%d) = %d hexes, want 18", CatchmentRadius, len(ring))
	}
	for wantOrdinal, c := range ring {
		gotOrdinal, ok := RingOrdinal(center, CatchmentRadius, c)
		if !ok {
			t.Fatalf("RingOrdinal(%v) not found, want ordinal %d", c, wantOrdinal+1)
		}
		if gotOrdinal != wantOrdinal+1 {
			t.Errorf("RingOrdinal(%v) = %d, want %d", c, gotOrdinal, wantOrdinal+1)
		}
	}
}

func TestRingOrdinal_CenterAndOutsideAreNotInRing(t *testing.T) {
	center := Coord{0, 0}
	if _, ok := RingOrdinal(center, CatchmentRadius, center); ok {
		t.Error("center hex reported as part of its own ring")
	}
	if _, ok := RingOrdinal(center, CatchmentRadius, Coord{99, 99}); ok {
		t.Error("far-away hex reported as part of the ring")
	}
}
