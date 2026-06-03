package messenger

import (
	"testing"
	"time"
)

func TestMessengerTravelDuration(t *testing.T) {
	cases := []struct {
		dist int
		want time.Duration
	}{
		{0, 0},
		{4, 2 * time.Hour},   // 4 hexes × 0.5 h/hex
		{10, 5 * time.Hour},  // 10 hexes × 0.5 h/hex
	}
	for _, c := range cases {
		if got := MessengerTravelDuration(c.dist); got != c.want {
			t.Errorf("MessengerTravelDuration(%d) = %v, want %v", c.dist, got, c.want)
		}
	}
}

func TestTradeTravelDuration(t *testing.T) {
	if got := TradeTravelDuration(6); got != 3*time.Hour { // 6 × 0.5 h/hex
		t.Errorf("TradeTravelDuration(6) = %v, want 3h", got)
	}
}

func TestReturnDurationFloor(t *testing.T) {
	// Zero distance still takes the 6-minute minimum (no instant teleport).
	got := returnDuration(0, "plains")
	if got < 5*time.Minute || got > 7*time.Minute {
		t.Errorf("returnDuration(0) = %v, want ~6m floor", got)
	}
	// Any real distance clears the floor.
	if d := returnDuration(5, "plains"); d <= 6*time.Minute {
		t.Errorf("returnDuration(5) = %v, want > 6m floor", d)
	}
}
