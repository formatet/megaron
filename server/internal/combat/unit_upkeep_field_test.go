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
		// Grain ÷100 (mig 136, UpkeepSpecs' own ÷100 calibration): 50→0.5,
		// 100→1.0, 4→0.04, 60→0.6, 120→1.2, 80→0.8, 160→1.6. Silver untouched.
		{"garrison spearman full size", "spearman", "land", 100, "garrison", 0.5, 1},
		{"marching spearman full size — silver unchanged", "spearman", "land", 100, "marching", 1.0, 1},
		{"positioned spearman half size", "spearman", "land", 50, "positioned", 0.5, 0.5},
		{"garrison galley — naval never doubles", "galley", "naval", 1, "garrison", 0.04, 1.5},
		{"marching galley — naval never doubles", "galley", "naval", 1, "marching", 0.04, 1.5},
		{"garrison elite_infantry full size", "elite_infantry", "land", 100, "garrison", 0.6, 2},
		{"marching elite_infantry full size", "elite_infantry", "land", 100, "marching", 1.2, 2},
		{"garrison war_chariot full size", "war_chariot", "land", 100, "garrison", 0.8, 3},
		{"marching war_chariot full size", "war_chariot", "land", 100, "marching", 1.6, 3},
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
