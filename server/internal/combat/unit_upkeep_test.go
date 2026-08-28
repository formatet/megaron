package combat

// UnitUpkeep is the single source of truth for per-unit upkeep scaling, shared by
// the charging loop (Handle) and the army read surface. These cases pin the scaling
// contract: land scales with size/100, naval is flat per vessel, unknown = 0.

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
		// see upkeep.go's comment): 50→0.50, 70.5→0.705, 30→0.30, 4→0.04, 6→0.06.
		// Silver is untouched (currency, divisor 1).
		{"land spearman full size", "spearman", "land", 100, 0.50, 1},
		{"land spearman 141 scales up", "spearman", "land", 141, 0.705, 1.41},
		{"land elite half size", "elite_infantry", "land", 50, 0.30, 1},
		{"naval galley flat at size 1", "galley", "naval", 1, 0.04, 1.5},
		{"naval galley flat even if size>1", "galley", "naval", 5, 0.04, 1.5},
		{"naval war_galley flat", "war_galley", "naval", 3, 0.06, 2.5},
		{"unknown type costs nothing", "slinger", "land", 100, 0, 0},
	}
	const eps = 1e-9
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			up := UnitUpkeep(tc.unitType, tc.category, tc.size, "garrison")
			if math.Abs(up.Grain-tc.wantGrain) > eps {
				t.Errorf("Grain = %v, want %v", up.Grain, tc.wantGrain)
			}
			if math.Abs(up.Silver-tc.wantSilver) > eps {
				t.Errorf("Silver = %v, want %v", up.Silver, tc.wantSilver)
			}
		})
	}
}
