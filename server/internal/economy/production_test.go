package economy

import (
	"math"
	"testing"
	"time"
)

// strptr is a test helper for the optional pointer fields on ProductionRule.
func strptr(s string) *string { return &s }

func TestEffectiveRates_TerrainFilter(t *testing.T) {
	rules := []ProductionRule{
		{TerrainType: strptr("forest_olive_grove"), GoodKey: GoodCedar, RatePerMin: 0.1},
		{TerrainType: strptr("coast_beach"), GoodKey: GoodFish, RatePerMin: 0.2},
	}

	got := EffectiveRates(rules, "forest_olive_grove", nil, false, false)
	if got[GoodCedar] != 0.1 {
		t.Errorf("forest should produce cedar 0.1, got %.3f", got[GoodCedar])
	}
	if _, ok := got[GoodFish]; ok {
		t.Errorf("forest must not produce fish, got %v", got)
	}
}

func TestEffectiveRates_NilTerrainAlwaysFires(t *testing.T) {
	rules := []ProductionRule{
		{TerrainType: nil, GoodKey: GoodGrain, RatePerMin: 0.05},
	}
	for _, terrain := range []string{"coast_beach", "mountain_limestone", "deep_sea"} {
		got := EffectiveRates(rules, terrain, nil, false, false)
		if got[GoodGrain] != 0.05 {
			t.Errorf("nil-terrain rule should fire on %s, got %.3f", terrain, got[GoodGrain])
		}
	}
}

func TestEffectiveRates_BuildingRequired(t *testing.T) {
	rules := []ProductionRule{
		{BuildingType: strptr("lumbermill"), GoodKey: GoodCedar, RatePerMin: 0.05},
	}

	// Without the building, no production.
	if got := EffectiveRates(rules, "any", nil, false, false); got[GoodCedar] != 0 {
		t.Errorf("missing lumbermill should yield no cedar, got %.3f", got[GoodCedar])
	}
	// With the building, the rule fires.
	if got := EffectiveRates(rules, "any", []string{"lumbermill"}, false, false); got[GoodCedar] != 0.05 {
		t.Errorf("lumbermill should yield cedar 0.05, got %.3f", got[GoodCedar])
	}
}

func TestEffectiveRates_DepositGating(t *testing.T) {
	rules := []ProductionRule{
		{TerrainType: nil, GoodKey: GoodCopper, RatePerMin: 0.1, RequiresDeposit: strptr("copper")},
		{TerrainType: nil, GoodKey: GoodTin, RatePerMin: 0.1, RequiresDeposit: strptr("tin")},
	}

	none := EffectiveRates(rules, "any", nil, false, false)
	if none[GoodCopper] != 0 || none[GoodTin] != 0 {
		t.Errorf("no deposits should yield no copper/tin, got %v", none)
	}

	copperOnly := EffectiveRates(rules, "any", nil, true, false)
	if copperOnly[GoodCopper] != 0.1 {
		t.Errorf("copper deposit should yield copper, got %.3f", copperOnly[GoodCopper])
	}
	if copperOnly[GoodTin] != 0 {
		t.Errorf("copper deposit must not yield tin, got %.3f", copperOnly[GoodTin])
	}

	both := EffectiveRates(rules, "any", nil, true, true)
	if both[GoodCopper] != 0.1 || both[GoodTin] != 0.1 {
		t.Errorf("both deposits should yield copper and tin, got %v", both)
	}
}

func TestEffectiveRates_RatesSum(t *testing.T) {
	// The cedar fallback pattern: a lumbermill rule (any terrain) plus a
	// forest-specific rule should sum on forest terrain.
	rules := []ProductionRule{
		{BuildingType: strptr("lumbermill"), GoodKey: GoodCedar, RatePerMin: 0.05},
		{TerrainType: strptr("forest_olive_grove"), BuildingType: strptr("lumbermill"), GoodKey: GoodCedar, RatePerMin: 0.1},
	}
	got := EffectiveRates(rules, "forest_olive_grove", []string{"lumbermill"}, false, false)
	if math.Abs(got[GoodCedar]-0.15) > 1e-9 {
		t.Errorf("forest lumbermill should sum to 0.15, got %.3f", got[GoodCedar])
	}
}

func TestEffectiveRates_Empty(t *testing.T) {
	got := EffectiveRates(nil, "any", nil, false, false)
	if len(got) != 0 {
		t.Errorf("no rules should yield empty map, got %v", got)
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
