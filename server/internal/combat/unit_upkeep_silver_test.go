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

func TestUnitUpkeep_HomeWages(t *testing.T) {
	cases := []struct {
		name       string
		unitType   string
		category   string
		size       int
		status     string
		wantSilver float64
	}{
		{"spearman garrison", "spearman", "land", 100, "garrison", 0},
		{"spearman marching — status never changes silver", "spearman", "land", 100, "marching", 0},
		{"elite_infantry garrison", "elite_infantry", "land", 100, "garrison", 0.75},
		{"war_chariot garrison", "war_chariot", "land", 100, "garrison", 1},
		{"galley garrison", "galley", "naval", 1, "garrison", 0.1},
		{"war_galley garrison", "war_galley", "naval", 1, "garrison", 0.25},
		{"merchantman garrison", "merchantman", "naval", 1, "garrison", 0.05},
	}
	const eps = 1e-9
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			up := UnitUpkeep(tc.unitType, tc.category, tc.size, tc.status, rationCrewForTest(tc.unitType))
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
	got := UnitUpkeep("spearman", "land", 100, "marching", 0).Silver
	if got != 0 {
		t.Errorf(`UnitUpkeep("spearman","land",100,"marching").Silver = %v, want 0`, got)
	}
}
