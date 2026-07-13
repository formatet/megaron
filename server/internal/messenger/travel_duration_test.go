package messenger

// Regression test for Fas 1d: negative ETAs/"ago" in the CLI (e.g. a trade
// offer's ETA before its own send time, "-145m ago" on a delivered message).
//
// Root cause: MessengerTravelDuration/TradeTravelDuration/returnDuration
// (this file, recall.go) computed the WALL-CLOCK arrives_at DISPLAY column by
// assuming "1 game hour (tick) = 1 real hour" — hardcoded time.Hour math that
// ignores the real tick cadence. That assumption only holds at the default 60
// min/tick cadence; on any world running faster (e.g. TICK_MINUTES=1 on the
// CT 126 dev server), the ACTUAL delivery is driven by ticks (fires in
// minutes), while the stored arrives_at/expires_at column — computed with the
// stale real-hour formula — sits hours in the future. Once the CLI compares
// that stale future timestamp to true wall-clock now, time.Since() goes
// negative.
//
// Fas A Run 2: these three functions now convert through tick.RealUntil
// (package tick's TickSeconds), not the deprecated TickMinutes var — it
// floors to 1 minute and silently produces coarse (or, on a sub-minute
// TICK_SECONDS cadence like the CT 126 dev server's TICK_SECONDS=6, 10x too
// long) display durations. See internal/tick/eta.go.

import (
	"testing"
	"time"

	"github.com/poleia/server/internal/tick"
)

// withTickSeconds overrides the package-level tick.TickSeconds for the
// duration of a test and restores it after — a plain var (read once from
// TICK_SECONDS/TICK_MINUTES at init), safe to swap in-process.
func withTickSeconds(t *testing.T, seconds int) {
	t.Helper()
	orig := tick.TickSeconds
	tick.TickSeconds = seconds
	t.Cleanup(func() { tick.TickSeconds = orig })
}

// TestMessengerTravelDuration_MatchesTicksAtFastCadence is the load-bearing
// case: on a sped-up world (1 real minute/tick, as configured on the CT 126
// dev server before the TICK_SECONDS cadence), the display duration must
// equal the SAME number of ticks used for actual scheduling
// (MessengerTravelTicks), each worth 1 real minute — not 1 real hour. Before
// the fix this returned 60x too much wall-clock time.
func TestMessengerTravelDuration_MatchesTicksAtFastCadence(t *testing.T) {
	withTickSeconds(t, 60)
	dist := 5
	wantTicks := MessengerTravelTicks(dist)
	want := time.Duration(wantTicks) * time.Minute
	if got := MessengerTravelDuration(dist); got != want {
		t.Errorf("MessengerTravelDuration(%d) = %v, want %v (%d ticks × 1 min/tick)", dist, got, want, wantTicks)
	}
}

func TestTradeTravelDuration_MatchesTicksAtFastCadence(t *testing.T) {
	withTickSeconds(t, 60)
	dist := 5
	wantTicks := TradeTravelTicks(dist)
	want := time.Duration(wantTicks) * time.Minute
	if got := TradeTravelDuration(dist); got != want {
		t.Errorf("TradeTravelDuration(%d) = %v, want %v (%d ticks × 1 min/tick)", dist, got, want, wantTicks)
	}
}

func TestReturnDuration_MatchesTicksAtFastCadence(t *testing.T) {
	withTickSeconds(t, 60)
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
	withTickSeconds(t, 3600)
	dist := 5
	want := time.Duration(MessengerTravelTicks(dist)) * time.Hour
	if got := MessengerTravelDuration(dist); got != want {
		t.Errorf("MessengerTravelDuration(%d) at default cadence = %v, want %v", dist, got, want)
	}
}

// TestMessengerTravelDuration_ExactAtSubMinuteCadence is the Fas A Run 2
// load-bearing case: on TICK_SECONDS=6 (the CT 126 dev cadence), the derived
// duration must be the EXACT number of seconds — never floored to whole
// minutes the way the retired TickMinutes conversion would (6s/tick floors
// to "1 minute/tick", 10x too long).
func TestMessengerTravelDuration_ExactAtSubMinuteCadence(t *testing.T) {
	withTickSeconds(t, 6)
	dist := 5
	wantTicks := MessengerTravelTicks(dist)
	want := time.Duration(wantTicks) * 6 * time.Second
	if got := MessengerTravelDuration(dist); got != want {
		t.Errorf("MessengerTravelDuration(%d) at TICK_SECONDS=6 = %v, want %v exact (%d ticks × 6s)", dist, got, want, wantTicks)
	}
}

func TestTradeTravelDuration_ExactAtSubMinuteCadence(t *testing.T) {
	withTickSeconds(t, 6)
	dist := 5
	wantTicks := TradeTravelTicks(dist)
	want := time.Duration(wantTicks) * 6 * time.Second
	if got := TradeTravelDuration(dist); got != want {
		t.Errorf("TradeTravelDuration(%d) at TICK_SECONDS=6 = %v, want %v exact (%d ticks × 6s)", dist, got, want, wantTicks)
	}
}

func TestReturnDuration_ExactAtSubMinuteCadence(t *testing.T) {
	withTickSeconds(t, 6)
	dist := 5
	terrain := "plains"
	wantTicks := returnTicks(dist, terrain)
	want := time.Duration(wantTicks) * 6 * time.Second
	if got := returnDuration(dist, terrain); got != want {
		t.Errorf("returnDuration(%d, %q) at TICK_SECONDS=6 = %v, want %v exact (%d ticks × 6s)", dist, terrain, got, want, wantTicks)
	}
}
