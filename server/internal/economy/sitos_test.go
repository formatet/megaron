package economy

import (
	"math"
	"testing"
)

func testSitosCfg() SitosConfig {
	return SitosConfig{
		TaxRate:             0.01,
		RefPriceFloor:       0.5,
		RefPriceCeiling:     3.0,
		FundCapMult:         20,
		StartingFundDays:    10,
		PriceSmoothingTicks: 6,
		SubsistenceGoods:    []string{"grain", "fish"},
		SilverLiquidCapDays: 10,
		SilverStartDays:     5,
	}
}

// TestFundCap_PopInvariant: the fund's cap, expressed in days-of-grain-need,
// is exactly FundCapMult regardless of population. A tiny colony and a huge
// capital get the same coverage fraction — the plan's pop-invariance property.
func TestFundCap_PopInvariant(t *testing.T) {
	cfg := testSitosCfg()
	const base = 3.0
	for _, pop := range []int{100, 20000} {
		need := dailyGrainNeedInSilver(pop, base)
		if need <= 0 {
			t.Fatalf("need should be positive for pop=%d", pop)
		}
		coverageDays := FundCap(pop, base, cfg) / need
		if math.Abs(coverageDays-cfg.FundCapMult) > 1e-9 {
			t.Errorf("pop=%d coverage-days=%.6f, want FundCapMult=%.1f", pop, coverageDays, cfg.FundCapMult)
		}
	}
}

// TestGenesisFundSeed_PopInvariant: the genesis seed, in days-of-grain-need, is
// exactly StartingFundDays for any population.
func TestGenesisFundSeed_PopInvariant(t *testing.T) {
	cfg := testSitosCfg()
	const base = 3.0
	for _, pop := range []int{100, 20000} {
		need := dailyGrainNeedInSilver(pop, base)
		seed, cap := GenesisFundSeed(pop, base, cfg)
		if math.Abs(seed/need-cfg.StartingFundDays) > 1e-9 {
			t.Errorf("pop=%d seed-days=%.6f, want %.1f", pop, seed/need, cfg.StartingFundDays)
		}
		if math.Abs(cap/need-cfg.FundCapMult) > 1e-9 {
			t.Errorf("pop=%d cap-days=%.6f, want %.1f", pop, cap/need, cfg.FundCapMult)
		}
	}
}

// TestGenesisSilverLiquid_PopInvariant: the genesis LIQUID silver seed and cap,
// in days-of-grain-need, are exactly SilverStartDays / SilverLiquidCapDays for
// any population — pop-invariant, same shape as GenesisFundSeed.
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

// TestEvaluateSitosAction_Exhaustion: an empty fund cannot buy surplus (noop),
// and a nearly-empty fund never spends more silver than it has.
func TestEvaluateSitosAction_Exhaustion(t *testing.T) {
	reference := 30.0
	stock := 90.0 // well above reference → surplus
	refPrice := 1.0
	actualPrice := 0.4 // < ref → surplus condition

	// Empty fund → noop.
	a := EvaluateSitosAction(refPrice, actualPrice, stock, reference, 0, 2000, 50, 1000)
	if a.Kind != "noop" {
		t.Errorf("empty fund should noop on surplus, got %+v", a)
	}

	// Small fund → buys, but never spends more than it holds.
	fundSilver := 5.0
	a = EvaluateSitosAction(refPrice, actualPrice, stock, reference, fundSilver, 2000, 50, 1000)
	if a.Kind != "buy" {
		t.Fatalf("small fund should still buy on surplus, got %+v", a)
	}
	if a.SilverMoved > fundSilver+1e-9 {
		t.Errorf("buy spent %.4f > fund %.4f — would drive fund negative", a.SilverMoved, fundSilver)
	}
}

// TestEvaluateSitosAction_SettlementCantPay: a shortage where the settlement has
// no silver → noop (the fund never gives grain away for free).
func TestEvaluateSitosAction_SettlementCantPay(t *testing.T) {
	reference := 30.0
	stock := 5.0       // below reference → shortage
	refPrice := 1.0
	actualPrice := 2.5 // > ref → shortage condition

	a := EvaluateSitosAction(refPrice, actualPrice, stock, reference, 500, 2000, 0, 1000)
	if a.Kind != "noop" {
		t.Errorf("settlement with no silver should noop on shortage, got %+v", a)
	}
}

