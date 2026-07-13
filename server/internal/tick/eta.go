package tick

import (
	"time"

	"github.com/poleia/server/internal/clock"
)

// RealUntil returns the exact real-time duration remaining until dueTick,
// measured from currentTick, at the runtime tick cadence (TickSeconds — NOT
// TickMinutes, which floors to 1 minute and silently produces coarse or
// wrong ETAs on a sub-minute TICK_SECONDS dev cadence, e.g. TICK_SECONDS=6).
// Floored at zero: a due tick that has already passed (or equals the current
// tick) never yields a negative duration.
//
// For sites that only have a relative remaining-tick count (not an absolute
// dueTick), RealUntil(restTicks, 0) is equivalent to
// RealUntil(currentTick+restTicks, currentTick) and needs no currentTick.
func RealUntil(dueTick, currentTick int) time.Duration {
	remaining := dueTick - currentTick
	if remaining < 0 {
		remaining = 0
	}
	return time.Duration(remaining) * time.Duration(TickSeconds) * time.Second
}

// EtaAt returns the wall-clock instant dueTick is expected to fire, given the
// world's currentTick and clk as the time source. This is the ETA-display
// helper: RealUntil converted to an absolute time.Time via clk.Now().
func EtaAt(clk clock.Clock, dueTick, currentTick int) time.Time {
	return clk.Now().Add(RealUntil(dueTick, currentTick))
}
