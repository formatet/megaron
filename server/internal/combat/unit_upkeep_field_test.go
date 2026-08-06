package combat

// SLICE A (soldatens föda, Timothy 2026-08-05): a soldier is a person. Garrison
// = a civilian's ration (pop×0.5/day, economy/recompute.go:359 is the anchor —
// UpkeepSpecs' land grain figures are calibrated to be exactly that at size
// 100). In the field ("marching"/"positioned") he eats double: mobilising and
// standing out cost more than standing home. Silver (sold) never changes with
// status. Naval upkeep is per hull, flat, and status never touches it.
//
// These cases pin the status contract on top of unit_upkeep_test.go's existing
// scaling contract (land size/100, naval flat, unknown = 0).

import (
	"math"
	"testing"
)

func TestUnitUpkeep_Status(t *testing.T) {
	cases := []struct {
		name       string
		unitType   string
		category   string
		size       int
		status     string
		wantGrain  float64
		wantSilver float64
	}{
		{"garrison spearman full size", "spearman", "land", 100, "garrison", 50, 1},
		{"marching spearman full size — silver unchanged", "spearman", "land", 100, "marching", 100, 1},
		{"positioned spearman half size", "spearman", "land", 50, "positioned", 50, 0.5},
		{"garrison galley — naval never doubles", "galley", "naval", 1, "garrison", 4, 1.5},
		{"marching galley — naval never doubles", "galley", "naval", 1, "marching", 4, 1.5},
		{"garrison elite_infantry full size", "elite_infantry", "land", 100, "garrison", 60, 2},
		{"marching elite_infantry full size", "elite_infantry", "land", 100, "marching", 120, 2},
		{"garrison war_chariot full size", "war_chariot", "land", 100, "garrison", 80, 3},
		{"marching war_chariot full size", "war_chariot", "land", 100, "marching", 160, 3},
		{"unknown type costs nothing regardless of status", "slinger", "land", 100, "marching", 0, 0},
	}
	const eps = 1e-9
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			up := UnitUpkeep(tc.unitType, tc.category, tc.size, tc.status)
			if math.Abs(up.Grain-tc.wantGrain) > eps {
				t.Errorf("Grain = %v, want %v", up.Grain, tc.wantGrain)
			}
			if math.Abs(up.Silver-tc.wantSilver) > eps {
				t.Errorf("Silver = %v, want %v", up.Silver, tc.wantSilver)
			}
		})
	}
}
