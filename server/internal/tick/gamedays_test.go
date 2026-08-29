package tick

import (
	"testing"
	"time"
)

// TestGameDaysLeft_RoundsUp pins the rule a cooldown message lives or dies by:
// a wait with ANY time left on it must never render as "0". The rite cooldown
// printed "on cooldown for another 0 minutes" while still refusing the rite —
// a refusal contradicting its own reason, the same class as cli-sanning row D.
func TestGameDaysLeft_RoundsUp(t *testing.T) {
	old := TickSeconds
	TickSeconds = 3600 // one tick = one hour of real time = one game-day
	defer func() { TickSeconds = old }()

	cases := []struct {
		name string
		d    time.Duration
		want int
	}{
		{"a whole tick is one game-day", time.Hour, 1},
		{"one second left still costs a whole day", time.Second, 1},
		{"just under two ticks rounds up to two", 2*time.Hour - time.Second, 2},
		{"exactly two ticks is two", 2 * time.Hour, 2},
		{"a full 24-tick cooldown", 24 * time.Hour, 24},
		{"nothing left is zero", 0, 0},
		{"already elapsed is zero, never negative", -time.Hour, 0},
	}
	for _, c := range cases {
		if got := GameDaysLeft(c.d); got != c.want {
			t.Errorf("%s: GameDaysLeft(%v) = %d, want %d", c.name, c.d, got, c.want)
		}
	}
}

// TestGameDaysLeft_HonoursTickCadence proves the helper reads the world's real
// cadence rather than assuming one. The acceptance rig runs TICK_SECONDS=6; a
// helper hard-coded to an hour would report 1 game-day for a 24-tick cooldown
// there and quietly mis-inform every dev-world reading.
func TestGameDaysLeft_HonoursTickCadence(t *testing.T) {
	old := TickSeconds
	defer func() { TickSeconds = old }()

	TickSeconds = 6
	if got := GameDaysLeft(24 * 6 * time.Second); got != 24 {
		t.Errorf("at TICK_SECONDS=6, GameDaysLeft(144s) = %d, want 24", got)
	}
	TickSeconds = 3600
	if got := GameDaysLeft(24 * 6 * time.Second); got != 1 {
		t.Errorf("at TICK_SECONDS=3600, GameDaysLeft(144s) = %d, want 1 (part of a tick still costs one)", got)
	}
}

// TestFormatGameDays_Singular — keryx shipped "arrives in 1 game-days" until
// rad K caught it. A plural on 1 reads as a bug to the player.
func TestFormatGameDays_Singular(t *testing.T) {
	cases := map[int]string{0: "0 game-days", 1: "1 game-day", 2: "2 game-days", 24: "24 game-days"}
	for in, want := range cases {
		if got := FormatGameDays(in); got != want {
			t.Errorf("FormatGameDays(%d) = %q, want %q", in, got, want)
		}
	}
}
