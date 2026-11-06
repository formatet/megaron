package movement

import "testing"

func speedRoute() StoredRoute {
	hexes := [][2]int{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}, {5, 0}}
	return StoredRoute{Version: 2, OffsetMilli: 250, StartTick: 100, EndTick: 120, Hexes: hexes, Costs: []int64{1000, 3000, 2000, 2000, 2000}}
}

func TestSpeedUpRemaining_ArrivalPositionAndPastBoundaries(t *testing.T) {
	r := speedRoute()
	for _, now := range []int64{100250, 103900, 108000, 114100, 118999} {
		got, ok := SpeedUpRemaining(r, now, 3, 4)
		left := int64(120) - now/1000
		want := now/1000 + max(1, (3*left+3)/4)
		if !ok {
			if want < 120 {
				t.Fatalf("now %d: expected a speed-up to tick %d", now, want)
			}
			continue
		}
		if int64(got.EndTick) != want {
			t.Fatalf("now %d: arrival tick %d, want %d", now, got.EndTick, want)
		}
		before, _ := PositionAt(r.Move(), Milli(now))
		after, err := PositionAt(got.Move(), Milli(now))
		if err != nil || before != after {
			t.Fatalf("now %d: position changed %v -> %v (%v)", now, before, after, err)
		}
		ob, _ := Boundaries(r.Move())
		nb, err := Boundaries(got.Move())
		if err != nil {
			t.Fatal(err)
		}
		for i := range ob {
			if int64(ob[i]) <= now && ob[i] != nb[i] {
				t.Fatalf("now %d: passed boundary %d moved %d -> %d", now, i, ob[i], nb[i])
			}
			if int64(ob[i]) > now && nb[i] >= ob[i] && i < len(ob)-1 {
				t.Fatalf("now %d: boundary %d ahead did not come closer", now, i)
			}
		}
		if nb[len(nb)-1] != Milli(want*1000+250) {
			t.Fatalf("now %d: last boundary %d", now, nb[len(nb)-1])
		}
	}
}

func TestSpeedUpRemaining_MarkedOrLegacyOrOneTickLeftIsUntouched(t *testing.T) {
	r := speedRoute()
	r.Sped = "already"
	if _, ok := SpeedUpRemaining(r, 105000, 3, 4); ok {
		t.Fatal("a marked journey must not be sped up again")
	}
	r = speedRoute()
	r.Version = 0
	r.OffsetMilli = 0
	if _, ok := SpeedUpRemaining(r, 105000, 3, 4); ok {
		t.Fatal("a route without the precise clock is left alone")
	}
	r = speedRoute()
	if _, ok := SpeedUpRemaining(r, 119100, 3, 4); ok {
		t.Fatal("one tick left gains nothing")
	}
}

func TestSpeedUpRemaining_ComesBeforeDepartureCountsFromStart(t *testing.T) {
	r := speedRoute()
	got, ok := SpeedUpRemaining(r, 90000, 3, 4)
	if !ok || got.EndTick != 100+15 {
		t.Fatalf("whole journey x0.75: got %d ok %v", got.EndTick, ok)
	}
}
