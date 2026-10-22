package combat

// UnitUpkeep is the single source of truth for per-unit upkeep scaling, shared by
// the charging loop (Handle) and the army read surface. These cases pin the scaling
// contract: land scales with size/100, naval food uses actual crew and silver uses actual crew, unknown = 0.

import (
	"math"
	"testing"
)

func TestUnitUpkeep(t *testing.T) {
	cases := []struct {
		name       string
		unitType   string
		category   string
		size       int
		wantGrain  float64
		wantSilver float64
	}{
		// Grain ÷100 (mig 136, UpkeepSpecs' own calibration — not grain's ÷43.2,
		// see upkeep.go's comment): 50→0.50, 70.5→0.705, 30→0.30, naval food now derives from actual rowers.
		// Silver is untouched (currency, divisor 1).
		{"land spearman full size", "spearman", "land", 100, 0.50, 0},
		{"land spearman 141 scales up", "spearman", "land", 141, 0.705, 0},
		{"land elite half size", "elite_infantry", "land", 50, 0.30, 0.375},
		{"naval galley crew ration independent of hull size at size 1", "galley", "naval", 1, 0.10, 0.1},
		{"naval galley crew ration independent of hull size even if size>1", "galley", "naval", 5, 0.10, 0.1},
		{"naval war_galley crew ration", "war_galley", "naval", 3, 0.25, 0.25},
		{"unknown type costs nothing", "slinger", "land", 100, 0, 0},
	}
	const eps = 1e-9
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			up := UnitUpkeep(tc.unitType, tc.category, tc.size, "garrison", rationCrewForTest(tc.unitType))
			if math.Abs(up.Grain-tc.wantGrain) > eps {
				t.Errorf("Grain = %v, want %v", up.Grain, tc.wantGrain)
			}
			if math.Abs(up.Silver-tc.wantSilver) > eps {
				t.Errorf("Silver = %v, want %v", up.Silver, tc.wantSilver)
			}
		})
	}
}
