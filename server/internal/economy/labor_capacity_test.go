package economy

import "testing"

// TestLaborCapacity_GrainIsExempt — spannmål är försörjningsvaran. Att grinda hur
// många som får bruka jorden skulle svälta städer som inte har rum att bygga.
func TestLaborCapacity_GrainIsExempt(t *testing.T) {
	if got := LaborCapacity("grain", false, 0, 0, 0); got != 1.0 {
		t.Errorf("grain must be uncapped even with no field path, no hex/building slots and no labor pool, got %.2f", got)
	}
}

// TestLaborCapacity_HexPlusBuildingAdd — hexslots (P3) och byggnadsslots (P2)
// lägger till varandra, samma additiva form som innan båda blev absoluta tal.
func TestLaborCapacity_HexPlusBuildingAdd(t *testing.T) {
	const pool = 200
	hexOnly := LaborCapacity("oil", true, 4, 0, pool)
	hexPlusBuilding := LaborCapacity("oil", true, 4, 2, pool)
	if hexPlusBuilding <= hexOnly {
		t.Errorf("adding 2 building slots must raise capacity above the hex-only floor %.4f, got %.4f",
			hexOnly, hexPlusBuilding)
	}
	wantBuildingShare := 2.0 / float64(pool)
	gotBuildingShare := hexPlusBuilding - hexOnly
	if diff := gotBuildingShare - wantBuildingShare; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("the building's contribution should be exactly slots/laborPool = %.6f, got %.6f",
			wantBuildingShare, gotBuildingShare)
	}
}

// TestLaborCapacity_NoSlotsNoWork — en vara utan vare sig hex- eller
// byggnadsplatser (och utan fältväg att falla tillbaka på) kan ingen
// sysselsättas med.
func TestLaborCapacity_NoSlotsNoWork(t *testing.T) {
	if got := LaborCapacity("pottery", false, 0, 0, 500); got != 0 {
		t.Errorf("a good with neither hex nor building slots nor a field path must employ nobody, got %.2f", got)
	}
}

// TestLaborCapacity_SlotsAreAbsoluteNotAShare — P2/P3:s gemensamma poäng
// (megaron_plan_fysisk_gubbemodell.md: "P1 gav 3x men inte bromsen — P4 ÄR
// balansmekanismen", och P2+P3 är de slices som lägger på bromsen). Ett fast
// antal platser (hex+byggnad) måste ge ett FAST antal sysselsatta —
// capacity × laborPool ska vara exakt slots — oavsett hur stor staden är.
func TestLaborCapacity_SlotsAreAbsoluteNotAShare(t *testing.T) {
	const hexSlots, buildingSlots = 6, 4
	const totalSlots = hexSlots + buildingSlots
	for _, pool := range []int{50, 500, 5000} {
		cap := LaborCapacity("silver", true, hexSlots, buildingSlots, pool)
		employed := cap * float64(pool)
		if diff := employed - float64(totalSlots); diff > 1e-9 || diff < -1e-9 {
			t.Errorf("laborPool=%d: %d total slots should employ exactly %d citizens, got %.4f (capacity %.6f)",
				pool, totalSlots, totalSlots, employed, cap)
		}
	}
}

// TestLaborCapacity_UncoveredGoodFallsBackToTerrainBase — regression för det
// verkliga fyndet vid P3:s bygge: wine/oil/stone finns inte i §8.3 och tappade
// tyst till NOLL kapacitet innan den här fallbacken lades till
// (TestRecomputeProduction_WineOn{RiverValley,Plains}OnlyCatchment gick från
// positiv rate till exakt 0). hexSlots=0 men hasFieldPath=true måste ge
// GoodLaborTerrainBase, inte 0.
func TestLaborCapacity_UncoveredGoodFallsBackToTerrainBase(t *testing.T) {
	if got := LaborCapacity("wine", true, 0, 0, 500); got != GoodLaborTerrainBase {
		t.Errorf("a good with a field path but no §8.3 hex-capacity coverage should fall back to %.2f, got %.2f",
			GoodLaborTerrainBase, got)
	}
}

// TestLaborCapacity_NeverExceedsWholeCity — kapaciteten är en ANDEL av staden och
// får aldrig gå över 1.0. Also covers laborPool=0 (founder-phase host, or a
// settlement that just lost its whole population) — must not divide by zero.
func TestLaborCapacity_NeverExceedsWholeCity(t *testing.T) {
	if got := LaborCapacity("oil", true, 20, 30, 10); got > 1.0 {
		t.Errorf("capacity is a share of the city and must never exceed 1.0, got %.2f", got)
	}
	if got := LaborCapacity("oil", true, 20, 30, 0); got > 1.0 || got < 0 {
		t.Errorf("laborPool=0 must not divide by zero or go negative, got %.2f", got)
	}
}

