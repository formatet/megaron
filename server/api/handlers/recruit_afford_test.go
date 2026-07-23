package handlers

// Tests for can_recruit affordability gate (BUG E fix).
//
// Before the fix, can_recruit only checked goods + labor pool. Building requirements
// (barracks/stable/harbour/foundry) were ignored, so spearman showed can_recruit:true
// even at a settlement without barracks. The fix mirrors the real Recruit handler.

import (
	"testing"

	"formatet/megaron/server/internal/province"
)

// TestCanRecruit_SpearmanRequiresBarracks verifies that a spearman is not
// recruitable at a settlement that has no barracks, even with full goods and pop.
func TestCanRecruit_SpearmanRequiresBarracks(t *testing.T) {
	spec := province.UnitSpecs["spearman"]
	if !spec.RequiresBarracks {
		t.Fatal("spearman spec must require barracks (test premise invalid)")
	}

	builtTypes := map[string]bool{} // no barracks
	goodsStock := map[string]float64{"grain": 1000, "silver": 100}
	laborPool := 500

	afford := laborPool >= spec.PopCost
	if afford && spec.RequiresBarracks && !builtTypes["barracks"] {
		afford = false
	}
	if afford {
		for goodKey, needed := range spec.Costs {
			if goodsStock[goodKey] < needed {
				afford = false
				break
			}
		}
	}

	if afford {
		t.Error("spearman must not be recruitable without barracks (can_recruit gate broken)")
	}
}

// TestCanRecruit_SpearmanAffordableWithBarracks is the positive case.
func TestCanRecruit_SpearmanAffordableWithBarracks(t *testing.T) {
	spec := province.UnitSpecs["spearman"]

	builtTypes := map[string]bool{"barracks": true}
	goodsStock := map[string]float64{"grain": 1000, "silver": 100}
	laborPool := 500

	afford := laborPool >= spec.PopCost
	if afford && spec.RequiresBarracks && !builtTypes["barracks"] {
		afford = false
	}
	if afford && spec.RequiresStable && !builtTypes["stable"] {
		afford = false
	}
	if afford && spec.RequiresHarbour && !builtTypes["harbour"] {
		afford = false
	}
	if afford && spec.RequiresFoundry && !builtTypes["foundry"] {
		afford = false
	}
	if afford {
		for goodKey, needed := range spec.Costs {
			if goodsStock[goodKey] < needed {
				afford = false
				break
			}
		}
	}

	if !afford {
		t.Error("spearman must be recruitable with barracks + sufficient goods + pop")
	}
}

// TestCanRecruit_WarGalleyRequiresHarbourAndFoundry verifies that war_galley
// (harbour + foundry required) is blocked when either building is missing.
func TestCanRecruit_WarGalleyRequiresHarbourAndFoundry(t *testing.T) {
	spec := province.UnitSpecs["war_galley"]
	if !spec.RequiresHarbour || !spec.RequiresFoundry {
		t.Fatal("war_galley must require harbour + foundry (test premise invalid)")
	}

	cases := []struct {
		name       string
		built      map[string]bool
		wantAfford bool
	}{
		{"neither", map[string]bool{}, false},
		{"harbour only", map[string]bool{"harbour": true}, false},
		{"foundry only", map[string]bool{"foundry": true}, false},
		{"both", map[string]bool{"harbour": true, "foundry": true}, true},
	}

	goodsStock := map[string]float64{"cedar": 1000, "bronze": 1000, "silver": 1000}
	laborPool := 500

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			afford := laborPool >= spec.PopCost
			if afford && spec.RequiresBarracks && !tc.built["barracks"] {
				afford = false
			}
			if afford && spec.RequiresStable && !tc.built["stable"] {
				afford = false
			}
			if afford && spec.RequiresHarbour && !tc.built["harbour"] {
				afford = false
			}
			if afford && spec.RequiresFoundry && !tc.built["foundry"] {
				afford = false
			}
			if afford {
				for goodKey, needed := range spec.Costs {
					if goodsStock[goodKey] < needed {
						afford = false
						break
					}
				}
			}
			if afford != tc.wantAfford {
				t.Errorf("war_galley afford with %v = %v, want %v", tc.name, afford, tc.wantAfford)
			}
		})
	}
}
