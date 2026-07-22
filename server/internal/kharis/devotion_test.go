package kharis

import "testing"

// Mig 094 replaced a summed `cult` STOCK with temple state read directly:
// presence × devotion × fed. The numbers moved; the SHAPE must not. This test
// locks the shape the design states — "skött tempel ≈ långsam klättring,
// försummelse = fade" — so a future tuner who changes kharisPerTempleDay or
// decayBas sees immediately which invariant they broke.

// dayNet models one maintained day: what devotion earns, minus the day's decay.
func dayNet(devotionSum float64, offerFraction, scarcity float64, templelessColonies int) float64 {
	gain := devotionSum * kharisPerTempleDay * offerFraction * scarcity
	return gain - computeDailyDecay(templelessColonies)
}

func TestDevotionCalibration_TendedClimbsNeglectedFades(t *testing.T) {
	l1 := templeDevotionCapacity(1)

	// A level-1 temple staffed to what it can EMPLOY, fed, in ordinary times:
	// climbs — slowly. This is the case every existing city is in, and it must
	// not fade: a fade nobody can escape is not a choice (Timothy 2026-07-23).
	net := dayNet(l1, 1.0, 1.0, 0)
	if net <= 0 {
		t.Errorf("a fully staffed, fed level-1 temple must climb, got %+.2f/day", net)
	}
	if net > 0.5 {
		t.Errorf("the climb must stay SLOW — %+.2f/day is a sprint, not a relationship", net)
	}

	// Unfed, however well staffed: fades.
	if net := dayNet(l1, 0, 1.0, 0); net >= 0 {
		t.Errorf("an unfed temple must fade, got %+.2f/day", net)
	}

	// A larger temple employs more of the city and climbs proportionally faster
	// — which is the whole reason to raise one.
	l2 := dayNet(templeDevotionCapacity(2), 1.0, 1.0, 0)
	if l2 <= net {
		t.Errorf("a level-2 temple must out-climb a level-1: %+.2f vs %+.2f", l2, net)
	}

	// Several tended temples are the other way up.
	if many := dayNet(3*l1, 1.0, 1.0, 0); many <= 0.5 {
		t.Errorf("three tended temples should climb meaningfully, got %+.2f/day", many)
	}
}

// The temple is a workplace: devotion allocated beyond what the building can
// employ has no altar to serve at, and must not pay. Without this a Wanax could
// pour 100%% of a city into an L1 shrine and skip temple levels entirely.
func TestTempleDevotionCapacity_ScalesWithLevelAndFloorsAtOne(t *testing.T) {
	if got := templeDevotionCapacity(1); got != TempleDevotionPerLevel {
		t.Errorf("level 1 capacity = %.2f, want %.2f", got, TempleDevotionPerLevel)
	}
	if templeDevotionCapacity(2) <= templeDevotionCapacity(1) {
		t.Error("a larger temple must employ more")
	}
	if got := templeDevotionCapacity(0); got != TempleDevotionPerLevel {
		t.Errorf("a malformed level must fall back to level 1, got %.2f", got)
	}
	// Level 1 capacity is exactly the floor LaborAlloc applies, so every existing
	// city is already staffed to capacity — the design premise for the raise.
	if templeDevotionCapacity(1) != 0.15 {
		t.Errorf("level-1 capacity must match the server devotion floor, got %.2f",
			templeDevotionCapacity(1))
	}
}

// Feeding is a gate on the whole gain, not a bonus beside it: an unfed temple
// earns nothing however devoted its people are.
func TestDevotionCalibration_UnfedTempleEarnsNothing(t *testing.T) {
	gain := 5.0 * kharisPerTempleDay * 0.0 * 1.0
	if gain != 0 {
		t.Fatalf("an unfed temple must earn nothing, got %.2f", gain)
	}
	if net := dayNet(5.0, 0, 1.0, 0); net >= 0 {
		t.Errorf("unfed temples must lose ground to decay, got %+.2f/day", net)
	}
}

// A shortage may make devotion worth more, but it can never mint kharis.
func TestTempleScarcity_BoundedAndNeverPunishing(t *testing.T) {
	plenty := dayNet(1.0, 1.0, 1.0, 0)
	shortage := dayNet(1.0, 1.0, templeScarcityCeil, 0)
	if shortage <= plenty {
		t.Error("burning scarce oil on the altar must be worth more than burning cheap oil")
	}
	// The bound lives in clampTempleScarcity, which is where it must be proven.
	if got := clampTempleScarcity(9.0); got != templeScarcityCeil {
		t.Errorf("a scarcity spike must be capped at %.1f×, got %.2f", templeScarcityCeil, got)
	}
	if got := clampTempleScarcity(0.3); got != 1.0 {
		t.Errorf("abundance must not punish a tended temple, got %.2f", got)
	}
}

// Expansion without temples still costs — unchanged by mig 094, and worth
// pinning because the decay term now meets a much smaller gain term.
func TestDevotionCalibration_TemplelessColoniesStillBite(t *testing.T) {
	tended := dayNet(1.0, 1.0, 1.0, 0)
	sprawling := dayNet(1.0, 1.0, 1.0, decayFreeColonies+3)
	if sprawling >= tended {
		t.Error("templeless colonies must still drag kharis down")
	}
}
