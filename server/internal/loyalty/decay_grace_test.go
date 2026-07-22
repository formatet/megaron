package loyalty

// Regression test for the loyalty-decay grace window (Fynd 2026-07-08, sub-minute
// fix 2026-07-22).
//
// Root cause: DecayHandler.Handle filtered "recently reassured" colonies with a
// hard-coded `le.created_at > now() - interval '48 hours'` WALL-CLOCK window,
// while the decay tick itself is TICK-scheduled (fires once per game-day). The
// first fix scaled through tick.TickMinutes — but TickMinutes FLOORS to 1 minute,
// so on the CT 126 sub-minute cadence (TICK_SECONDS=6) 2 game-days (= 48 ticks =
// 288 real seconds = 4.8 min) still read as 2×24×1 = 48 minutes — ~10× too long,
// leaving decay effectively disabled again. The window is now graceDays ×
// TicksPerDay × TickSeconds SECONDS, scaling exactly like tick/eta.go.

import (
	"testing"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/tick"
)

// withTickSeconds overrides the package-level tick.TickSeconds for the duration
// of a test and restores it after — a plain var (read once from the env at init),
// safe to swap in-process. Mirrors the messenger travel-duration tests.
func withTickSeconds(t *testing.T, seconds int) {
	t.Helper()
	orig := tick.TickSeconds
	tick.TickSeconds = seconds
	t.Cleanup(func() { tick.TickSeconds = orig })
}

// TestDecayGraceSeconds_DefaultCadence pins the historical (correct-by-
// coincidence) case: at the default 60 min/tick (3600 s) the grace window must
// still be 172800 s (48 h), so the fix does not change behaviour for the common case.
func TestDecayGraceSeconds_DefaultCadence(t *testing.T) {
	withTickSeconds(t, 3600)
	want := loyaltyDecayGraceDays * events.TicksPerDay * 3600 // 2 × 24 × 3600 = 172800 (48 h)
	if got := decayGraceSeconds(); got != want {
		t.Errorf("decayGraceSeconds() at default cadence = %d, want %d (== 48 h)", got, want)
	}
}

// TestDecayGraceSeconds_SubMinuteCadence is the load-bearing case: on the CT 126
// dev cadence (TICK_SECONDS=6) the window must collapse to a true 2 game-days =
// 288 real seconds (4.8 min), not the 48 minutes the floored-minute conversion
// produced. This is the exact case the earlier TickMinutes fix still got wrong.
func TestDecayGraceSeconds_SubMinuteCadence(t *testing.T) {
	withTickSeconds(t, 6)
	want := loyaltyDecayGraceDays * events.TicksPerDay * 6 // 2 × 24 × 6 = 288 s = 4.8 min
	if got := decayGraceSeconds(); got != want {
		t.Errorf("decayGraceSeconds() at TICK_SECONDS=6 = %d, want %d (== 2 game-days)", got, want)
	}
	if got := decayGraceSeconds(); got >= 48*60 {
		t.Errorf("decayGraceSeconds() = %d s still ≥ the floored-minute 48-minute window", got)
	}
}

// TestDecayGraceSeconds_ScalesWithCadence checks Timothy's normal 2 min/tick
// cadence (120 s/tick → 2 game-days = 5760 s = 96 real minutes).
func TestDecayGraceSeconds_ScalesWithCadence(t *testing.T) {
	withTickSeconds(t, 120)
	want := loyaltyDecayGraceDays * events.TicksPerDay * 120 // 2 × 24 × 120 = 5760 s = 96 min
	if got := decayGraceSeconds(); got != want {
		t.Errorf("decayGraceSeconds() at 120 s/tick = %d, want %d", got, want)
	}
}
