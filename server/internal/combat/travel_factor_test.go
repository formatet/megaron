package combat

// Slice A (megaron_plan_skeppsfart_besattning.md §3): TravelFactor collects
// the five copies of the travel-time multiplier chain (dispatch, arrival,
// recall, redirect, damaged return) into one function. This test is the
// behaviour-neutrality proof: it checks TravelFactor's output against the
// LITERAL products of the known constants (0.6 war galley, 1.4 merchantman,
// 2.0 nomadic host, ×1.5 laden) — never against NavalSpeedFactor/
// MarchHoursFactorFor themselves, so a bug that crept into both the
// production code and this test at once would still be caught.

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
		if got := TravelFactor(c.utype, false); math.Abs(got-c.notLaden) > epsilon {
			t.Errorf("TravelFactor(%s, laden=false) = %v, want %v", c.utype, got, c.notLaden)
		}
		if got := TravelFactor(c.utype, true); math.Abs(got-c.laden) > epsilon {
			t.Errorf("TravelFactor(%s, laden=true) = %v, want %v", c.utype, got, c.laden)
		}
	}
}
