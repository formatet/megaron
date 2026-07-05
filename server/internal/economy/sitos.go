package economy

import "math"

// dailyGrainNeedInSilver is the anchor for every Sitos capacity figure (fund
// cap, genesis seed): a settlement's daily grain need (pop × 0.5, mirroring
// RecomputeProduction's grainConsumptionPerTick × TicksPerDay) priced at
// grain's base_value. Binding capacity to population keeps coverage
// pop-invariant: a pop=100 colony and a pop=20000 capital are both covered
// for the same number of days.
func dailyGrainNeedInSilver(population int, grainBaseValue float64) float64 {
	if population < 0 {
		population = 0
	}
	return float64(population) * 0.5 * grainBaseValue
}

// FundCap returns a settlement's Sitos fund capacity for its current population.
func FundCap(population int, grainBaseValue float64, cfg SitosConfig) float64 {
	return dailyGrainNeedInSilver(population, grainBaseValue) * cfg.FundCapMult
}

// GenesisFundSeed returns the one-time starting silver for a new settlement's
// Sitos fund (join.go capital creation, foundColony colony creation) and its
// cap. Both are pure pop×const formulas so a small colony and a large capital
// get proportionally identical treatment. Silver invariant: this seed is a
// deliberate exception (documented in temenos_sitos.md) — silver otherwise
// only ever moves fund↔settlement.
func GenesisFundSeed(population int, grainBaseValue float64, cfg SitosConfig) (seed, cap float64) {
	need := dailyGrainNeedInSilver(population, grainBaseValue)
	return need * cfg.StartingFundDays, need * cfg.FundCapMult
}

// GenesisSilverLiquid returns a new settlement's starting LIQUID silver (goods
// amount) and its silver-good cap, both pop-anchored to grain-need so the ratio
// to the fund stays pop-invariant. Deliberate genesis silver injection — same
// documented exception class as GenesisFundSeed (temenos_sitos.md).
func GenesisSilverLiquid(population int, grainBaseValue float64, cfg SitosConfig) (seed, cap float64) {
	need := dailyGrainNeedInSilver(population, grainBaseValue)
	return need * cfg.SilverStartDays, need * cfg.SilverLiquidCapDays
}

// RefPrice computes the fund's smoothed, clamped shadow price for a
// subsistence good. The moving average is derived deterministically from the
// lazy-eval tuple (amount, rate, calcTick) — no price-history table needed:
// settled() stock at tick T−i is amount + rate·(T−i−calcTick), evaluated for
// the last PriceSmoothingTicks ticks. This assumes rate is constant across
// the window, true between RecomputeProduction calls (daily).
func RefPrice(baseValue, amount, rate, calcTick float64, currentTick int, cfg SitosConfig) float64 {
	w := cfg.PriceSmoothingTicks
	if w < 1 {
		w = 1
	}
	var sum float64
	for i := 0; i < w; i++ {
		elapsed := float64(currentTick-i) - calcTick
		if elapsed < 0 {
			elapsed = 0
		}
		stock := amount + rate*elapsed
		if stock < 0 {
			stock = 0
		}
		sum += LocalPrice(baseValue, stock, rate)
	}
	avg := sum / float64(w)
	if avg < cfg.RefPriceFloor {
		return cfg.RefPriceFloor
	}
	if avg > cfg.RefPriceCeiling {
		return cfg.RefPriceCeiling
	}
	return avg
}

// SitosAction is the outcome of evaluating one settlement/good/tick for the
// fund: "buy" (fund removes surplus from the city, paying silver), "sell"
// (fund covers a shortage, city pays silver), or "noop". Quantity and
// SilverMoved are always ≥ 0; Kind disambiguates direction.
type SitosAction struct {
	Kind        string // "buy" | "sell" | "noop"
	Quantity    float64
	SilverMoved float64
}

// EvaluateSitosAction decides the fund's action this tick for one good.
// Pure function (no I/O) so the conservation math is unit-testable directly.
//
// Silver STRICTLY conserved (temenos_sitos.md §Korrigeringar #1): every
// quantity is triple-gated — the source must be able to afford it AND the
// receiving side must have cap headroom for it — so no UPDATE ever silently
// clips an amount after silver has already left the other party. Grain
// (good stock) is the free source/sink: destroyed on buy, created on sell.
//
//   - Surplus (actualPrice < refPrice, stock above the reference band):
//     fund buys down to reference, gated by fund.silver and the
//     settlement's silver cap headroom.
//   - Shortage (actualPrice > refPrice, stock below the reference band):
//     fund sells up to reference, gated by settlement.silver and the
//     fund's cap headroom.
//   - Empty fund (surplus case) or settlement can't pay (shortage case) →
//     "noop" — no transaction. This is intended, not an error.
func EvaluateSitosAction(refPrice, actualPrice, stock, reference float64, fundSilver, fundCap, settlementSilver, settlementSilverCap float64) SitosAction {
	if refPrice <= 0 {
		return SitosAction{Kind: "noop"}
	}

	switch {
	case actualPrice < refPrice && stock > reference:
		q := stock - reference
		q = math.Min(q, fundSilver/refPrice)
		q = math.Min(q, math.Max(0, settlementSilverCap-settlementSilver)/refPrice)
		if q <= 0 {
			return SitosAction{Kind: "noop"}
		}
		return SitosAction{Kind: "buy", Quantity: q, SilverMoved: q * refPrice}

	case actualPrice > refPrice && stock < reference:
		q := reference - stock
		q = math.Min(q, settlementSilver/refPrice)
		q = math.Min(q, math.Max(0, fundCap-fundSilver)/refPrice)
		if q <= 0 {
			return SitosAction{Kind: "noop"}
		}
		return SitosAction{Kind: "sell", Quantity: q, SilverMoved: q * refPrice}

	default:
		return SitosAction{Kind: "noop"}
	}
}
