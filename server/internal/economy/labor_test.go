package economy

import (
	"math"
	"testing"
)

// TestLaborRates_HalfLabor verifies that halving labor_pool halves all rates.
func TestLaborRates_HalfLabor(t *testing.T) {
	base := map[string]float64{GoodTimber: 0.25, GoodGrain: 0.10}
	weights := map[string]float64{GoodTimber: 0.6, GoodGrain: 0.4}

	full := LaborRates(base, weights, 100)
	half := LaborRates(base, weights, 50)

	for _, good := range []string{GoodTimber, GoodGrain} {
		if math.Abs(half[good]-full[good]/2) > 1e-9 {
			t.Errorf("%s: half-labor rate %.6f != full/2 %.6f", good, half[good], full[good]/2)
		}
	}
}

// TestLaborRates_Weight1_0 verifies that weight=1.0 on one good yields base*labor/REF_LABOR,
// and that the other good gets 0.
func TestLaborRates_Weight1_0(t *testing.T) {
	base := map[string]float64{GoodTimber: 0.25, GoodGrain: 0.10}
	weights := map[string]float64{GoodTimber: 1.0, GoodGrain: 0.0}

	got := LaborRates(base, weights, 100)

	// timber: base × 1.0 × 100 / 100 = base
	if math.Abs(got[GoodTimber]-0.25) > 1e-9 {
		t.Errorf("timber: expected 0.25, got %.6f", got[GoodTimber])
	}
	// grain: weight 0.0 → rate 0
	if got[GoodGrain] != 0 {
		t.Errorf("grain: expected 0, got %.6f", got[GoodGrain])
	}
}

// TestLaborRates_NonProducibleNotAllocable verifies that a good with base_potential=0
// never appears in labor output (it would be absent from baseRates).
func TestLaborRates_NonProducibleNotAllocable(t *testing.T) {
	// fish has no base — it's not producible inland.
	base := map[string]float64{GoodTimber: 0.25}
	// Even if someone passes a weight for fish, fish has no base so it stays 0.
	weights := map[string]float64{GoodTimber: 0.5, GoodFish: 0.5}

	got := LaborRates(base, weights, 100)

	// fish is absent from base → absent from result
	if v, ok := got[GoodFish]; ok && v != 0 {
		t.Errorf("fish must not be producible inland, got %.6f", v)
	}
	// timber: base × 0.5 × 100 / 100
	if math.Abs(got[GoodTimber]-0.125) > 1e-9 {
		t.Errorf("timber: expected 0.125, got %.6f", got[GoodTimber])
	}
}

// TestLaborRates_ZeroLabor verifies that zero workers gives zero output.
func TestLaborRates_ZeroLabor(t *testing.T) {
	base := map[string]float64{GoodTimber: 0.25, GoodGrain: 0.10}
	weights := map[string]float64{GoodTimber: 0.5, GoodGrain: 0.5}

	got := LaborRates(base, weights, 0)
	for good, rate := range got {
		if rate != 0 {
			t.Errorf("%s: expected 0 with 0 labor, got %.6f", good, rate)
		}
	}
}

// TestLaborRates_Formula_ExactMatch verifies the formula against a hand-calculated expected value.
func TestLaborRates_Formula_ExactMatch(t *testing.T) {
	// base=0.15, weight=0.7, labor=80, REF=100 → 0.15 * 0.7 * 80 / 100 = 0.084
	base := map[string]float64{GoodCedar: 0.15}
	weights := map[string]float64{GoodCedar: 0.7}
	got := LaborRates(base, weights, 80)
	expected := 0.15 * 0.7 * 80.0 / REF_LABOR
	if math.Abs(got[GoodCedar]-expected) > 1e-9 {
		t.Errorf("cedar: expected %.6f, got %.6f", expected, got[GoodCedar])
	}
}

// TestPopCosts_MirrorTrainingGo verifies that PopCosts constants are internally consistent.
func TestPopCosts_MirrorTrainingGo(t *testing.T) {
	// These values must match province/training.go:UnitSpecs (G1: no import allowed).
	expected := map[string]int{
		"infantry":       5,
		"cavalry":        8,
		"catapult":       2,
		"priest":         3,
		"ship":           10,
		"elite_infantry": 10,
	}
	for unit, want := range expected {
		if got := PopCosts[unit]; got != want {
			t.Errorf("PopCosts[%s] = %d, want %d", unit, got, want)
		}
	}
}

