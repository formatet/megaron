package combat

// UnitUpkeep is the single source of truth for per-unit upkeep scaling, shared by
// the charging loop (Handle) and the army read surface. These cases pin the scaling
// contract: land scales with size/100, naval is flat per vessel, priest/unknown = 0.

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
		{"land spearman full size", "spearman", "land", 100, 5, 2},
		{"land spearman 141 scales up", "spearman", "land", 141, 7.05, 2.82},
		{"land elite half size", "elite_infantry", "land", 50, 3, 2},
		{"naval galley flat at size 1", "galley", "naval", 1, 4, 3},
		{"naval galley flat even if size>1", "galley", "naval", 5, 4, 3},
		{"naval war_galley flat", "war_galley", "naval", 3, 6, 5},
		{"priest costs nothing", "priest", "land", 100, 0, 0},
		{"unknown type costs nothing", "slinger", "land", 100, 0, 0},
	}
	const eps = 1e-9
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			up := UnitUpkeep(tc.unitType, tc.category, tc.size)
			if math.Abs(up.Grain-tc.wantGrain) > eps {
				t.Errorf("Grain = %v, want %v", up.Grain, tc.wantGrain)
			}
			if math.Abs(up.Silver-tc.wantSilver) > eps {
				t.Errorf("Silver = %v, want %v", up.Silver, tc.wantSilver)
			}
		})
	}
}
