package economy

// Pure, DB-free tests for FoodConsumptionSplit — the fisk-föder-befolkningen
// invariant (grain first, fish for the rest) in isolation from
// RecomputeProduction's SQL. These pin AK1-AK3 from the contract at the
// formula level; recompute_fish_test.go proves the same shapes end-to-end
// through a real settlement + catchment.

import "testing"

const splitEps = 1e-9

// TestFoodConsumptionSplit_AK1_FishFullyCoversZeroGrain: a settlement with
// zero grain production and fish production >= demand gets grain net EXACTLY
// 0 (never negative) and fish net = fishProd - demand.
func TestFoodConsumptionSplit_AK1_FishFullyCoversZeroGrain(t *testing.T) {
	demand := 5.0
	grainProd := 0.0
	fishProd := 8.0
	grainNet, fishNet := FoodConsumptionSplit(demand, grainProd, fishProd)
	if grainNet != 0 {
		t.Errorf("grainNet = %v, want exactly 0 (not negative)", grainNet)
	}
	wantFishNet := fishProd - demand
	if diff := fishNet - wantFishNet; diff > splitEps || diff < -splitEps {
		t.Errorf("fishNet = %v, want %v (fishProd - demand)", fishNet, wantFishNet)
	}
}

// TestFoodConsumptionSplit_AK2_NoFishIsUnchanged: fishProd=0 must reproduce
// the pre-slice formula exactly — grainNet = grainProd - demand, fishNet = 0
// — a settlement with no fish production regresses in no way.
func TestFoodConsumptionSplit_AK2_NoFishIsUnchanged(t *testing.T) {
	cases := []struct {
		name             string
		demand, grainProd float64
	}{
		{"grain covers demand", 5.0, 12.0},
		{"grain short of demand", 5.0, 2.0},
		{"grain exactly at demand", 5.0, 5.0},
		{"no grain, no fish, populated", 5.0, 0.0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			grainNet, fishNet := FoodConsumptionSplit(c.demand, c.grainProd, 0)
			wantGrainNet := c.grainProd - c.demand
			if diff := grainNet - wantGrainNet; diff > splitEps || diff < -splitEps {
				t.Errorf("grainNet = %v, want %v (grainProd - demand, pre-slice formula)", grainNet, wantGrainNet)
			}
			if fishNet != 0 {
				t.Errorf("fishNet = %v, want 0 (no fish production)", fishNet)
			}
		})
	}
}

// TestFoodConsumptionSplit_AK3_PartialFishCoverage: fishProd < demand (with
// grainProd=0, the AK3 scenario as worded in the contract) gives fish net
// EXACTLY 0 (fish fully consumed, no surplus) and grain net =
// -(demand - fishProd).
func TestFoodConsumptionSplit_AK3_PartialFishCoverage(t *testing.T) {
	demand := 5.0
	grainProd := 0.0
	fishProd := 2.0
	grainNet, fishNet := FoodConsumptionSplit(demand, grainProd, fishProd)
	if fishNet != 0 {
		t.Errorf("fishNet = %v, want exactly 0 (fish fully consumed, no surplus)", fishNet)
	}
	wantGrainNet := -(demand - fishProd)
	if diff := grainNet - wantGrainNet; diff > splitEps || diff < -splitEps {
		t.Errorf("grainNet = %v, want %v (-(demand - fishProd))", grainNet, wantGrainNet)
	}
}

// TestFoodConsumptionSplit_TotalDrainAlwaysEqualsDemand: whatever the split
// between grain and fish, the SUM of what was drawn from both goods (their
// production minus their net) is exactly demand — never more, never less.
// This is the "one storhet" half of the invariant, independent of AK1-AK3's
// individual scenarios.
func TestFoodConsumptionSplit_TotalDrainAlwaysEqualsDemand(t *testing.T) {
	cases := []struct {
		demand, grainProd, fishProd float64
	}{
		{5, 0, 8},     // AK1: fish fully covers
		{5, 12, 0},    // AK2: grain alone covers
		{5, 0, 2},     // AK3: partial fish, no grain
		{5, 2, 1},     // mixed partial coverage
		{5, 0, 0},     // nothing produced
		{0, 3, 3},     // no population, no demand
	}
	for _, c := range cases {
		grainNet, fishNet := FoodConsumptionSplit(c.demand, c.grainProd, c.fishProd)
		grainDrawn := c.grainProd - grainNet
		fishDrawn := c.fishProd - fishNet
		totalDrawn := grainDrawn + fishDrawn
		if diff := totalDrawn - c.demand; diff > splitEps || diff < -splitEps {
			t.Errorf("demand=%v grainProd=%v fishProd=%v: total drawn = %v, want exactly demand (%v)",
				c.demand, c.grainProd, c.fishProd, totalDrawn, c.demand)
		}
		if fishNet < -splitEps {
			t.Errorf("demand=%v grainProd=%v fishProd=%v: fishNet = %v, must never be negative (fish is never the sink)",
				c.demand, c.grainProd, c.fishProd, fishNet)
		}
	}
}
