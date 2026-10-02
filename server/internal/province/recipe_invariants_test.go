package province

// megaron_plan_byggkostnader steg 5: building costs are RECIPES (materialet är
// det huset är gjort av), pinned as invariants rather than as a sum. Invariant 5
// is economy.TestRuleTable_NoBuildingLowersPerGubbe; invariant 4 (the founding
// stock) lives in api/handlers/create_metropolis_livestock_test.go.

import "testing"

// 1. Every material of every building and wall recipe is 0 or a whole number >= 4.
func TestRecipes_MaterialsAreZeroOrWholeAtLeastFour(t *testing.T) {
	check := func(name string, s BuildingSpec) {
		for g, v := range s.Costs {
			if v != 0 && (v < 4 || v != float64(int(v))) {
				t.Errorf("%s %s = %v, want 0 or a whole number >= 4", name, g, v)
			}
		}
	}
	for bt, s := range BuildingSpecs {
		check(string(bt), s)
	}
	for lvl, s := range WallLevelSpecs {
		check("wall L"+string(rune('0'+lvl)), s)
	}
}

// 2. Levelling: materials x2 / x4, time x2 / x3, cedar 0/4/8 for the cedar buildings.
func TestRecipes_LevelledTrappa(t *testing.T) {
	for bt := range LevelledBuildings {
		base := BuildingSpecs[bt]
		for lvl, mult := range map[int]float64{2: 2, 3: 4} {
			got, ok := LevelledSpec(bt, lvl)
			if !ok {
				t.Fatalf("%s L%d: no spec", bt, lvl)
			}
			for _, g := range []string{"timber", "stone"} {
				if got.Costs[g] != base.Costs[g]*mult {
					t.Errorf("%s L%d %s = %v, want %v", bt, lvl, g, got.Costs[g], base.Costs[g]*mult)
				}
			}
			if got.DurationTicks != base.DurationTicks*lvl {
				t.Errorf("%s L%d duration = %d, want %d", bt, lvl, got.DurationTicks, base.DurationTicks*lvl)
			}
			wantCedar := 0.0
			if LevelCedarBuildings[bt] {
				wantCedar = map[int]float64{2: 4, 3: 8}[lvl]
			}
			if got.Costs["cedar"] != wantCedar {
				t.Errorf("%s L%d cedar = %v, want %v", bt, lvl, got.Costs["cedar"], wantCedar)
			}
		}
		if l1, _ := LevelledSpec(bt, 1); l1.Costs["cedar"] != 0 {
			t.Errorf("%s L1 costs cedar", bt)
		}
	}
}

// 3. The basic workplaces cost no bronze at level 1.
func TestRecipes_BasicWorkplacesCostNoBronze(t *testing.T) {
	for _, bt := range []BuildingType{BuildingFarm, BuildingLumbermill, BuildingStonequarry, BuildingMine, BuildingFoundry} {
		if BuildingSpecs[bt].Costs["bronze"] != 0 {
			t.Errorf("%s L1 costs bronze", bt)
		}
	}
}

// 6. The plain ships and the spearman cost no bronze and no cedar.
func TestRecipes_PlainUnitsCostNoBronzeOrCedar(t *testing.T) {
	for _, u := range []string{"galley", "merchantman", "spearman"} {
		c := UnitSpecs[u].Costs
		if c["bronze"] != 0 || c["cedar"] != 0 {
			t.Errorf("%s costs bronze/cedar: %v", u, c)
		}
	}
}
