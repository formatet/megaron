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

// TestWallLevel3RequiresBronze verifies that the top-tier wall (level 3, Bronze Wall)
// costs bronze. This is the defensive counterpart of elite_infantry — both are
// only accessible once a city can produce/acquire bronze. WallLevelSpecs[3] is the
// canonical gate; the separate bronze_wall building type has been removed.
func TestWallLevel3RequiresBronze(t *testing.T) {
	spec, ok := WallLevelSpecs[3]
	if !ok {
		t.Fatal("WallLevelSpecs[3] must exist")
	}
	bronze, ok := spec.Costs["bronze"]
	if !ok {
		t.Fatal("WallLevelSpecs[3] must consume bronze")
	}
	if bronze <= 0 {
		t.Errorf("WallLevelSpecs[3] bronze cost must be > 0, got %.1f", bronze)
	}
	if spec.WallsBonus <= 0 {
		t.Error("WallLevelSpecs[3] must grant a walls bonus")
	}
}

// TestShipTaxonomy_GalleyTimber verifies galley byggs med timber, inte cedar.
func TestShipTaxonomy_GalleyTimber(t *testing.T) {
	spec, ok := UnitSpecs["galley"]
	if !ok {
		t.Fatal("galley must exist in UnitSpecs")
	}
	if _, hasCedar := spec.Costs["cedar"]; hasCedar {
		t.Error("galley should not cost cedar — it costs timber")
	}
	timber, ok := spec.Costs["timber"]
	if !ok || timber <= 0 {
		t.Errorf("galley must cost timber > 0, got %.1f", timber)
	}
	if !spec.RequiresHarbour {
		t.Error("galley (ship) must require harbour")
	}
}

// TestShipTaxonomy_WarGalleyCedarFoundry verifies war_galley kräver cedar + gjuteri.
// Brons ÄR borttaget ur kostnaden: war_galley var dubbelgateat bakom både cedar och
// brons, och eftersom brons är världens knappaste vara blev cedar aldrig efterfrågad.
// Gjuteriet står kvar som byggnadsgate; brons bär nu war_chariot + murnivåerna.
func TestShipTaxonomy_WarGalleyCedarFoundry(t *testing.T) {
	spec, ok := UnitSpecs["war_galley"]
	if !ok {
		t.Fatal("war_galley must exist in UnitSpecs")
	}
	if _, hasBronze := spec.Costs["bronze"]; hasBronze {
		t.Error("war_galley should not cost bronze — cedar is its strategic material")
	}
	cedar, hasCedar := spec.Costs["cedar"]
	if !hasCedar || cedar <= 0 {
		t.Errorf("war_galley must cost cedar > 0, got %.1f", cedar)
	}
	if !spec.RequiresHarbour {
		t.Error("war_galley must require harbour")
	}
	if !spec.RequiresFoundry {
		t.Error("war_galley must require foundry (bronskedja-gate)")
	}
	if spec.PopCost <= 10 {
		t.Errorf("war_galley PopCost should be > galley (10), got %d", spec.PopCost)
	}
}

// TestShipTaxonomy_MerchantmanTimberNoFoundry verifies merchantman byggs med timber, inget gjuteri.
func TestShipTaxonomy_MerchantmanTimberNoFoundry(t *testing.T) {
	spec, ok := UnitSpecs["merchantman"]
	if !ok {
		t.Fatal("merchantman must exist in UnitSpecs")
	}
	timber, ok := spec.Costs["timber"]
	if !ok || timber <= 0 {
		t.Errorf("merchantman must cost timber > 0, got %.1f", timber)
	}
	if spec.RequiresFoundry {
		t.Error("merchantman should not require foundry")
	}
	if !spec.RequiresHarbour {
		t.Error("merchantman must require harbour")
	}
}

// TestWarChariotRequiresBronzeStableNotFoundry verifies war_chariot costs bronze and
// requires a stable but NOT a foundry — a city that BUYS bronze can still build it.
func TestWarChariotRequiresBronzeStableNotFoundry(t *testing.T) {
	spec, ok := UnitSpecs["war_chariot"]
	if !ok {
		t.Fatal("war_chariot must exist in UnitSpecs")
	}
	bronze, hasBronze := spec.Costs["bronze"]
	if !hasBronze || bronze <= 0 {
		t.Errorf("war_chariot must cost bronze > 0, got %.1f", bronze)
	}
	if !spec.RequiresStable {
		t.Error("war_chariot must require a stable")
	}
	if spec.RequiresFoundry {
		t.Error("war_chariot must NOT require a foundry (buyable-bronze design)")
	}
}

func TestResourceLedger_SnapshotSilver(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	rl := ResourceLedger{Silver: ResourceState{Amount: 50, RatePerMinute: 1, Cap: 500, LastCalcAt: base}}
	snap := rl.Snapshot(base.Add(10 * time.Minute))
	if math.Abs(snap["silver"]-60) > 1e-9 {
		t.Errorf("silver snapshot should be 60, got %.3f", snap["silver"])
	}
	full := rl.SnapshotFull(base.Add(10 * time.Minute))
	if math.Abs(full["silver"].Amount-60) > 1e-9 || full["silver"].Rate != 1 || full["silver"].Cap != 500 {
		t.Errorf("full silver snapshot mismatch: %+v", full["silver"])
	}
}
