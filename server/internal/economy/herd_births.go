package economy

import "math"

// The herd breeds itself (order 33, Timothy 2026-10-19). Deterministic, no
// dice: every tenth animal gives one calf per birth day. On a tick divisible by
// HerdFedBirthEvery (5) the calves the city's grain can feed are born, each
// costing 1 grain; on a tick divisible by HerdGrazeBirthEvery (20) ALL calves
// are born, the ones grain cannot feed living on pasture alone. Grain buys no
// more calves than the pen has room for (Timothy 2026-10-30): a bought calf
// the pen cannot hold would be lost after the meal, its grain wasted.
const (
	HerdFedBirthEvery   = 5
	HerdGrazeBirthEvery = 20
	HerdPerCalf         = 10
)

// HerdBirths returns the calves born on tick (born) and how many of them are
// paid for with grain (fed, 1 grain each, fed <= born, fed <= room under herdCap).
func HerdBirths(herd, grain, herdCap float64, tick int) (born, fed int) {
	if tick%HerdFedBirthEvery != 0 {
		return 0, 0
	}
	calves := int(math.Floor(math.Max(0, herd) / HerdPerCalf))
	fed = calves
	if g := int(math.Floor(math.Max(0, grain))); g < fed {
		fed = g
	}
	if room := int(math.Floor(math.Max(0, herdCap-math.Max(0, herd)))); room < fed {
		fed = room
	}
	born = fed
	if tick%HerdGrazeBirthEvery == 0 {
		born = calves
	}
	return born, fed
}

// NextHerdBirth is the first tick after now on which HerdBirths gives calves,
// with its born/fed; at is 0 when none come within a grazing cycle.
func NextHerdBirth(herd, grain, herdCap float64, now int) (born, fed, at int) {
	for t := now + 1; t <= now+HerdGrazeBirthEvery; t++ {
		if b, f := HerdBirths(herd, grain, herdCap, t); b > 0 {
			return b, f, t
		}
	}
	return 0, 0, 0
}
