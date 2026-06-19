package economy

import (
	"math"
	"testing"
	"time"
)

// strptr is a test helper for the optional pointer fields on ProductionRule.
func strptr(s string) *string { return &s }

// TestBronzeChain_TinOnlyOnMountain verifies the geographic forcing function:
// a mountain_limestone hex WITH a tin deposit produces tin; a plains hex WITH
// the same deposit flag does NOT produce tin (wrong terrain). This is the
// fundamental rule that makes tin scarce and forces cross-island trade.
func TestBronzeChain_TinOnlyOnMountain(t *testing.T) {
	// Minimal rule set mirroring the real production_rules rows for tin.
	rules := []ProductionRule{
		{TerrainType: strptr("mountain_limestone"), GoodKey: GoodTin, RatePerMin: 0.01, RequiresDeposit: strptr("tin")},
		{TerrainType: strptr("mountain_limestone"), BuildingType: strptr("mine"), GoodKey: GoodTin, RatePerMin: 0.025, RequiresDeposit: strptr("tin")},
	}

	// Plains with tin deposit: no production (wrong terrain).
	plainsWithDeposit := effectiveRates(rules, "plains", nil, false, true)
	if plainsWithDeposit[GoodTin] != 0 {
		t.Errorf("plains must not produce tin even with deposit, got %.4f", plainsWithDeposit[GoodTin])
	}

	// Mountain_limestone without deposit: no production.
	mountainNoDeposit := effectiveRates(rules, "mountain_limestone", nil, false, false)
	if mountainNoDeposit[GoodTin] != 0 {
		t.Errorf("mountain without tin deposit must not produce tin, got %.4f", mountainNoDeposit[GoodTin])
	}

	// Mountain_limestone with deposit: produces tin (terrain-only rule).
	mountainWithDeposit := effectiveRates(rules, "mountain_limestone", nil, false, true)
	if mountainWithDeposit[GoodTin] != 0.01 {
		t.Errorf("mountain+deposit should produce 0.01 tin/min, got %.4f", mountainWithDeposit[GoodTin])
	}

	// Mountain_limestone + deposit + mine: both rules fire.
	mountainWithMine := effectiveRates(rules, "mountain_limestone", []string{"mine"}, false, true)
	if mountainWithMine[GoodTin] != 0.035 {
		t.Errorf("mountain+deposit+mine should produce 0.035 tin/min, got %.4f", mountainWithMine[GoodTin])
	}
}

// TestBronzeChain_CopperNotOnMountain verifies that copper production requires hills
// terrain (not mountain_limestone), reinforcing the geographic separation design:
// copper and tin are on different terrain types.
func TestBronzeChain_CopperNotOnMountain(t *testing.T) {
	rules := []ProductionRule{
		{TerrainType: strptr("hills"), GoodKey: GoodCopper, RatePerMin: 0.02, RequiresDeposit: strptr("copper")},
		{TerrainType: strptr("hills"), BuildingType: strptr("mine"), GoodKey: GoodCopper, RatePerMin: 0.04, RequiresDeposit: strptr("copper")},
	}

	// Mountain_limestone with copper deposit: no production (wrong terrain).
	mountainWithCopper := effectiveRates(rules, "mountain_limestone", nil, true, false)
	if mountainWithCopper[GoodCopper] != 0 {
		t.Errorf("mountain must not produce copper (copper is a hills good), got %.4f", mountainWithCopper[GoodCopper])
	}

	// Hills with copper deposit: produces copper.
	hillsWithCopper := effectiveRates(rules, "hills", nil, true, false)
	if hillsWithCopper[GoodCopper] != 0.02 {
		t.Errorf("hills+copper_deposit should produce 0.02/min, got %.4f", hillsWithCopper[GoodCopper])
	}
}

func TestGoodState_CurrentProjectsForward(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	g := GoodState{Amount: 100, Rate: 2, Cap: 1000, CalcAt: base}
	// 10 minutes at 2/min → +20.
	if got := g.Current(base.Add(10 * time.Minute)); math.Abs(got-120) > 1e-9 {
		t.Errorf("expected 120 after 10 min, got %.3f", got)
	}
}

func TestGoodState_CurrentClampsToCap(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	g := GoodState{Amount: 990, Rate: 2, Cap: 1000, CalcAt: base}
	if got := g.Current(base.Add(60 * time.Minute)); got != 1000 {
		t.Errorf("should clamp to cap 1000, got %.3f", got)
	}
}

func TestGoodState_CurrentFloorsAtZero(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	// Negative rate (e.g. recipe consumption) must never drive stock below zero.
	g := GoodState{Amount: 10, Rate: -2, Cap: 1000, CalcAt: base}
	if got := g.Current(base.Add(60 * time.Minute)); got != 0 {
		t.Errorf("should floor at 0, got %.3f", got)
	}
}

func TestGoodState_CurrentAtCalcAt(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	g := GoodState{Amount: 42, Rate: 5, Cap: 1000, CalcAt: base}
	if got := g.Current(base); got != 42 {
		t.Errorf("at calc_at should equal stored amount, got %.3f", got)
	}
}
