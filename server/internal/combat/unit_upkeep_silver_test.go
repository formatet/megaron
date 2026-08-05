package combat

// SLICE B (silverupkeep halveras, Timothy 2026-08-05): AG3 = lägre
// silverupkeep, formen "halvera hela tabellen". Grain is untouched — slice A
// set it. These cases pin the halved silver column, and confirm status never
// changes silver (a field unit's pay is the same as garrisoned — only grain
// doubles, per slice A).

import (
	"math"
	"testing"
)

func TestUnitUpkeep_SilverHalved(t *testing.T) {
	cases := []struct {
		name       string
		unitType   string
		category   string
		size       int
		status     string
		wantSilver float64
	}{
		{"spearman garrison", "spearman", "land", 100, "garrison", 1},
		{"spearman marching — status never changes silver", "spearman", "land", 100, "marching", 1},
		{"elite_infantry garrison", "elite_infantry", "land", 100, "garrison", 2},
		{"war_chariot garrison", "war_chariot", "land", 100, "garrison", 3},
		{"galley garrison", "galley", "naval", 1, "garrison", 1.5},
		{"war_galley garrison", "war_galley", "naval", 1, "garrison", 2.5},
		{"merchantman garrison", "merchantman", "naval", 1, "garrison", 1},
		{"priest costs nothing", "priest", "land", 100, "garrison", 0},
	}
	const eps = 1e-9
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			up := UnitUpkeep(tc.unitType, tc.category, tc.size, tc.status)
			if math.Abs(up.Silver-tc.wantSilver) > eps {
				t.Errorf("Silver = %v, want %v", up.Silver, tc.wantSilver)
			}
		})
	}
}

// TestUnitUpkeep_SpearmanSilverStatusIndependent locks the exact assertion
// the plan names: status never changes silver, checked directly rather than
// via the table above.
func TestUnitUpkeep_SpearmanSilverStatusIndependent(t *testing.T) {
	got := UnitUpkeep("spearman", "land", 100, "marching").Silver
	if got != 1 {
		t.Errorf(`UnitUpkeep("spearman","land",100,"marching").Silver = %v, want 1`, got)
	}
}