// TestWorkplaceSlots_UnknownBuildingOrLevelReturnsZero — arbetssätt §7: en
// tyst fallback som gissar ett värde är misstänkt. En byggnad som saknas ur
// tabellen (temple — cult labor, en egen väg) eller en nivå utanför tabellens
// spann ska ge noll platser, aldrig en gissning.
func TestWorkplaceSlots_UnknownBuildingOrLevelReturnsZero(t *testing.T) {
	if got := WorkplaceSlots("temple", 1); got != 0 {
		t.Errorf("temple is cult labor, not a workplace-slot building — want 0, got %d", got)
	}
	if got := WorkplaceSlots("farm", 0); got != 0 {
		t.Errorf("level 0 does not exist — want 0, got %d", got)
	}
	if got := WorkplaceSlots("farm", 99); got != 0 {
		t.Errorf("level past the table's range must not panic or guess — want 0, got %d", got)
	}
}

// TestWorkplaceSlots_LevelsIncrease — poängen med mekaniken: en högre nivå ökar
// antalet medborgare som kan sysselsättas, för varje byggnad i tabellen.
func TestWorkplaceSlots_LevelsIncrease(t *testing.T) {
	for buildingType := range workplaceSlotTable {
		l1 := WorkplaceSlots(buildingType, 1)
		l2 := WorkplaceSlots(buildingType, 2)
		l3 := WorkplaceSlots(buildingType, 3)
		if l1 <= 0 {
			t.Errorf("%s level 1 must grant at least one slot, got %d", buildingType, l1)
		}
		if l2 <= l1 || l3 <= l2 {
			t.Errorf("%s slots must strictly increase with level, got L1=%d L2=%d L3=%d",
				buildingType, l1, l2, l3)
		}
	}
}

// TestWorkplaceSlots_MineAndSilverMineMatchTaxonomy — regression for the exact
// bug this file's history records: an earlier version of this table
// extrapolated 1/2/4 for Mine/Silver mine instead of reading
// Temenos_varutaxonomi_sol.md §8.2's own 2/4/6. Pinned so it cannot silently
// drift back.
func TestWorkplaceSlots_MineAndSilverMineMatchTaxonomy(t *testing.T) {
	for _, bt := range []string{"mine", "silver_mine"} {
		want := [3]int{2, 4, 6}
		for level := 1; level <= 3; level++ {
			if got := WorkplaceSlots(bt, level); got != want[level-1] {
				t.Errorf("%s level %d: taxonomy §8.2 says %d, got %d", bt, level, want[level-1], got)
			}
		}
	}
}

// TestHexCapacity_UnknownTerrainGrantsNothing — arbetssätt §7: a terrain with
// no matching rule (e.g. mountain_red, semi_desert — in the enum but not in
// §8.3's coverage) must contribute nothing, not a guessed default.
func TestHexCapacity_UnknownTerrainGrantsNothing(t *testing.T) {
	if _, ok := terrainCapacityTable["mountain_red"]; ok {
		t.Errorf("mountain_red has no §8.3 rule — it must stay absent from the table, not default to something")
	}
	if _, ok := terrainCapacityTable["semi_desert"]; ok {
		t.Errorf("semi_desert has no §8.3 rule — it must stay absent from the table, not default to something")
	}
}

// TestHexCapacity_WithBuildingExceedsWithout — poängen med §8.3: den relevanta
// byggnaden höjer hexens tak, den ersätter det inte.
func TestHexCapacity_WithBuildingExceedsWithout(t *testing.T) {
	for terrain, rule := range terrainCapacityTable {
		if rule.relevantBuilding == "" {
			continue // flodfiske: ingen byggnad boostar river/river_ford/deep_sea idag
		}
		if rule.capWithBuilding <= rule.capNoBuilding {
			t.Errorf("%s (%s): capWithBuilding (%d) must exceed capNoBuilding (%d)",
				terrain, rule.goodKey, rule.capWithBuilding, rule.capNoBuilding)
		}
	}
	for _, rule := range plainsCapacityRules {
		if rule.relevantBuilding == "" {
			continue // livestock: ingen betesbyggnad finns än
		}
		if rule.capWithBuilding <= rule.capNoBuilding {
			t.Errorf("plains/%s: capWithBuilding (%d) must exceed capNoBuilding (%d)",
				rule.goodKey, rule.capWithBuilding, rule.capNoBuilding)
		}
	}
}
