package tick

import (
	"testing"
	"time"
)

// TestAnchor_MilliAt_Clamps is R3 (megaron_plan_rorelse_sparad_vag.md): a
// stale read (now before At) never goes negative, and a now past a full tick
// (the tick worker hasn't advanced yet) never spills into the next tick's
// Milli range.
func TestAnchor_MilliAt_Clamps(t *testing.T) {
	withTickSecondsForTest(t, 60)
	at := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	a := Anchor{Tick: 5, At: at}

	if got := a.MilliAt(at.Add(-10 * time.Second)); got != 5000 {
		t.Errorf("MilliAt(before At) = %d, want 5000 (clamped to the tick's own start)", got)
	}
	if got := a.MilliAt(at.Add(5 * time.Minute)); got != 5999 {
		t.Errorf("MilliAt(long past the tick) = %d, want 5999 (clamped to the tick's own end)", got)
	}
	if got := a.MilliAt(at); got != 5000 {
		t.Errorf("MilliAt(exactly At) = %d, want 5000", got)
	}
	if got := a.MilliAt(at.Add(30 * time.Second)); got != 5500 {
		t.Errorf("MilliAt(halfway through the tick) = %d, want 5500", got)
	}
}

// TestAnchor_RoundTrip: MilliAt(WallAt(m)) returns m for every m within the
// anchor's own tick (the ONLY range WallAt/MilliAt agree on a shared meaning —
// MilliAt clamps outside it, so a round trip is only expected inside).
func TestAnchor_RoundTrip(t *testing.T) {
	withTickSecondsForTest(t, 3600) // 1h ticks, so 1000 Milli units = 3.6s each — no rounding loss
	at := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	a := Anchor{Tick: 100, At: at}

	for m := int64(100000); m <= 100999; m += 37 {
		wall := a.WallAt(m)
		got := a.MilliAt(wall)
		if got != m {
			t.Errorf("MilliAt(WallAt(%d)) = %d, want %d", m, got, m)
		}
	}
}

// TestAnchor_WallAt_Endpoints: WallAt(Tick*1000) is exactly At, and
// WallAt((Tick+1)*1000) is exactly one tick duration later.
func TestAnchor_WallAt_Endpoints(t *testing.T) {
	withTickSecondsForTest(t, 60)
	at := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	a := Anchor{Tick: 5, At: at}

	if got := a.WallAt(5000); !got.Equal(at) {
		t.Errorf("WallAt(Tick*1000) = %v, want %v", got, at)
	}
	want := at.Add(60 * time.Second)
	if got := a.WallAt(6000); !got.Equal(want) {
		t.Errorf("WallAt((Tick+1)*1000) = %v, want %v", got, want)
	}
}
