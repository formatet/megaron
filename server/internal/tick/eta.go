package tick

import (
	"strconv"
	"time"

	"formatet/megaron/server/internal/clock"
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

// GameDaysLeft converts a remaining real-time duration back into the unit the
// player actually counts in: whole game-days, one per tick
// (megaron_plan_ticket_ar_dygnet — "ett tick ÄR dygnet").
//
// ⭐ Rounds UP, and that is the point: a wait with any time left on it must
// never render as "0". The rite cooldown printed
// "on cooldown for another 0 minutes" while still refusing the rite — a refusal
// that contradicts its own reason, the same class as cli-sanning row D.
//
// ⚠️ This is the inverse of RealUntil and exists because several player-facing
// surfaces hold only a duration, not a tick count. Where a tick count IS
// available, use it directly rather than round-tripping through wall time.
func GameDaysLeft(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	perTick := time.Duration(TickSeconds) * time.Second
	if perTick <= 0 {
		return 0
	}
	days := int(d / perTick)
	if d%perTick > 0 {
		days++
	}
	return days
}

// FormatGameDays renders a whole game-day count with the right number.
// Singular matters: keryx shipped "arrives in 1 game-days" until it was caught
// (rad K, cli-sanning), and a plural on 1 reads as a bug to the player.
func FormatGameDays(days int) string {
	if days == 1 {
		return "1 game-day"
	}
	return strconv.Itoa(days) + " game-days"
}

// EtaAt returns the wall-clock instant dueTick is expected to fire, given the
// world's currentTick and clk as the time source. This is the ETA-display
// helper: RealUntil converted to an absolute time.Time via clk.Now().
func EtaAt(clk clock.Clock, dueTick, currentTick int) time.Time {
	return clk.Now().Add(RealUntil(dueTick, currentTick))
}
