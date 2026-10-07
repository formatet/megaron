package messenger

import (
	"testing"
	"time"
)

// These pin the default cadence (3600 real seconds/tick = 60 min/tick)
// explicitly rather than relying on ambient TICK_SECONDS/TICK_MINUTES, since
// MessengerTravelDuration now converts
// through tick.RealUntil (Fas A Run 2, travel_duration_test.go) instead of a
// hardcoded real-hour.

func TestMessengerTravelDuration(t *testing.T) {
	withTickSeconds(t, 3600)
	cases := []struct {
		dist int
		want time.Duration
	}{
		{0, time.Hour},      // floors to 1 tick (MessengerTravelTicks' own floor) = 1h at default cadence
		{4, 2 * time.Hour},  // 4 hexes × 0.5 h/hex = 2 ticks
		{10, 5 * time.Hour}, // 10 hexes × 0.5 h/hex = 5 ticks
	}
	for _, c := range cases {
		if got := MessengerTravelDuration(c.dist); got != c.want {
			t.Errorf("MessengerTravelDuration(%d) = %v, want %v", c.dist, got, c.want)
		}
	}
}
