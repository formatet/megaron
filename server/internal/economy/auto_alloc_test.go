package economy

import (
	"math"
	"testing"
)

// applyAutoAlloc is a pure-function harness that mirrors AutoAllocateUnlocked's
// grain-skim algorithm so it can be tested without a DB transaction.
//
// Input:  weights = current settlement_labor map (good_key → weight).
//         unlockedGoods = goods to attempt to allocate (same semantics as AutoAllocateUnlocked).
// Output: updated weights map and the list of goods that were actually allocated.
func applyAutoAlloc(weights map[string]float64, unlockedGoods []string) (map[string]float64, []string) {
	out := make(map[string]float64, len(weights))
	for k, v := range weights {
		out[k] = v
	}
	var sumW float64
	for _, w := range out {
		sumW += w
	}
	grainW := out[GoodGrain]
	var allocated []string

	for _, g := range unlockedGoods {
		if g == GoodGrain {
			continue
		}
		idle := 1.0 - sumW
		if idle < 0 {
			idle = 0
		}
		var actualSlice, skimFromGrain float64
		if idle >= oreSlice {
			actualSlice = oreSlice
			skimFromGrain = 0
		} else {
			need := oreSlice - idle
			skim := grainW
			if skim > need {
				skim = need
			}
			actualSlice = idle + skim
			skimFromGrain = skim
		}
		if actualSlice <= 0 {
			continue
		}
		if skimFromGrain > 0 {
			grainW -= skimFromGrain
			out[GoodGrain] = grainW
		}
		out[g] = actualSlice
		sumW += actualSlice - skimFromGrain
		allocated = append(allocated, g)
	}
	return out, allocated
}

// TestAutoAlloc_GrainSkimFullAlloc: grain=1.0 (fully allocated), one ore unlocked.
// Expected: ore gets oreSlice=0.15, grain falls to 0.85, Σ=1.0.
func TestAutoAlloc_GrainSkimFullAlloc(t *testing.T) {
	weights := map[string]float64{GoodGrain: 1.0}
	result, allocated := applyAutoAlloc(weights, []string{GoodCopper})

	if len(allocated) != 1 || allocated[0] != GoodCopper {
		t.Fatalf("expected copper allocated, got %v", allocated)
	}
	if math.Abs(result[GoodCopper]-oreSlice) > 1e-9 {
		t.Errorf("copper weight: want %.2f, got %.6f", oreSlice, result[GoodCopper])
	}
	wantGrain := 1.0 - oreSlice
	if math.Abs(result[GoodGrain]-wantGrain) > 1e-9 {
		t.Errorf("grain weight: want %.2f, got %.6f", wantGrain, result[GoodGrain])
	}
	// Σ must stay ≤ 1.0
	var sum float64
	for _, w := range result {
		sum += w
	}
	if sum > 1.0+1e-9 {
		t.Errorf("Σ weights exceeds 1.0: %.6f", sum)
	}
}

// TestAutoAlloc_IdleCapacity: sumW=0.5 (idle ≥ oreSlice), one ore unlocked.
// Expected: ore gets oreSlice without touching grain.
func TestAutoAlloc_IdleCapacity(t *testing.T) {
	weights := map[string]float64{GoodGrain: 0.5}
	result, allocated := applyAutoAlloc(weights, []string{GoodTin})

	if len(allocated) != 1 || allocated[0] != GoodTin {
		t.Fatalf("expected tin allocated, got %v", allocated)
	}
	if math.Abs(result[GoodTin]-oreSlice) > 1e-9 {
		t.Errorf("tin weight: want %.2f, got %.6f", oreSlice, result[GoodTin])
	}
	// Grain must be untouched.
	if math.Abs(result[GoodGrain]-0.5) > 1e-9 {
		t.Errorf("grain should be untouched: want 0.50, got %.6f", result[GoodGrain])
	}
}

// TestAutoAlloc_NoCapacity: fully allocated to timber only (grain=0, idle=0).
// Expected: ore skipped because actualSlice=0.
func TestAutoAlloc_NoCapacity(t *testing.T) {
	weights2 := map[string]float64{GoodTimber: 1.0}
	result, allocated := applyAutoAlloc(weights2, []string{GoodSilver})

	// sumW=1.0, idle=0, grainW=0 → actualSlice=0 → skipped.
	if len(allocated) != 0 {
		t.Errorf("expected no allocation when no capacity, got %v", allocated)
	}
	if _, exists := result[GoodSilver]; exists && result[GoodSilver] > 0 {
		t.Errorf("silver should have no weight, got %.6f", result[GoodSilver])
	}
}

// TestAutoAlloc_SkipGrain: grain itself must never be auto-allocated.
func TestAutoAlloc_SkipGrain(t *testing.T) {
	weights := map[string]float64{}
	_, allocated := applyAutoAlloc(weights, []string{GoodGrain})
	for _, g := range allocated {
		if g == GoodGrain {
			t.Errorf("grain must never be auto-allocated via unlocked-goods path")
		}
	}
}

// TestAutoAlloc_SilverMine: the canonical use-case — silver mine built, grain=1.0.
// silver gets 0.15, grain 0.85, sum=1.0.
func TestAutoAlloc_SilverMine(t *testing.T) {
	weights := map[string]float64{GoodGrain: 1.0}
	result, allocated := applyAutoAlloc(weights, []string{GoodSilver})

	if len(allocated) != 1 || allocated[0] != GoodSilver {
		t.Fatalf("expected silver allocated, got %v", allocated)
	}
	if math.Abs(result[GoodSilver]-oreSlice) > 1e-9 {
		t.Errorf("silver: want %.2f, got %.6f", oreSlice, result[GoodSilver])
	}
	if math.Abs(result[GoodGrain]-0.85) > 1e-9 {
		t.Errorf("grain: want 0.85, got %.6f", result[GoodGrain])
	}
	var sum float64
	for _, w := range result {
		sum += w
	}
	if sum > 1.0+1e-9 {
		t.Errorf("Σ exceeds 1.0: %.6f", sum)
	}
}
