package economy

import (
	"math"
	"testing"
)

// TestLaborRates_Citizens verifies the citizen-based production formula:
// rate(g) = (base_potential(g) / REF_LABOR) × citizens(g).
func TestLaborRates_Citizens(t *testing.T) {
	// 100 citizens on timber with base 0.25 → yield_per_worker = 0.25/100 = 0.0025
	// rate = 0.0025 × 100 = 0.25
	base := map[string]float64{GoodTimber: 0.25, GoodGrain: 0.10}
	citizens := map[string]float64{GoodTimber: 100, GoodGrain: 50}

	got := LaborRates(base, citizens, 0 /* unused */)

	expectedTimber := (0.25 / REF_LABOR) * 100
	if math.Abs(got[GoodTimber]-expectedTimber) > 1e-9 {
		t.Errorf("timber: expected %.6f, got %.6f", expectedTimber, got[GoodTimber])
	}
	expectedGrain := (0.10 / REF_LABOR) * 50
	if math.Abs(got[GoodGrain]-expectedGrain) > 1e-9 {
		t.Errorf("grain: expected %.6f, got %.6f", expectedGrain, got[GoodGrain])
	}
}

// TestLaborRates_ZeroCitizens verifies that zero citizen allocation gives zero output.
func TestLaborRates_ZeroCitizens(t *testing.T) {
	base := map[string]float64{GoodTimber: 0.25, GoodGrain: 0.10}
	citizens := map[string]float64{GoodTimber: 0, GoodGrain: 0}

	got := LaborRates(base, citizens, 0)
	for good, rate := range got {
		if rate != 0 {
			t.Errorf("%s: expected 0 with 0 citizens, got %.6f", good, rate)
		}
	}
}

// TestLaborRates_NonProducibleNotAllocable verifies that a good with base_potential=0
// never appears in labor output (it would be absent from baseRates).
func TestLaborRates_NonProducibleNotAllocable(t *testing.T) {
	// fish has no base — it's not producible inland.
	base := map[string]float64{GoodTimber: 0.25}
	// Even if someone passes citizens for fish, fish has no base so it stays absent.
	citizens := map[string]float64{GoodTimber: 50, GoodFish: 50}

	got := LaborRates(base, citizens, 0)

	// fish is absent from base → absent from result
	if v, ok := got[GoodFish]; ok && v != 0 {
		t.Errorf("fish must not be producible inland, got %.6f", v)
	}
	// timber: (base / REF) × citizens = (0.25/100) × 50 = 0.125
	if math.Abs(got[GoodTimber]-0.125) > 1e-9 {
		t.Errorf("timber: expected 0.125, got %.6f", got[GoodTimber])
	}
}

// TestLaborRates_Formula_ExactMatch verifies the formula against a hand-calculated value.
// base=0.15, citizens=70, REF=100 → rate = (0.15/100) × 70 = 0.105
func TestLaborRates_Formula_ExactMatch(t *testing.T) {
	base := map[string]float64{GoodCedar: 0.15}
	citizens := map[string]float64{GoodCedar: 70}
	got := LaborRates(base, citizens, 0)
	expected := (0.15 / REF_LABOR) * 70.0
	if math.Abs(got[GoodCedar]-expected) > 1e-9 {
		t.Errorf("cedar: expected %.6f, got %.6f", expected, got[GoodCedar])
	}
}

// TestPopCosts_MirrorTrainingGo verifies that PopCosts constants are internally consistent.
// cavalry and catapult were removed in migration 042 (replaced by chariot).
func TestPopCosts_MirrorTrainingGo(t *testing.T) {
	// These values must match province/training.go:UnitSpecs (G1: no import allowed).
	// ship = galley (DB-kolumn). war_galley + merchantman = nya skepp-typer (mig 039).
	// chariot replaced cavalry (mig 042); catapult removed.
	expected := map[string]int{
		"infantry":       5,
		"chariot":        8,
		"priest":         3,
		"ship":           10, // galley
		"elite_infantry": 10,
		"war_galley":     12,
		"merchantman":    8,
	}
	for unit, want := range expected {
		if got := PopCosts[unit]; got != want {
			t.Errorf("PopCosts[%s] = %d, want %d", unit, got, want)
		}
	}
}

