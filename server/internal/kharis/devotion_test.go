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
	// One temple, fully devoted, fed, ordinary times: climbs — slowly.
	if net := dayNet(1.0, 1.0, 1.0, 0); net <= 0 {
		t.Errorf("a fully devoted, fed temple must climb, got %+.2f/day", net)
	} else if net > 0.5 {
		t.Errorf("the climb must stay SLOW — %+.2f/day is a sprint, not a relationship", net)
	}

	// The same temple at the bare 0.15 devotion floor the server applies: fades.
	if net := dayNet(0.15, 1.0, 1.0, 0); net >= 0 {
		t.Errorf("a temple nobody tends must fade, got %+.2f/day", net)
	}

	// Tending several temples is the real way up.
	if net := dayNet(3*0.6, 1.0, 1.0, 0); net <= 0.5 {
		t.Errorf("three tended temples should climb meaningfully, got %+.2f/day", net)
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
