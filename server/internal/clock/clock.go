// Package clock provides the GameClock abstraction for Megaron.
// All time-dependent logic must go through Clock.Now() — never time.Now() directly.
// This enables deterministic tests and pause-aware wall time in production.
package clock

import (
	"sync"
	"time"
)

// Clock is the single source of time for all game logic.
type Clock interface {
	Now() time.Time
}

// WallClock is the production clock. It adjusts for server downtime by tracking
// a pause offset: gaps longer than PauseThreshold between heartbeats are subtracted
// from wall time so that offline periods don't advance the game clock.
type WallClock struct {
	mu             sync.RWMutex
	pausedDuration time.Duration
}

// PauseThreshold is the minimum gap between heartbeats before a period is
// considered a server pause rather than normal jitter.
const PauseThreshold = 30 * time.Second

// NewWallClock creates a WallClock. Call RecordDowntime on startup with the
// duration of the last detected gap before using.
func NewWallClock() *WallClock {
	return &WallClock{}
}

// RecordDowntime adds a detected offline gap to the pause offset.
// Call once at startup with the gap since the last heartbeat.
func (c *WallClock) RecordDowntime(gap time.Duration) {
	if gap <= PauseThreshold {
		return
	}
	c.mu.Lock()
	c.pausedDuration += gap
	c.mu.Unlock()
}

// Now returns wall time minus accumulated pause duration.
func (c *WallClock) Now() time.Time {
	c.mu.RLock()
	paused := c.pausedDuration
	c.mu.RUnlock()
	return time.Now().Add(-paused)
}

// TestClock is a manually controllable clock for use in tests.
// The zero value starts at the zero time; call Set or Advance before use.
type TestClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewTestClock creates a TestClock set to a fixed reference time.
func NewTestClock(t time.Time) *TestClock {
	return &TestClock{now: t}
}

// Now returns the current test time.
func (c *TestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Set moves the clock to an absolute time.
func (c *TestClock) Set(t time.Time) {
	c.mu.Lock()
	c.now = t
	c.mu.Unlock()
}

// Advance moves the clock forward by d.
func (c *TestClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}