// TestEvaluateSitosAction_CapHeadroomGating covers the triple-gate conservation
// fix: neither leg may overshoot the RECEIVING party's cap headroom, so no silver
// is ever silently clipped after leaving the other party.
func TestEvaluateSitosAction_CapHeadroomGating(t *testing.T) {
	reference := 30.0
	refPrice := 1.0

	// Sell leg: settlement has plenty of silver, but the fund is almost at cap.
	// SilverMoved must not exceed fund headroom.
	fundSilver, fundCap := 1990.0, 2000.0
	a := EvaluateSitosAction(refPrice, 2.5, 5.0, reference, fundSilver, fundCap, 1000, 1000)
	if a.Kind != "sell" {
		t.Fatalf("expected sell, got %+v", a)
	}
	if a.SilverMoved > (fundCap-fundSilver)+1e-9 {
		t.Errorf("sell moved %.4f silver > fund headroom %.4f — would overshoot cap", a.SilverMoved, fundCap-fundSilver)
	}

	// Buy leg: fund has plenty, but the settlement's silver is almost at its cap.
	// SilverMoved must not exceed the settlement's silver headroom.
	settlementSilver, settlementSilverCap := 995.0, 1000.0
	a = EvaluateSitosAction(refPrice, 0.4, 90.0, reference, 5000, 10000, settlementSilver, settlementSilverCap)
	if a.Kind != "buy" {
		t.Fatalf("expected buy, got %+v", a)
	}
	if a.SilverMoved > (settlementSilverCap-settlementSilver)+1e-9 {
		t.Errorf("buy moved %.4f silver > settlement headroom %.4f", a.SilverMoved, settlementSilverCap-settlementSilver)
	}
}

// TestRefPrice_SmoothingDampensShock: a sudden stock jump moves the smoothed
// RefPrice less between adjacent ticks than the raw LocalPrice would for the same
// jump — the plan's "chock hoppar ej mer" requirement as a comparative assertion.
func TestRefPrice_SmoothingDampensShock(t *testing.T) {
	cfg := testSitosCfg()
	const base = 3.0

	// A settlement draining fast (rate<0): stock at calc_tick=100 is 20, dropping
	// 5/tick. Compare price change between currentTick=100 and 101.
	amount, rate, calcTick := 20.0, -5.0, 100.0

	rawAt := func(tk int) float64 {
		stock := amount + rate*(float64(tk)-calcTick)
		if stock < 0 {
			stock = 0
		}
		return LocalPrice(base, stock, rate)
	}
	rawDelta := math.Abs(rawAt(101) - rawAt(100))
	smoothDelta := math.Abs(
		RefPrice(base, amount, rate, calcTick, 101, cfg) -
			RefPrice(base, amount, rate, calcTick, 100, cfg))

	if smoothDelta > rawDelta+1e-9 {
		t.Errorf("smoothing should dampen tick-to-tick shock: smoothDelta=%.4f rawDelta=%.4f", smoothDelta, rawDelta)
	}
}

// TestRefPrice_ClampsToFloorCeiling: extreme stocks still yield a price in range.
func TestRefPrice_ClampsToFloorCeiling(t *testing.T) {
	cfg := testSitosCfg()
	const base = 3.0

	// Empty & draining → would price very high → clamp to ceiling.
	high := RefPrice(base, 0, -10, 100, 106, cfg)
	if high > cfg.RefPriceCeiling+1e-9 || high < cfg.RefPriceFloor-1e-9 {
		t.Errorf("high-shortage refPrice %.4f out of [%.1f,%.1f]", high, cfg.RefPriceFloor, cfg.RefPriceCeiling)
	}

	// Stock far above its rate-derived reference (rate=10 → reference=720) →
	// deep surplus → would price very low → clamp to floor.
	low := RefPrice(base, 2000, 10, 100, 106, cfg)
	if low < cfg.RefPriceFloor-1e-9 || low > cfg.RefPriceCeiling+1e-9 {
		t.Errorf("low-surplus refPrice %.4f out of [%.1f,%.1f]", low, cfg.RefPriceFloor, cfg.RefPriceCeiling)
	}
}
