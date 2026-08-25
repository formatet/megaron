package combat

// Slice A (megaron_plan_skeppsfart_besattning.md §3): TravelFactor collects
// the five copies of the travel-time multiplier chain (dispatch, arrival,
// recall, redirect, damaged return) into one function. This test is the
// behaviour-neutrality proof: it checks TravelFactor's output against the
// LITERAL products of the known constants (0.6 war galley, 1.4 merchantman,
// 2.0 nomadic host, ×1.5 laden) — never against NavalSpeedFactor/
// MarchHoursFactorFor themselves, so a bug that crept into both the
// production code and this test at once would still be caught.
//
// Slice B (§4) added the crew term to TravelFactor's signature. Every case
// here passes crew = unit.CrewFor(utype) — a FULLY crewed unit — so this
// table doubles as Slice B's AC2 ("fullbemannat är oförändrat"): the literal
// products above must still hold once CrewSpeedFactor is in the multiply
// chain, because a full crew must always resolve to a 1.0 factor.

import (
	"math"
	"testing"

	"formatet/megaron/server/internal/unit"
)

func TestTravelFactor_MatchesLiteralProducts(t *testing.T) {
	cases := []struct {
		utype       unit.Type
		notLaden    float64
		laden       float64
	}{
		// Land units and the plain "galley" type: NavalSpeedFactor's default
		// case (1.0) × MarchHoursFactorFor's default case (1.0).
		{unit.TypeSpearman, 1.0, 1.5},
		{unit.TypeEliteInfantry, 1.0, 1.5},
		{unit.TypeWarChariot, 1.0, 1.5},
		{unit.TypeGalley, 1.0, 1.5},
		// War galley: NavalSpeedFactor 0.6 × MarchHoursFactorFor 1.0.
		{unit.TypeWarGalley, 0.6, 0.9},
		// Merchantman: NavalSpeedFactor 1.4 × MarchHoursFactorFor 1.0.
		{unit.TypeMerchantman, 1.4, 2.1},
		// Nomadic host: NavalSpeedFactor 1.0 (default) × MarchHoursFactorFor 2.0.
		{unit.TypeNomadicHost, 2.0, 3.0},
	}

	const epsilon = 1e-9
	for _, c := range cases {
		fullCrew := unit.CrewFor(c.utype)
		if got := TravelFactor(c.utype, fullCrew, false); math.Abs(got-c.notLaden) > epsilon {
			t.Errorf("TravelFactor(%s, crew=%d (full), laden=false) = %v, want %v", c.utype, fullCrew, got, c.notLaden)
		}
		if got := TravelFactor(c.utype, fullCrew, true); math.Abs(got-c.laden) > epsilon {
			t.Errorf("TravelFactor(%s, crew=%d (full), laden=true) = %v, want %v", c.utype, fullCrew, got, c.laden)
		}
	}
}
