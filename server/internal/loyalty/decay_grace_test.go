package loyalty

// Regression test for the loyalty-decay grace window (Fynd 2026-07-08).
//
// Root cause: DecayHandler.Handle filtered "recently reassured" colonies with a
// hard-coded `le.created_at > now() - interval '48 hours'` WALL-CLOCK window,
// while the decay tick itself is TICK-scheduled (fires once per game-day,
// e.DueTick + events.TicksPerDay). At the default 60 min/tick the two happen to
// agree (1 game-day = 24 ticks = 24 real-hours, so 48 h = 2 game-days). But on
// the CT 126 dev server (TICK_MINUTES=1) 48 real-hours = 2880 ticks = 120
// game-days, so the grace window swallowed the entire run and decay never fired
// — the "loyalty 2 uniformly" soak symptom. Same bug class as the messenger
// ETA fix (f354fd6): a tick-quantity expressed in raw wall-clock time.
//
// The window is now graceDays × TicksPerDay × TickMinutes minutes, scaling
// through tick.TickMinutes exactly like the messenger travel durations.

import (
	"testing"

	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/tick"
)

// withTickMinutes overrides the package-level tick.TickMinutes for the duration
// of a test and restores it after — a plain var (read once from TICK_MINUTES at
// init), safe to swap in-process. Mirrors the messenger travel-duration tests.
func withTickMinutes(t *testing.T, minutes int) {
	t.Helper()
	orig := tick.TickMinutes
	tick.TickMinutes = minutes
	t.Cleanup(func() { tick.TickMinutes = orig })
}

// TestDecayGraceMinutes_DefaultCadence pins the historical (correct-by-
// coincidence) case: at the default 60 min/tick the grace window must still be
// 2880 minutes (48 h), so the fix does not change behaviour for the common case.
func TestDecayGraceMinutes_DefaultCadence(t *testing.T) {
	withTickMinutes(t, 60)
	want := loyaltyDecayGraceDays * events.TicksPerDay * 60 // 2 × 24 × 60 = 2880
	if got := decayGraceMinutes(); got != want {
		t.Errorf("decayGraceMinutes() at default cadence = %d, want %d (== 48 h)", got, want)
	}
}

// TestDecayGraceMinutes_FastCadence is the load-bearing case: on a sped-up world
// (TICK_MINUTES=1) the window must collapse to a true 2 game-days = 48 real
// minutes, not 48 real hours. Before the fix this was effectively ~120
// game-days and decay never fired.
func TestDecayGraceMinutes_FastCadence(t *testing.T) {
	withTickMinutes(t, 1)
	want := loyaltyDecayGraceDays * events.TicksPerDay * 1 // 2 × 24 × 1 = 48
	if got := decayGraceMinutes(); got != want {
		t.Errorf("decayGraceMinutes() at TICK_MINUTES=1 = %d, want %d (== 2 game-days)", got, want)
	}
	if got := decayGraceMinutes(); got >= 48*60 {
		t.Errorf("decayGraceMinutes() = %d min still spans the old 48-hour wall-clock window", got)
	}
}

// TestDecayGraceMinutes_ScalesWithCadence checks the window tracks Timothy's
// normal 2 min/tick cadence too (2 game-days = 96 real minutes).
func TestDecayGraceMinutes_ScalesWithCadence(t *testing.T) {
	withTickMinutes(t, 2)
	want := loyaltyDecayGraceDays * events.TicksPerDay * 2 // 2 × 24 × 2 = 96
	if got := decayGraceMinutes(); got != want {
		t.Errorf("decayGraceMinutes() at TICK_MINUTES=2 = %d, want %d", got, want)
	}
}
