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
	const pop = 1000                    // need = 500/day
	grainOnly := CoverageDays(2000, pop) // 4 days on grain alone
	if math.Abs(grainOnly-4) > 1e-9 {
		t.Fatalf("grain-only coverage = %v, want 4", grainOnly)
	}
	// Same city, plus 8000 fish: 10000 food = 20 days. Anything that counted
	// only grain would still say 4 and trigger a release.
	withFish := CoverageDays(2000+8000, pop)
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
	const pop = 1000 // need 500/day; HighDays 30 → threshold 15000

	// Exactly at the threshold: nothing.
	if a := EvaluateGranaryAction(15000, 0, pop, cfg); a.Kind != "noop" {
		t.Errorf("at threshold: %q, want noop", a.Kind)
	}
	// 20000 food = 40 days: surplus 5000, tithe 10% = 500. NOT 10% of 20000.
	a := EvaluateGranaryAction(20000, 0, pop, cfg)
	if a.Kind != "store" {
		t.Fatalf("above threshold: %q, want store", a.Kind)
	}
	if math.Abs(a.Quantity-500) > 1e-9 {
		t.Errorf("tithe = %v, want 500 (10%% of the 5000 above the threshold, not of the whole stock)", a.Quantity)
	}
	// The tithe can never push the city below the threshold: quantity ≤ surplus.
	if a.Quantity > 20000-15000 {
		t.Errorf("tithe %v exceeds the surplus — would bite into covered food", a.Quantity)
	}
}

// TestEvaluateGranaryAction_TitheStopsAtCap: a full granary takes nothing more,
// and a nearly-full one takes only what fits. Storing past the cap would mean
// an UPDATE clipping food after the city had already been debited — the exact
// class of bug the fund's triple-gating existed to prevent.
func TestEvaluateGranaryAction_TitheStopsAtCap(t *testing.T) {
	cfg := testSitosCfg()
	const pop = 1000 // cap = 500 × 60 = 30000

	if a := EvaluateGranaryAction(100000, 30000, pop, cfg); a.Kind != "noop" {
		t.Errorf("full granary: %q, want noop", a.Kind)
	}
	// 200 units of headroom against a tithe that would otherwise be 8500.
	a := EvaluateGranaryAction(100000, 29800, pop, cfg)
	if a.Kind != "store" || math.Abs(a.Quantity-200) > 1e-9 {
		t.Errorf("near-full granary: %s %v, want store 200 (headroom-bound)", a.Kind, a.Quantity)
	}
}

// TestEvaluateGranaryAction_FamineRelief: below LowDays the granary releases up
// to the shortfall. This is the case E1 forbade and B2 struck — the fund was
// never allowed to be famine relief, and on top of that its momentum-detector
// trigger was silent in a stable famine anyway.
func TestEvaluateGranaryAction_FamineRelief(t *testing.T) {
	cfg := testSitosCfg()
	const pop = 1000 // need 500/day; LowDays 10 → 5000 food

	// 2 days of food, plenty in the granary: release the 4000 shortfall.
	a := EvaluateGranaryAction(1000, 20000, pop, cfg)
	if a.Kind != "release" || math.Abs(a.Quantity-4000) > 1e-9 {
		t.Errorf("famine with a full granary: %s %v, want release 4000", a.Kind, a.Quantity)
	}
	// Never more than the shortfall, even with a huge granary — the granary
	// tops the city up to LowDays, it does not empty itself into it.
	if a.Quantity > 5000-1000 {
		t.Errorf("released %v, more than the shortfall", a.Quantity)
	}
}

// TestEvaluateGranaryAction_EmptyGranaryCannotHelp: the ONLY limit on famine
// relief (B2). A city in famine with an empty granary gets nothing — not a
// smaller amount, nothing — and that has to be a state the model can be in,
// or the reserve is not a reserve.
func TestEvaluateGranaryAction_EmptyGranaryCannotHelp(t *testing.T) {
	cfg := testSitosCfg()
	if a := EvaluateGranaryAction(100, 0, 1000, cfg); a.Kind != "noop" {
		t.Errorf("famine with an empty granary: %q, want noop", a.Kind)
	}
	// A granary with less than the shortfall gives exactly what it has.
	a := EvaluateGranaryAction(1000, 300, 1000, cfg)
	if a.Kind != "release" || math.Abs(a.Quantity-300) > 1e-9 {
		t.Errorf("partial granary: %s %v, want release 300 (all it holds)", a.Kind, a.Quantity)
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
