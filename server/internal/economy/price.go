package economy

import (
	"math"

	"github.com/poleia/server/internal/events"
)

const (
	// referenceBufferDays: "comfortable stock" = N days of net per-tick flow at
	// the good's current rate — replaces the old cap×0.3 anchor (2026-07-05:
	// storage caps stopped being a meaningful ceiling, see temenos_ekonomi.md
	// §Lagringstak). Population/production-driven, so a small colony and a
	// giant capital both feel "comfortable" at the same number of days.
	referenceBufferDays = 3.0
	// referenceFloorUnits: fallback anchor for a good with zero/negative
	// current rate (e.g. production paused, or grain's net rate went negative
	// from consumption). Without a floor, ProductionReference would be 0 or
	// negative and the price formula would divide by ~0 — this keeps old
	// stock from a stalled producer pricing sensibly instead of flatlining to
	// baseValue. Deliberately small: it only matters in this edge case, not
	// for any actively producing settlement. Placeholder for this dev phase —
	// see [[temenos_ekonomi]] for a proper population-anchored replacement.
	referenceFloorUnits = 10.0
	shortageMultipler   = 2.0 // at stock=0: price = base × (1 + 2) = 3×base
	surplusMultiplier   = 0.5 // at stock=2×reference: price = base × (1 - 0.5) = 0.5×base
)

// ProductionReference returns the "comfortable stock" anchor for a good given
// its current net per-tick rate.
func ProductionReference(ratePerTick float64) float64 {
	r := ratePerTick * float64(events.TicksPerDay) * referenceBufferDays
	if r < referenceFloorUnits {
		return referenceFloorUnits
	}
	return r
}

// LocalPrice returns the local market price for one unit of a good.
//
// Formula (from design doc ekonomi_och_handel.md §Lokalt pris):
//
//	price = base × (1 + shortage) × (1 − surplus)
//	shortage = max(0, (reference − effective_stock) / reference)          — 0..1
//	surplus  = clamp((effective_stock − reference) / reference, 0, 1)     — 0..1
//
// ratePerTick is the net production/consumption rate PER TICK (settlement_goods.rate
// is per-tick after mig 071 — the per-minute unit was retired). Positive = net
// producer (mild downward pressure on price); negative = net consumer (shortage
// builds faster; grain's net rate folds population consumption, so it can be < 0).
func LocalPrice(baseValue, stock, ratePerTick float64) float64 {
	// Project stock one day (TicksPerDay ticks) ahead using the net per-tick rate
	// (captures whether we're filling or draining).
	lookaheadTicks := float64(events.TicksPerDay)
	projected := stock + ratePerTick*lookaheadTicks
	if projected < 0 {
		projected = 0
	}

	reference := ProductionReference(ratePerTick)

	shortage := math.Max(0, (reference-projected)/reference)
	surplus := math.Max(0, math.Min(1, (projected-reference)/reference))

	price := baseValue * (1 + shortageMultipler*shortage) * (1 - surplusMultiplier*surplus)
	if price < 0 {
		price = 0
	}
	return price
}
