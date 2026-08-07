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
	grainNet, fishNet, _ := FoodConsumptionSplit(demand, grainProd, fishProd, 0)
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
			grainNet, fishNet, _ := FoodConsumptionSplit(c.demand, c.grainProd, 0, 0)
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
	grainNet, fishNet, _ := FoodConsumptionSplit(demand, grainProd, fishProd, 0)
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
		grainNet, fishNet, _ := FoodConsumptionSplit(c.demand, c.grainProd, c.fishProd, 0)
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

// TestFoodConsumptionSplit_AK4_LivestockCoversTheRest: a settlement with a
// grain shortfall, zero fish, and a herd falls back to slaughtering the herd
// instead of starving — grainNet lands at exactly 0 (the shortfall is paid
// by livestock, not by draining grain further) and the herd shrinks by
// exactly enough whole animals to cover the demand.
func TestFoodConsumptionSplit_AK4_LivestockCoversTheRest(t *testing.T) {
	demand := 5.0
	grainProd := 0.0
	fishProd := 0.0
	livestockStock := 3.0 // 3 whole animals, 600 food available
	grainNet, fishNet, livestockConsumed := FoodConsumptionSplit(demand, grainProd, fishProd, livestockStock)
	if grainNet != 0 {
		t.Errorf("grainNet = %v, want exactly 0 (livestock covers the shortfall, city does not starve)", grainNet)
	}
	if fishNet != 0 {
		t.Errorf("fishNet = %v, want 0 (no fish production)", fishNet)
	}
	if livestockConsumed != 1 {
		t.Errorf("livestockConsumed = %v, want 1 (ceil(5/200) = 1 whole animal)", livestockConsumed)
	}
}

// TestFoodConsumptionSplit_LivestockSlaughteredInWholeUnits: a herd of 2.7
// (settled() can produce a fractional stock even though livestock only
// arrives/leaves in whole animals) may only be slaughtered in whole units —
// the fractional 0.7 is never itself treated as edible.
func TestFoodConsumptionSplit_LivestockSlaughteredInWholeUnits(t *testing.T) {
	demand := 1000.0 // far more than the herd could ever cover
	livestockStock := 2.7
	_, _, livestockConsumed := FoodConsumptionSplit(demand, 0, 0, livestockStock)
	if livestockConsumed != 2 {
		t.Errorf("livestockConsumed = %v, want 2 (floor(2.7), the 0.7 is not a slaughterable animal)", livestockConsumed)
	}
}

// TestFoodConsumptionSplit_LivestockUntouchedWhenGrainSuffices: a
// self-sufficient settlement (grain alone covers demand) never touches its
// herd — the fallback chain only engages once grain AND fish are both
// exhausted.
func TestFoodConsumptionSplit_LivestockUntouchedWhenGrainSuffices(t *testing.T) {
	_, _, livestockConsumed := FoodConsumptionSplit(5.0, 12.0, 0, 10.0)
	if livestockConsumed != 0 {
		t.Errorf("livestockConsumed = %v, want 0 (grain alone covers demand, herd must stay untouched)", livestockConsumed)
	}
}

// TestFoodConsumptionSplit_LivestockPartialCoverageStillStarves: a herd too
// small to cover the whole shortfall is fully spent, and the STILL-unmet
// remainder keeps draining grain — the herd delays starvation, it does not
// make it impossible.
func TestFoodConsumptionSplit_LivestockPartialCoverageStillStarves(t *testing.T) {
	demand := 1000.0
	livestockStock := 1.0 // covers only 200 of the 1000 shortfall
	grainNet, _, livestockConsumed := FoodConsumptionSplit(demand, 0, 0, livestockStock)
	if livestockConsumed != 1 {
		t.Errorf("livestockConsumed = %v, want 1 (the whole herd, still insufficient)", livestockConsumed)
	}
	wantGrainNet := -(demand - livestockFoodValue)
	if diff := grainNet - wantGrainNet; diff > splitEps || diff < -splitEps {
		t.Errorf("grainNet = %v, want %v (unmet demand after the herd is spent still drains grain)", grainNet, wantGrainNet)
	}
}
