package province

import (
	"math"
	"testing"
	"time"
)

func TestResourceState_CurrentProjectsForward(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	rs := ResourceState{Amount: 100, RatePerMinute: 2, Cap: 1000, LastCalcAt: base}
	if got := rs.Current(base.Add(30 * time.Minute)); math.Abs(got-160) > 1e-9 {
		t.Errorf("expected 160 after 30 min at 2/min, got %.3f", got)
	}
}

func TestResourceState_CurrentClampsToCap(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	rs := ResourceState{Amount: 980, RatePerMinute: 2, Cap: 1000, LastCalcAt: base}
	if got := rs.Current(base.Add(60 * time.Minute)); got != 1000 {
		t.Errorf("should clamp to cap 1000, got %.3f", got)
	}
}

func TestResourceState_CurrentFloorsAtZero(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	rs := ResourceState{Amount: 5, RatePerMinute: -2, Cap: 1000, LastCalcAt: base}
	if got := rs.Current(base.Add(60 * time.Minute)); got != 0 {
		t.Errorf("negative rate must floor at 0, got %.3f", got)
	}
}

// TestBronzeChain_EliteInfantryRequirements verifies that elite_infantry requires
// both a foundry and bronze as a material cost. This encodes the design invariant:
// no city can recruit elite troops without first crafting bronze, which in turn
// requires copper + tin. The geographic separation of deposits (mapgen) means
// trade is mandatory — this test ensures the gate is never accidentally removed.
func TestBronzeChain_EliteInfantryRequirements(t *testing.T) {
	spec, ok := UnitSpecs["elite_infantry"]
	if !ok {
		t.Fatal("elite_infantry must exist in UnitSpecs")
	}

	// Foundry gate: elite infantry requires a foundry building.
	if !spec.RequiresFoundry {
		t.Error("elite_infantry must require a foundry (bronze chain gate)")
	}
	// Barracks gate: elite infantry is a military unit — must also need barracks.
	if !spec.RequiresBarracks {
		t.Error("elite_infantry must require barracks")
	}

	// Material cost: bronze must be consumed per recruit.
	bronze, ok := spec.Costs["bronze"]
	if !ok {
		t.Fatal("elite_infantry must consume bronze (not present in Costs)")
	}
	if bronze <= 0 {
		t.Errorf("elite_infantry bronze cost must be > 0, got %.1f", bronze)
	}

	// Grain cost: military units require grain for upkeep.
	grain, ok := spec.Costs["grain"]
	if !ok {
		t.Error("elite_infantry must consume grain")
	}
	if grain <= 0 {
		t.Errorf("elite_infantry grain cost must be > 0, got %.1f", grain)
	}

	// Pop cost: elite infantry is expensive in manpower.
	if spec.PopCost <= 0 {
		t.Error("elite_infantry must have a positive pop cost")
	}
}

// TestBronzeChain_BronzeWallRequiresBronze verifies that bronze_wall costs bronze.
// The bronze wall is the defensive counterpart of elite_infantry — both are
// only accessible once a city can produce/acquire bronze.
func TestBronzeChain_BronzeWallRequiresBronze(t *testing.T) {
	spec, ok := BuildingSpecs[BuildingBronzeWall]
	if !ok {
		t.Fatal("bronze_wall must exist in BuildingSpecs")
	}
	bronze, ok := spec.Costs["bronze"]
	if !ok {
		t.Fatal("bronze_wall must consume bronze")
	}
	if bronze <= 0 {
		t.Errorf("bronze_wall bronze cost must be > 0, got %.1f", bronze)
	}
	if spec.WallsBonus <= 0 {
		t.Error("bronze_wall must grant a walls bonus")
	}
}

func TestResourceLedger_SnapshotGold(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	rl := ResourceLedger{Gold: ResourceState{Amount: 50, RatePerMinute: 1, Cap: 500, LastCalcAt: base}}
	snap := rl.Snapshot(base.Add(10 * time.Minute))
	if math.Abs(snap["gold"]-60) > 1e-9 {
		t.Errorf("gold snapshot should be 60, got %.3f", snap["gold"])
	}
	full := rl.SnapshotFull(base.Add(10 * time.Minute))
	if math.Abs(full["gold"].Amount-60) > 1e-9 || full["gold"].Rate != 1 || full["gold"].Cap != 500 {
		t.Errorf("full gold snapshot mismatch: %+v", full["gold"])
	}
}
