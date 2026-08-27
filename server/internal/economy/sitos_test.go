package economy

import (
	"math"
	"testing"
)

func testSitosCfg() SitosConfig {
	return SitosConfig{
		GranaryCapDays:      60,
		LowDays:             10,
		HighDays:            30,
		TithePct:            0.1,
		SubsistenceGoods:    []string{"grain", "fish"},
		SilverLiquidCapDays: 10,
		SilverStartDays:     5,
	}
}

// TestGranaryCap_PopInvariant: the granary's cap, expressed in days of the
// city's food need, is exactly GranaryCapDays regardless of population. A tiny
// colony and a huge capital are covered for the same number of days — the
// pop-invariance property the fund had and the granary keeps.
func TestGranaryCap_PopInvariant(t *testing.T) {
	cfg := testSitosCfg()
	for _, pop := range []int{100, 20000} {
		need := DailyFoodNeed(pop)
		if need <= 0 {
			t.Fatalf("need should be positive for pop=%d", pop)
		}
		days := GranaryCap(pop, cfg) / need
		if math.Abs(days-cfg.GranaryCapDays) > 1e-9 {
			t.Errorf("pop=%d cap-days=%.6f, want GranaryCapDays=%.1f", pop, days, cfg.GranaryCapDays)
		}
	}
}

// TestGenesisSilverLiquid_PopInvariant: the genesis LIQUID silver seed and cap,
// in days-of-grain-need, are exactly SilverStartDays / SilverLiquidCapDays for
// any population. This is now one of only TWO silver faucets in the game (B3),
// so its shape matters more than it did when the fund also minted.
func TestGenesisSilverLiquid_PopInvariant(t *testing.T) {
	cfg := testSitosCfg()
	const base = 3.0
	for _, pop := range []int{100, 20000} {
		need := dailyGrainNeedInSilver(pop, base)
		seed, cap := GenesisSilverLiquid(pop, base, cfg)
		if math.Abs(seed/need-cfg.SilverStartDays) > 1e-9 {
			t.Errorf("pop=%d seed-days=%.6f, want %.1f", pop, seed/need, cfg.SilverStartDays)
		}
		if math.Abs(cap/need-cfg.SilverLiquidCapDays) > 1e-9 {
			t.Errorf("pop=%d cap-days=%.6f, want %.1f", pop, cap/need, cfg.SilverLiquidCapDays)
		}
	}
}

// TestCoverageDays_CountsTheWholeBasket: coverage is food units over the ONE
// food need (B6), not grain over a grain need. A city fed mostly on fish is not
// starving, and a coverage figure that only saw grain would say it was — and
// would then release a reserve it did not need. The population has a single
// food need covered grain-first, fish-for-the-rest (FoodConsumptionSplit).
func TestCoverageDays_CountsTheWholeBasket(t *testing.T) {
	const pop = 1000                  // need = 5/day (mig 136, GrainConsumptionPerCitizenPerTick ÷100, not ÷43.2: 500→5)
	grainOnly := CoverageDays(20, pop) // 2000 → 20 (mig 136, same ÷100); 4 days on grain alone
	if math.Abs(grainOnly-4) > 1e-9 {
		t.Fatalf("grain-only coverage = %v, want 4", grainOnly)
	}
	// Same city, plus 80 fish (8000 → 80, mig 136 ÷100): 100 food = 20 days.
	// Anything that counted only grain would still say 4 and trigger a release.
	withFish := CoverageDays(20+80, pop)
	if math.Abs(withFish-20) > 1e-9 {
		t.Errorf("basket coverage = %v, want 20 (grain+fish over one food need)", withFish)
	}
}

// TestCoverageDays_NoPeopleNoNeed: a settlement with no population has no food
// need. Reported as 0 rather than +Inf, and EvaluateGranaryAction leaves it be —
// division by a zero need must not become a release of everything.
func TestCoverageDays_NoPeopleNoNeed(t *testing.T) {
	if got := CoverageDays(500, 0); got != 0 {
		t.Errorf("coverage with pop=0 = %v, want 0", got)
	}
	if a := EvaluateGranaryAction(0, 5000, 0, testSitosCfg()); a.Kind != "noop" {
		t.Errorf("action with pop=0 = %q, want noop", a.Kind)
	}
}