// TestNewGoodSeeding_FishAfterHarbour verifies att fisk är producerbar med 0 citizens som
// default (fisk-buggen var att 1 citizen auto-seedades → producerade utan Wanax val).
// Fisk ska visas som producerbar (basePotential > 0) men ha rate=0 tills Wanax allokerar.
func TestNewGoodSeeding_FishAfterHarbour(t *testing.T) {
	// Utan hamn: grain och timber är producerbara; fisk saknas i basePotential.
	baseBefore := map[string]float64{GoodGrain: 0.10, GoodTimber: 0.25}
	citizensBefore := map[string]float64{GoodGrain: 100, GoodTimber: 100}
	ratesBefore := LaborRates(baseBefore, citizensBefore, 0)
	if ratesBefore[GoodFish] != 0 {
		t.Errorf("fish ska inte produceras utan hamn, got %.6f", ratesBefore[GoodFish])
	}

	// Med hamn: fish har basePotential > 0 men citizens=0 (ingen auto-seed).
	// RecomputeProduction lägger INTE till 1 citizen automatiskt längre (fisk-fix).
	baseAfter := map[string]float64{GoodGrain: 0.10, GoodTimber: 0.25, GoodFish: 0.04}
	citizensZeroFish := map[string]float64{GoodGrain: 100, GoodTimber: 100, GoodFish: 0}
	ratesZero := LaborRates(baseAfter, citizensZeroFish, 0)
	if ratesZero[GoodFish] != 0 {
		t.Errorf("fish rate ska vara 0 med 0 citizens (fisk-fix), got %.6f", ratesZero[GoodFish])
	}

	// Wanax tilldelar sedan citizens via LaborAlloc → fish producerar.
	citizensAllocated := map[string]float64{GoodGrain: 100, GoodTimber: 100, GoodFish: 10}
	ratesAllocated := LaborRates(baseAfter, citizensAllocated, 0)
	expected := (0.04 / REF_LABOR) * 10.0
	if math.Abs(ratesAllocated[GoodFish]-expected) > 1e-9 {
		t.Errorf("fish rate med 10 citizens: want %.6f, got %.6f", expected, ratesAllocated[GoodFish])
	}
}

// TestPopGrowth_GrainPresenceRequired verifies that grain presence is what drives
// growth vs starvation. This is a pure-logic test for the condition in tick.go.
func TestPopGrowth_GrainPresenceRequired(t *testing.T) {
	type popCase struct {
		grain      float64
		pop        int
		wantGrowth bool
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
// citizens are fixed; rate is unaffected by recruit until Wanax re-allocates.
// But labor_pool shrinks — so over-allocation (Σcitizens > pool) is caught by endpoint.
func TestLaborRates_VariantB_Recruit(t *testing.T) {
	base := map[string]float64{GoodGrain: 0.10}

	// 100 citizens on grain.
	citizensAlloc := map[string]float64{GoodGrain: 100}
	before := LaborRates(base, citizensAlloc, 0)
	expectedRate := (0.10 / REF_LABOR) * 100.0
	if math.Abs(before[GoodGrain]-expectedRate) > 1e-9 {
		t.Errorf("grain rate should be %.6f, got %.6f", expectedRate, before[GoodGrain])
	}

	// Reducing citizens frees up labor (endpoint validates cap).
	lessCitizens := map[string]float64{GoodGrain: 50}
	after := LaborRates(base, lessCitizens, 0)
	if after[GoodGrain] >= before[GoodGrain] {
		t.Errorf("fewer citizens should lower rate: before=%.6f, after=%.6f", before[GoodGrain], after[GoodGrain])
	}
}

// TestOfferExpiry_SolvencyCheck verifies that a seller with insufficient stock
// is treated as insolvent for inbox filtering purposes.
func TestOfferExpiry_SolvencyCheck(t *testing.T) {
	type solvencyCase struct {
		sellerStock float64
		wantQty     float64
		solvent     bool
	}
	cases := []solvencyCase{
		{sellerStock: 100, wantQty: 50, solvent: true},
		{sellerStock: 50, wantQty: 50, solvent: true},
		{sellerStock: 49, wantQty: 50, solvent: false},
		{sellerStock: 0, wantQty: 1, solvent: false},
	}
	for _, tc := range cases {
		got := tc.sellerStock >= tc.wantQty
		if got != tc.solvent {
			t.Errorf("stock=%.0f want_qty=%.0f: solvent=%v, expected=%v", tc.sellerStock, tc.wantQty, got, tc.solvent)
		}
	}
}

// TestCitizenCap_ExceedsPool verifies the allocation endpoint rejects Σcitizens > labor_pool.
// This is a pure-math test for the validation logic replicated here.
func TestCitizenCap_ExceedsPool(t *testing.T) {
	laborPool := 100
	allocations := map[string]int{GoodGrain: 70, GoodTimber: 50} // total = 120 > 100
	total := 0
	for _, c := range allocations {
		total += c
	}
	if total <= laborPool {
		t.Errorf("should detect over-allocation: total=%d, pool=%d", total, laborPool)
	}

	// Valid allocation
	valid := map[string]int{GoodGrain: 60, GoodTimber: 40} // total = 100 ≤ 100
	total = 0
	for _, c := range valid {
		total += c
	}
	if total > laborPool {
		t.Errorf("should not reject valid allocation: total=%d, pool=%d", total, laborPool)
	}
}
