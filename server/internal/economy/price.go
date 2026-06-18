package economy

import "math"

const (
	referenceRatio    = 0.3 // comfortable stock = cap × referenceRatio
	shortageMultipler = 2.0 // at stock=0: price = base × (1 + 2) = 3×base
	surplusMultiplier = 0.5 // at stock=cap: price = base × (1 - 0.5) = 0.5×base
)

// LocalPrice returns the local market price for one unit of a good.
//
// Formula (from design doc ekonomi_och_handel.md §Lokalt pris):
//
//	price = base × (1 + shortage) × (1 − surplus)
//	shortage = max(0, (reference − effective_stock) / reference)   — 0..1
//	surplus  = max(0, (effective_stock − reference) / (cap − reference))  — 0..1
//
// ratePerMin is the net production/consumption rate. Positive = net producer
// (mild downward pressure on price); negative = net consumer (shortage builds faster).
// Currently rates are always ≥ 0 in settlement_goods, but the parameter is wired up
// so it works correctly once net consumption is tracked continuously.
func LocalPrice(baseValue, stock, ratePerMin, cap float64) float64 {
	// Project stock 1 day ahead using net rate (captures whether we're filling or draining).
	const lookaheadMin = 60.0 * 24.0
	projected := stock + ratePerMin*lookaheadMin
	if projected < 0 {
		projected = 0
	}
	if projected > cap {
		projected = cap
	}

	reference := cap * referenceRatio
	if reference <= 0 {
		return baseValue
	}

	shortage := math.Max(0, (reference-projected)/reference)
	surplus := 0.0
	denominator := cap - reference
	if denominator > 0 {
		surplus = math.Max(0, (projected-reference)/denominator)
	}

	price := baseValue * (1 + shortageMultipler*shortage) * (1 - surplusMultiplier*surplus)
	if price < 0 {
		price = 0
	}
	return price
}
