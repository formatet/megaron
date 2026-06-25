package tick

import (
	"testing"
	"time"

	"github.com/poleia/server/internal/clock"
)

// TestTicksDue verifies the pure timing calculation: one tick per TICK_MINUTES,
// catch-up returns the correct count without jumping.
func TestTicksDue(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tickDur := 60 * time.Minute

	cases := []struct {
		name        string
		clockOffset time.Duration // how far ahead clock.Now() is from base
		lastTickAt  time.Time
		want        int
	}{
		{
			name:        "no tick due — half a period elapsed",
			clockOffset: 30 * time.Minute,
			lastTickAt:  base,
			want:        0,
		},
		{
			name:        "exactly one period — one tick due",
			clockOffset: 60 * time.Minute,
			lastTickAt:  base,
			want:        1,
		},
		{
			name:        "one period plus one second — still one tick due",
			clockOffset: 61 * time.Minute,
			lastTickAt:  base,
			want:        1,
		},
		{
			name:        "catch-up: three periods elapsed",
			clockOffset: 3 * 60 * time.Minute,
			lastTickAt:  base,
			want:        3,
		},
		{
			name:        "catch-up: five periods (server was down)",
			clockOffset: 5 * 60 * time.Minute,
			lastTickAt:  base,
			want:        5,
		},
		{
			name:        "zero elapsed — no tick",
			clockOffset: 0,
			lastTickAt:  base,
			want:        0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clk := clock.NewTestClock(base.Add(tc.clockOffset))
			got := ticksDue(clk.Now(), tc.lastTickAt, tickDur)
			if got != tc.want {
				t.Errorf("ticksDue = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestCatchUpDeterministic verifies that catch-up advances one tick at a time
// (no jumps) by simulating the last_tick_at bookkeeping.
func TestCatchUpDeterministic(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tickDur := 60 * time.Minute

	// Simulate 5 ticks of downtime: clock is 5 periods ahead of lastTickAt.
	clk := clock.NewTestClock(base.Add(5 * tickDur))
	lastTickAt := base
	currentTick := 0

	// Simulate the catch-up loop: each iteration advances one tick.
	for {
		due := ticksDue(clk.Now(), lastTickAt, tickDur)
		if due == 0 {
			break
		}
		// Worker advances exactly 1 tick per TX, bumps lastTickAt by tickDur.
		currentTick++
		lastTickAt = lastTickAt.Add(tickDur)
	}

	if currentTick != 5 {
		t.Errorf("catch-up advanced %d ticks, want 5", currentTick)
	}
	// After catch-up, no more ticks are due.
	if due := ticksDue(clk.Now(), lastTickAt, tickDur); due != 0 {
		t.Errorf("ticks still due after catch-up: %d, want 0", due)
	}
}

// TestTicksDueEdge verifies that exactly one tick fires per period boundary,
// not two — the "no jump" invariant.
func TestTicksDueEdge(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tickDur := 60 * time.Minute

	// After exactly N periods, N ticks due (not N+1, not N-1).
	for n := 1; n <= 10; n++ {
		clk := clock.NewTestClock(base.Add(time.Duration(n) * tickDur))
		got := ticksDue(clk.Now(), base, tickDur)
		if got != n {
			t.Errorf("after %d periods: ticksDue = %d, want %d", n, got, n)
		}
	}
}
