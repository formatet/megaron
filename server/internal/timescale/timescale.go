// Package timescale provides a global time-compression factor for playtesting.
// Set TIME_SCALE=10 to make all in-game durations run 10× faster.
// Default is 1.0 (real time). Only used in duration scheduling — never affects
// clock.Now() itself, so timestamps in the DB remain wall-clock correct.
package timescale

import (
	"os"
	"strconv"
	"time"
)

// Factor is the global speed multiplier, read once from TIME_SCALE at startup.
// Factor=10 → all durations 10× shorter.
var Factor = readFactor()

func readFactor() float64 {
	if v := os.Getenv("TIME_SCALE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 1 {
			return f
		}
	}
	return 1.0
}

// Apply divides d by Factor. Use this wherever a scheduled-event duration is computed.
func Apply(d time.Duration) time.Duration {
	if Factor == 1.0 {
		return d
	}
	return time.Duration(float64(d) / Factor)
}
