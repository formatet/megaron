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
	// GoodCult is excluded from sumW — mirrors the same exclusion in
	// AutoAllocateUnlocked (see the comment there for why: cult is additive per
	// the allocate handler's contract and must never be counted against the 1.0
	// labor budget).
	var sumW float64
	for k, w := range out {
		if k == GoodCult {
			continue
		}
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

// TestAutoAlloc_CultNotBudgeted is the regression guard for the 2026-07-25 bug:
// a temple city with grain=0.85 + cult=0.15 (settlement_labor rows sum to
// 1.00) has genuine idle capacity of 0.15 once cult is excluded — per the
// allocate handler's contract (api/handlers/province.go LaborAlloc, "cult ...
// is ADDITIVE ... NOT added to totalPct"), cult sits outside the 1.0 labor
// budget entirely, so it never shrinks grain's real 0.85 share. A newly
// unlocked ore must therefore get its full oreSlice (0.15) out of that idle
// capacity, WITHOUT skimming grain — even though naively summing all
// settlement_labor rows (grain 0.85 + cult 0.15 = 1.00) makes it look like
// there is zero idle room, which is exactly the bug: it would skim the whole
// slice from grain instead (grain 0.85 → 0.70). Before the fix, sumW counted
// cult and this test failed; after the fix cult is excluded from sumW and
// idle capacity is measured correctly.
//
// (grain=0.85+cult=0.15 is the live-world shape confirmed 2026-07-25 on CT 126
// at Asine/Gla/Gournia/Iolkos/Megara/Mykene/Palaikastro — 7 temple cities with
// no ore allocated yet. The Knossos grain=1.0+cult=0.15 number quoted in the
// symptom writeup is a saturated edge case: with grain ALONE already at 1.0,
// true idle is 0 with or without this fix, so the fix makes no observable
// difference there specifically — it changes behaviour precisely for cities
// like the seven above, where non-cult labor is below 1.0 but naive summing
// hides that idle room behind cult's additive weight.)
func TestAutoAlloc_CultNotBudgeted(t *testing.T) {
	weights := map[string]float64{GoodGrain: 0.85, GoodCult: 0.15}
	result, allocated := applyAutoAlloc(weights, []string{GoodCopper})

	if len(allocated) != 1 || allocated[0] != GoodCopper {
		t.Fatalf("expected copper allocated, got %v", allocated)
	}
	if math.Abs(result[GoodCopper]-oreSlice) > 1e-9 {
		t.Errorf("copper weight: want %.2f (full slice from idle), got %.6f", oreSlice, result[GoodCopper])
	}
	// Grain must be untouched — the slice came from idle room outside cult's
	// additive allocation, not skimmed from grain.
	if math.Abs(result[GoodGrain]-0.85) > 1e-9 {
		t.Errorf("grain must be untouched: want 0.85, got %.6f", result[GoodGrain])
	}
	// Cult itself must be untouched too.
	if math.Abs(result[GoodCult]-0.15) > 1e-9 {
		t.Errorf("cult weight must be untouched: want 0.15, got %.6f", result[GoodCult])
	}
}
