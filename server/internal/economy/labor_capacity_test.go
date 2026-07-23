package economy

import "testing"

// TestLaborCapacity_GrainIsExempt — spannmål är försörjningsvaran. Att grinda hur
// många som får bruka jorden skulle svälta städer som inte har rum att bygga.
func TestLaborCapacity_GrainIsExempt(t *testing.T) {
	if got := LaborCapacity("grain", false, 0); got != 1.0 {
		t.Errorf("grain must be uncapped even with no fields and no buildings, got %.2f", got)
	}
}

// TestLaborCapacity_FieldsWithoutBuildings — en vara med terrängväg kan brukas
// direkt av en andel av staden utan att något är byggt.
func TestLaborCapacity_FieldsWithoutBuildings(t *testing.T) {
	got := LaborCapacity("timber", true, 0)
	if got != GoodLaborTerrainBase {
		t.Errorf("field-only good should get exactly the terrain base %.2f, got %.2f",
			GoodLaborTerrainBase, got)
	}
}

// TestLaborCapacity_NoPathNoWork — en vara utan både terrängväg och byggnad kan
// ingen sysselsättas med.
func TestLaborCapacity_NoPathNoWork(t *testing.T) {
	if got := LaborCapacity("pottery", false, 0); got != 0 {
		t.Errorf("a good with neither field path nor workplace must employ nobody, got %.2f", got)
	}
}

// TestLaborCapacity_LevelsAddStations — poängen med mekaniken: en högre nivå ökar
// antalet medborgare som kan sysselsättas.
func TestLaborCapacity_LevelsAddStations(t *testing.T) {
	l1 := LaborCapacity("pottery", false, 1)
	l2 := LaborCapacity("pottery", false, 2)
	if l2 <= l1 {
		t.Errorf("level 2 must employ more than level 1, got %.2f vs %.2f", l2, l1)
	}
	if l1 != BuildingLaborPerLevel {
		t.Errorf("level 1 should be exactly BuildingLaborPerLevel %.2f, got %.2f",
			BuildingLaborPerLevel, l1)
	}
}

// TestLaborCapacity_NeverExceedsWholeCity — kapaciteten är en ANDEL av staden och
// får aldrig gå över 1.0, annars skulle en klampning mot den vara verkningslös.
func TestLaborCapacity_NeverExceedsWholeCity(t *testing.T) {
	if got := LaborCapacity("oil", true, 3); got > 1.0 {
		t.Errorf("capacity is a share of the city and must never exceed 1.0, got %.2f", got)
	}
}

// TestLaborCapacity_NoLiveRegression — nivå 1 måste täcka varje allokering som
// faktiskt fanns i drift när mekaniken landade (mätt 2026-07-23), precis som
// Timothy kalibrerade templets nivå 1 på det 0.15-golv LaborAlloc redan lade.
// Annars sänker deployen produktionen i städer som inte gjort något fel.
func TestLaborCapacity_NoLiveRegression(t *testing.T) {
	cases := []struct {
		good      string
		field     bool
		levels    int
		allocated float64
	}{
		{"timber", true, 0, 0.25},    // högsta fält-allokering i drift
		{"silver", false, 1, 0.50},   // högsta rena byggnads-allokering (Argyros)
		{"copper", true, 1, 0.20},
		{"fish", true, 1, 0.20},
		{"stone", true, 1, 0.20},
		{"livestock", true, 0, 0.20}, // livestock har ingen byggnadsregel alls
		{"oil", true, 1, 0.20},
		{"horses", false, 1, 0.15},
		{"pottery", false, 1, 0.15},
		{"purple", true, 0, 0.05},
	}
	for _, c := range cases {
		if got := LaborCapacity(c.good, c.field, c.levels); got < c.allocated {
			t.Errorf("%s: level-1 capacity %.2f is below the %.2f already allocated in drift — this deploy would cut production",
				c.good, got, c.allocated)
		}
	}
}
