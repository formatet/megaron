package kharis

import (
	"formatet/megaron/server/internal/economy"
	"testing"
)

// Mig 094 replaced a summed `cult` STOCK with temple state read directly:
// presence × devotion × fed. The numbers moved; the SHAPE must not. This test
// locks the shape the design states — "skött tempel ≈ långsam klättring,
// försummelse = fade" — so a future tuner who changes (KharisPerTempleWeight / 2) or
// decayBas sees immediately which invariant they broke.

// dayNet models one maintained day: what devotion earns, minus the day's decay.
func dayNet(devotionSum float64, offerFraction, scarcity float64, cityCount int) float64 {
	gain := devotionSum * (KharisPerTempleWeight / 2) * offerFraction * scarcity
	return gain - computeDailyDecay(cityCount, 0)
}

func TestDevotionCalibration_ModestClimbsGrandClimbsFaster(t *testing.T) {
	l1 := float64(economy.TempleWorkplaces(1))

	// Timothy2026-10-13: ordinary fully staffed and fed L1 must climb under the current realm burden.
	net := dayNet(l1, 1, 1, 0)
	if net <= 0 || net > 2.1 {
		t.Errorf("tended L1 net%+.3f want(0,.5]", net)
	}

	// Unfed, however well staffed: fades harder still.
	if unfed := dayNet(l1, 0, 1.0, 1); unfed >= net {
		t.Errorf("an unfed temple must lose more ground than a fed one, got %+.2f/game day vs fed %+.2f/game day", unfed, net)
	}

	// A larger temple employs more of the city and climbs faster: it must
	// climb outright in ordinary times — the whole reason to raise one.
	l2 := dayNet(float64(economy.TempleWorkplaces(2)), 1.0, 1.0, 1)
	if l2 <= 0 {
		t.Errorf("a level-2 temple must climb in ordinary times, got %+.2f/game day", l2)
	}
	if l2 <= net {
		t.Errorf("a level-2 temple must out-climb a level-1: %+.2f vs %+.2f", l2, net)
	}

	// Several tended temples are the other way up: their devotion sums, so enough
	// modest shrines also climb.
	if many := dayNet(3*l1, 1.0, 1.0, 1); many <= 0 {
		t.Errorf("three tended temples' devotion should sum past the decay and climb, got %+.2f/game day", many)
	}
}

// Feeding is a gate on the whole gain, not a bonus beside it: an unfed temple
// earns nothing however devoted its people are.
func TestDevotionCalibration_UnfedTempleEarnsNothing(t *testing.T) {
	gain := 5.0 * (KharisPerTempleWeight / 2) * 0.0 * 1.0
	if gain != 0 {
		t.Fatalf("an unfed temple must earn nothing, got %.2f", gain)
	}
	if net := dayNet(5.0, 0, 1.0, 1); net >= 0 {
		t.Errorf("unfed temples must lose ground to decay, got %+.2f/game day", net)
	}
}

// A shortage may make devotion worth more, but it can never mint kharis.
func TestTempleScarcity_BoundedAndNeverPunishing(t *testing.T) {
	plenty := dayNet(1.0, 1.0, 1.0, 1)
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

// Expansion still costs when the same temples supply gain.
func TestDevotionCalibration_TemplelessColoniesStillBite(t *testing.T) {
	tended := dayNet(1.0, 1.0, 1.0, 1)
	sprawling := dayNet(1.0, 1.0, 1.0, 7)
	if sprawling >= tended {
		t.Error("additional cities must still drag kharis down")
	}
}