// TestEvaluateGranaryAction_TitheOnlyTakesAboveTheThreshold: the tithe is taken
// from the surplus ABOVE HighDays, never from the stock as a whole. That is
// what makes it self-limiting — no separate "don't tax a poor city" rule is
// needed, and a city exactly at the threshold is left alone.
func TestEvaluateGranaryAction_TitheOnlyTakesAboveTheThreshold(t *testing.T) {
	cfg := testSitosCfg()
	const pop = 1000 // need 5/day (mig 136, ÷100: 500→5); HighDays 30 → threshold 150 (15000→150)

	// Exactly at the threshold: nothing.
	if a := EvaluateGranaryAction(150, 0, pop, cfg); a.Kind != "noop" {
		t.Errorf("at threshold: %q, want noop", a.Kind)
	}
	// 200 food (20000→200, mig 136 ÷100) = 40 days: surplus 50, tithe 10% = 5. NOT 10% of 200.
	a := EvaluateGranaryAction(200, 0, pop, cfg)
	if a.Kind != "store" {
		t.Fatalf("above threshold: %q, want store", a.Kind)
	}
	if math.Abs(a.Quantity-5) > 1e-9 {
		t.Errorf("tithe = %v, want 5 (10%% of the 50 above the threshold, not of the whole stock)", a.Quantity)
	}
	// The tithe can never push the city below the threshold: quantity ≤ surplus.
	if a.Quantity > 200-150 {
		t.Errorf("tithe %v exceeds the surplus — would bite into covered food", a.Quantity)
	}
}

// TestEvaluateGranaryAction_TitheStopsAtCap: a full granary takes nothing more,
// and a nearly-full one takes only what fits. Storing past the cap would mean
// an UPDATE clipping food after the city had already been debited — the exact
// class of bug the fund's triple-gating existed to prevent.
func TestEvaluateGranaryAction_TitheStopsAtCap(t *testing.T) {
	cfg := testSitosCfg()
	const pop = 1000 // cap = 5 × 60 = 300 (mig 136, ÷100: need 500→5, cap 30000→300)

	if a := EvaluateGranaryAction(1000, 300, pop, cfg); a.Kind != "noop" {
		t.Errorf("full granary: %q, want noop", a.Kind)
	}
	// 2 units of headroom (200→2) against a tithe that would otherwise be 85 (8500→85).
	a := EvaluateGranaryAction(1000, 298, pop, cfg)
	if a.Kind != "store" || math.Abs(a.Quantity-2) > 1e-9 {
		t.Errorf("near-full granary: %s %v, want store 2 (headroom-bound)", a.Kind, a.Quantity)
	}
}

// TestEvaluateGranaryAction_FamineRelief: below LowDays the granary releases up
// to the shortfall. This is the case E1 forbade and B2 struck — the fund was
// never allowed to be famine relief, and on top of that its momentum-detector
// trigger was silent in a stable famine anyway.
func TestEvaluateGranaryAction_FamineRelief(t *testing.T) {
	cfg := testSitosCfg()
	const pop = 1000 // need 5/day (mig 136, ÷100: 500→5); LowDays 10 → 50 food (5000→50)

	// 2 days of food (1000→10), plenty in the granary (20000→200): release the
	// 40 shortfall (4000→40).
	a := EvaluateGranaryAction(10, 200, pop, cfg)
	if a.Kind != "release" || math.Abs(a.Quantity-40) > 1e-9 {
		t.Errorf("famine with a full granary: %s %v, want release 40", a.Kind, a.Quantity)
	}
	// Never more than the shortfall, even with a huge granary — the granary
	// tops the city up to LowDays, it does not empty itself into it.
	if a.Quantity > 50-10 {
		t.Errorf("released %v, more than the shortfall", a.Quantity)
	}
}

// TestEvaluateGranaryAction_EmptyGranaryCannotHelp: the ONLY limit on famine
// relief (B2). A city in famine with an empty granary gets nothing — not a
// smaller amount, nothing — and that has to be a state the model can be in,
// or the reserve is not a reserve.
func TestEvaluateGranaryAction_EmptyGranaryCannotHelp(t *testing.T) {
	cfg := testSitosCfg()
	// 100 → 1 (mig 136, ÷100 — same food-unit scale as DailyFoodNeed).
	if a := EvaluateGranaryAction(1, 0, 1000, cfg); a.Kind != "noop" {
		t.Errorf("famine with an empty granary: %q, want noop", a.Kind)
	}
	// A granary with less than the shortfall gives exactly what it has.
	// 1000→10, 300→3 (mig 136, ÷100).
	a := EvaluateGranaryAction(10, 3, 1000, cfg)
	if a.Kind != "release" || math.Abs(a.Quantity-3) > 1e-9 {
		t.Errorf("partial granary: %s %v, want release 3 (all it holds)", a.Kind, a.Quantity)
	}
}

// TestEvaluateGranaryAction_BandIsQuiet: between the thresholds nothing happens.
// The fund's failure was the opposite — it fired whenever the stock MOVED and
// was silent at every equilibrium, including a stable famine.
func TestEvaluateGranaryAction_BandIsQuiet(t *testing.T) {
	cfg := testSitosCfg()
	const pop = 1000
	for _, days := range []float64{10, 15, 25, 30} {
		stock := days * DailyFoodNeed(pop)
		if a := EvaluateGranaryAction(stock, 10000, pop, cfg); a.Kind != "noop" {
			t.Errorf("coverage %.0f days: %q, want noop", days, a.Kind)
		}
	}
}
