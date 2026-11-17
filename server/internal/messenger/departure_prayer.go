package messenger

import (
	"formatet/megaron/server/internal/tick"
	"time"
)

// RunnerArrival is the wall-clock time a Runner sent now reaches its due tick.
func RunnerArrival(now time.Time, current, due int) time.Time {
	if due <= current {
		return now
	}
	return now.Add(tick.RealUntil(due, current))
}
