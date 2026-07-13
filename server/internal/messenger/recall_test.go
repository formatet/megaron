package messenger

import (
	"testing"
	"time"
)

// These pin the default cadence (3600 real seconds/tick = 60 min/tick)
// explicitly rather than relying on ambient TICK_SECONDS/TICK_MINUTES, since
// MessengerTravelDuration/TradeTravelDuration/returnDuration now convert
// through tick.RealUntil (Fas A Run 2, travel_duration_test.go) instead of a
// hardcoded real-hour.

func TestMessengerTravelDuration(t *testing.T) {
	withTickSeconds(t, 3600)
	cases := []struct {
		dist int
		want time.Duration
	}{
		{0, time.Hour},       // floors to 1 tick (MessengerTravelTicks' own floor) = 1h at default cadence
		{4, 2 * time.Hour},   // 4 hexes × 0.5 h/hex = 2 ticks
		{10, 5 * time.Hour},  // 10 hexes × 0.5 h/hex = 5 ticks
	}
	for _, c := range cases {
		if got := MessengerTravelDuration(c.dist); got != c.want {
			t.Errorf("MessengerTravelDuration(%d) = %v, want %v", c.dist, got, c.want)
		}
	}
}

func TestTradeTravelDuration(t *testing.T) {
	withTickSeconds(t, 3600)
	if got := TradeTravelDuration(6); got != 3*time.Hour { // 6 × 0.5 h/hex = 3 ticks
		t.Errorf("TradeTravelDuration(6) = %v, want 3h", got)
	}
}

func TestReturnDurationFloor(t *testing.T) {
	withTickSeconds(t, 3600)
	// Zero distance still floors to 1 tick (returnTicks' own floor) — 1h at
	// default cadence, not an arbitrary sub-tick "6 minutes": actual delivery
	// can never complete faster than 1 tick, so displaying less would just be
	// the same tick/wall-clock mismatch this fix removes, in the other direction.
	got := returnDuration(0, "plains")
	if got != time.Hour {
		t.Errorf("returnDuration(0) = %v, want 1h (1-tick floor at default cadence)", got)
	}
	// Any real distance clears the floor.
	if d := returnDuration(5, "plains"); d <= time.Hour {
		t.Errorf("returnDuration(5) = %v, want > 1h floor", d)
	}
}