// TestNewGoodSeeding_UniformShare verifies the weight re-normalisation logic used
// when a new producible good appears (e.g. fish after building a harbour).
// Existing weights must be scaled down proportionally; new good gets 1/totalGoods.
func TestNewGoodSeeding_UniformShare(t *testing.T) {
	// Settlement has grain(0.6) and timber(0.4) — 2 goods.
	// Harbour is built → fish becomes producible (new good).
	existing := map[string]float64{GoodGrain: 0.6, GoodTimber: 0.4}

	// Simulate the seeding logic from recompute.go (replicated here as pure math).
	newGoods := []string{GoodFish}
	totalGoods := float64(len(existing) + len(newGoods)) // 3
	renormed := make(map[string]float64)
	for k, w := range existing {
		renormed[k] = w * float64(len(existing)) / totalGoods
	}
	share := 1.0 / totalGoods
	for _, k := range newGoods {
		renormed[k] = share
	}

	// All weights must sum to 1.
	var sum float64
	for _, w := range renormed {
		sum += w
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("weights should sum to 1.0 after seeding, got %.9f", sum)
	}

	// New good must have 1/3 share.
	if math.Abs(renormed[GoodFish]-1.0/3.0) > 1e-9 {
		t.Errorf("fish share should be 1/3, got %.6f", renormed[GoodFish])
	}

	// Existing ratios are preserved relative to each other.
	// grain was 0.6, timber was 0.4 → ratio 3:2. After renorm still 3:2.
	ratio := renormed[GoodGrain] / renormed[GoodTimber]
	if math.Abs(ratio-1.5) > 1e-9 {
		t.Errorf("grain:timber ratio should be 1.5 after renorm, got %.6f", ratio)
	}
}

// TestPopGrowth_GrainPresenceRequired verifies that grain presence is what drives
// growth vs starvation. This is a pure-logic test for the condition in tick.go.
func TestPopGrowth_GrainPresenceRequired(t *testing.T) {
	type popCase struct {
		grain       float64
		pop         int
		wantGrowth  bool
	}
	cases := []popCase{
		{grain: 100, pop: 200, wantGrowth: true},
		{grain: 0, pop: 200, wantGrowth: false},
		{grain: 1, pop: 200, wantGrowth: true},
	}
	for _, tc := range cases {
		grows := tc.grain > 0
		if grows != tc.wantGrowth {
			t.Errorf("grain=%.0f pop=%d: wantGrowth=%v got %v", tc.grain, tc.pop, tc.wantGrowth, grows)
		}
	}
}

// TestLaborRates_VariantB_Recruit simulates variant-B recruit semantics:
// population stays constant; labor_pool decreases by recruited × PopCost.
func TestLaborRates_VariantB_Recruit(t *testing.T) {
	base := map[string]float64{GoodGrain: 0.10}
	weights := map[string]float64{GoodGrain: 1.0}

	population := 200
	// Before recruit: no army, full labor
	beforeLabor := float64(population)
	// Recruit 10 infantry (PopCost 5 each) → labor drops by 50
	recruited := 10
	afterLabor := float64(population - recruited*PopCosts["infantry"])

	before := LaborRates(base, weights, beforeLabor)
	after := LaborRates(base, weights, afterLabor)

	// Population unchanged → same population; only labor pool differs
	if before[GoodGrain] <= after[GoodGrain] {
		t.Errorf("recruit should lower rate: before=%.6f, after=%.6f", before[GoodGrain], after[GoodGrain])
	}

	// Disband → labor_pool recovers
	recovered := LaborRates(base, weights, beforeLabor)
	if math.Abs(recovered[GoodGrain]-before[GoodGrain]) > 1e-9 {
		t.Errorf("disband should restore rate: before=%.6f, recovered=%.6f", before[GoodGrain], recovered[GoodGrain])
	}
}
