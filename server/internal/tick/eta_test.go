package tick

import (
	"testing"
	"time"

	"github.com/poleia/server/internal/clock"
)

// withTickSecondsForTest overrides the package-level TickSeconds for the
// duration of a test and restores it after — TickSeconds is a plain var
// (read once from TICK_SECONDS/TICK_MINUTES at init), safe to swap in-process.
func withTickSecondsForTest(t *testing.T, seconds int) {
	t.Helper()
	orig := TickSeconds
	TickSeconds = seconds
	t.Cleanup(func() { TickSeconds = orig })
}

// TestRealUntil_FloorsAtZero is the "already due / overdue" case: dueTick at
// or before currentTick must never yield a negative duration.
func TestRealUntil_FloorsAtZero(t *testing.T) {
	withTickSecondsForTest(t, 6)
	if got := RealUntil(5, 10); got != 0 {
		t.Errorf("RealUntil(5, 10) = %v, want 0 (dueTick already passed)", got)
	}
	if got := RealUntil(10, 10); got != 0 {
		t.Errorf("RealUntil(10, 10) = %v, want 0 (dueTick == currentTick)", got)
	}
}

// TestRealUntil_ExactAtSubMinuteCadence is the load-bearing case: on
// TICK_SECONDS=6 (the CT 126 dev cadence) the derived duration must be the
// EXACT (dueTick-currentTick)*6s — never floored to whole minutes the way
// TickMinutes would (TickMinutes=6/60 floors to 1, i.e. 10x too long).
func TestRealUntil_ExactAtSubMinuteCadence(t *testing.T) {
	withTickSecondsForTest(t, 6)
	got := RealUntil(14, 4) // 10 ticks remaining
	want := 60 * time.Second
	if got != want {
		t.Errorf("RealUntil(14, 4) at TICK_SECONDS=6 = %v, want %v (10 ticks * 6s, exact)", got, want)
	}
}

// TestRealUntil_DefaultCadence pins the historical 60 min/tick default: 1
// tick remaining should still be exactly 1 hour.
func TestRealUntil_DefaultCadence(t *testing.T) {
	withTickSecondsForTest(t, 3600)
	if got := RealUntil(1, 0); got != time.Hour {
		t.Errorf("RealUntil(1, 0) at default cadence = %v, want 1h", got)
	}
}

func TestEtaAt_AddsRealUntilToClockNow(t *testing.T) {
	withTickSecondsForTest(t, 6)
	base := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTestClock(base)

	got := EtaAt(clk, 14, 4) // 10 ticks * 6s = 60s
	want := base.Add(60 * time.Second)
	if !got.Equal(want) {
		t.Errorf("EtaAt(clk, 14, 4) = %v, want %v", got, want)
	}
}

func TestEtaAt_FloorsAtZero(t *testing.T) {
	withTickSecondsForTest(t, 6)
	base := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTestClock(base)

	got := EtaAt(clk, 1, 10) // dueTick already passed
	if !got.Equal(base) {
		t.Errorf("EtaAt(clk, 1, 10) = %v, want %v (clk.Now(), no negative offset)", got, base)
	}
}
