package economy

import (
	"math"

	"github.com/poleia/server/internal/events"
)

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
// ratePerTick is the net production/consumption rate PER TICK (settlement_goods.rate
// is per-tick after mig 071 — the per-minute unit was retired). Positive = net
// producer (mild downward pressure on price); negative = net consumer (shortage
// builds faster; grain's net rate folds population consumption, so it can be < 0).
func LocalPrice(baseValue, stock, ratePerTick, cap float64) float64 {
	// Project stock one day (TicksPerDay ticks) ahead using the net per-tick rate
	// (captures whether we're filling or draining).
	lookaheadTicks := float64(events.TicksPerDay)
	projected := stock + ratePerTick*lookaheadTicks
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
