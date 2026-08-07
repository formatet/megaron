package economy

import "testing"

// TestLaborCapacity_GrainIsExempt — spannmål är försörjningsvaran. Att grinda hur
// många som får bruka jorden skulle svälta städer som inte har rum att bygga.
func TestLaborCapacity_GrainIsExempt(t *testing.T) {
	if got := LaborCapacity("grain", false, 0, 0); got != 1.0 {
		t.Errorf("grain must be uncapped even with no fields, no buildings and no labor pool, got %.2f", got)
	}
}

// TestLaborCapacity_FieldsWithoutBuildings — en vara med terrängväg kan brukas
// direkt av en andel av staden utan att något är byggt.
func TestLaborCapacity_FieldsWithoutBuildings(t *testing.T) {
	got := LaborCapacity("timber", true, 0, 500)
	if got != GoodLaborTerrainBase {
		t.Errorf("field-only good should get exactly the terrain base %.2f, got %.2f",
			GoodLaborTerrainBase, got)
	}
}

// TestLaborCapacity_NoPathNoWork — en vara utan både terrängväg och byggnad kan
// ingen sysselsättas med.
func TestLaborCapacity_NoPathNoWork(t *testing.T) {
	if got := LaborCapacity("pottery", false, 0, 500); got != 0 {
		t.Errorf("a good with neither field path nor workplace must employ nobody, got %.2f", got)
	}
}

// TestLaborCapacity_SlotsAreAbsoluteNotAShare — P2's whole point
// (megaron_plan_fysisk_gubbemodell.md: "P1 gav 3x men inte bromsen — P4 ÄR
// balansmekanismen", and P2 is the first slice to apply the brake to
// buildings). A fixed slot count must translate into a FIXED headcount of
// employed citizens — capacity × laborPool must equal buildingSlots exactly —
// regardless of how big the city is. Under the old share model this ratio grew
// with population; that is the regression this test exists to catch.
func TestLaborCapacity_SlotsAreAbsoluteNotAShare(t *testing.T) {
	const slots = 4
	for _, pool := range []int{50, 500, 5000} {
		cap := LaborCapacity("silver", false, slots, pool)
		employed := cap * float64(pool)
		if diff := employed - float64(slots); diff > 1e-9 || diff < -1e-9 {
			t.Errorf("laborPool=%d: %d slots should employ exactly %d citizens, got %.4f (capacity %.6f)",
				pool, slots, slots, employed, cap)
		}
	}
}

// TestLaborCapacity_FieldPlusBuildingAdd — ett fält och en byggnad för samma
// vara (t.ex. olja: farm-fältet och Olive Press) lägger till varandra, precis
// som innan P2 — bara byggnadstermen bytte enhet från andel till styckeslot.
func TestLaborCapacity_FieldPlusBuildingAdd(t *testing.T) {
	const pool = 200
	fieldOnly := LaborCapacity("oil", true, 0, pool)
	fieldPlusBuilding := LaborCapacity("oil", true, 2, pool)
	if fieldPlusBuilding <= fieldOnly {
		t.Errorf("adding 2 building slots must raise capacity above the field-only floor %.4f, got %.4f",
			fieldOnly, fieldPlusBuilding)
	}
	wantSlotShare := 2.0 / float64(pool)
	gotSlotShare := fieldPlusBuilding - fieldOnly
	if diff := gotSlotShare - wantSlotShare; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("the building's contribution should be exactly slots/laborPool = %.6f, got %.6f",
			wantSlotShare, gotSlotShare)
	}
}

// TestLaborCapacity_NeverExceedsWholeCity — kapaciteten är en ANDEL av staden och
// får aldrig gå över 1.0, annars skulle en klampning mot den vara verkningslös.
// Also covers the laborPool=0 edge (founder-phase host, or a settlement that
// just lost its whole population) — must not divide by zero.
func TestLaborCapacity_NeverExceedsWholeCity(t *testing.T) {
	if got := LaborCapacity("oil", true, 30, 10); got > 1.0 {
		t.Errorf("capacity is a share of the city and must never exceed 1.0, got %.2f", got)
	}
	if got := LaborCapacity("oil", true, 30, 0); got > 1.0 || got < 0 {
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
