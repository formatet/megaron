package messenger

// Regression test for Fas 1d: negative ETAs/"ago" in the CLI (e.g. a trade
// offer's ETA before its own send time, "-145m ago" on a delivered message).
//
// Root cause: MessengerTravelDuration/TradeTravelDuration/returnDuration
// (this file, recall.go) computed the WALL-CLOCK arrives_at DISPLAY column by
// assuming "1 game hour (tick) = 1 real hour" — hardcoded time.Hour math that
// ignores tick.TickMinutes. That assumption only holds at the default 60
// min/tick cadence; on any world running faster (e.g. TICK_MINUTES=1 on the
// CT 126 dev server), the ACTUAL delivery is driven by ticks (fires in
// minutes), while the stored arrives_at/expires_at column — computed with the
// stale real-hour formula — sits hours in the future. Once the CLI compares
// that stale future timestamp to true wall-clock now, time.Since() goes
// negative. Every other tick→wall-clock display conversion in the codebase
// (build_queue.complete_at, rite cooldowns — api/handlers/{settlement,province}.go)
// already scales through tick.TickMinutes; these three functions were the
// outliers still using the pre-tick-substrate (mig 067) hour-based formula.

import (
	"testing"
	"time"

	"github.com/poleia/server/internal/tick"
)

// withTickMinutes overrides the package-level tick.TickMinutes for the
// duration of a test and restores it after — a plain var (read once from
// TICK_MINUTES at init), safe to swap in-process.
func withTickMinutes(t *testing.T, minutes int) {
	t.Helper()
	orig := tick.TickMinutes
	tick.TickMinutes = minutes
	t.Cleanup(func() { tick.TickMinutes = orig })
}

// TestMessengerTravelDuration_MatchesTicksAtFastCadence is the load-bearing
// case: on a sped-up world (TICK_MINUTES=1, as configured on the CT 126 dev
// server), the display duration must equal the SAME number of ticks used for
// actual scheduling (MessengerTravelTicks), each worth 1 real minute — not 1
// real hour. Before the fix this returned 60x too much wall-clock time.
func TestMessengerTravelDuration_MatchesTicksAtFastCadence(t *testing.T) {
	withTickMinutes(t, 1)
	dist := 5
	wantTicks := MessengerTravelTicks(dist)
	want := time.Duration(wantTicks) * time.Minute
	if got := MessengerTravelDuration(dist); got != want {
		t.Errorf("MessengerTravelDuration(%d) = %v, want %v (%d ticks × 1 min/tick)", dist, got, want, wantTicks)
	}
}

func TestTradeTravelDuration_MatchesTicksAtFastCadence(t *testing.T) {
	withTickMinutes(t, 1)
	dist := 5
	wantTicks := TradeTravelTicks(dist)
	want := time.Duration(wantTicks) * time.Minute
	if got := TradeTravelDuration(dist); got != want {
		t.Errorf("TradeTravelDuration(%d) = %v, want %v (%d ticks × 1 min/tick)", dist, got, want, wantTicks)
	}
}

func TestReturnDuration_MatchesTicksAtFastCadence(t *testing.T) {
	withTickMinutes(t, 1)
	dist := 5
	terrain := "plains"
	wantTicks := returnTicks(dist, terrain)
	want := time.Duration(wantTicks) * time.Minute
	if got := returnDuration(dist, terrain); got != want {
		t.Errorf("returnDuration(%d, %q) = %v, want %v (%d ticks × 1 min/tick)", dist, terrain, got, want, wantTicks)
	}
}

// TestMessengerTravelDuration_MatchesDefaultCadence pins the historical
// (correct-by-coincidence) case: at the default 60 min/tick, 1 tick really is
// 1 real hour, so the fix must not change behaviour for the common case.
func TestMessengerTravelDuration_MatchesDefaultCadence(t *testing.T) {
	withTickMinutes(t, 60)
	dist := 5
	want := time.Duration(MessengerTravelTicks(dist)) * time.Hour
	if got := MessengerTravelDuration(dist); got != want {
		t.Errorf("MessengerTravelDuration(%d) at default cadence = %v, want %v", dist, got, want)
	}
}
