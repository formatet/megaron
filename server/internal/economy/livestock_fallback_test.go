package economy

// End-to-end (real DB) coverage for the livestock fallback tier
// (megaron_plan_foda_konsistens.md S1): the pure-function shape is proven in
// food_consumption_split_test.go, this file proves the actual DB write — the
// discrete slaughter debit and its idempotency guard.
//
// Utfodringsordningen D1/D3 (megaron_plan_utfodringsordningen.md, 2026-08-26)
// moved this whole fallback chain (grain → fish → livestock) OUT of
// RecomputeProduction and into FoodTickHandler (food_tick.go), operating on
// STOCK instead of production rate — these tests moved with it. The
// same-tick idempotency guard RecomputeProduction used to need (it runs many
// times a day) is no longer the relevant one; FoodTick runs once a day and
// idempotency is now about REPLAYING THE SAME SCHEDULED EVENT (G2,
// food_tick_test.go's TestFoodTick_ReplayIsIdempotent covers the general
// case — this file keeps the livestock-specific regression named).

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/events"
)

// TestFoodTick_LivestockFallback_CoversShortfallAndDoesNotStarve: a
// settlement with a grain shortfall (0.5 in stock — the flat nearjord trickle
// a mountain catchment would raw-produce, per D1), zero fish, and a herd of 3
// must slaughter exactly one animal (ceil(2.0/166.67)) to cover what grain
// doesn't — food_unmet_amount lands at 0 (does not starve) instead of
// carrying the residual.
func TestFoodTick_LivestockFallback_CoversShortfallAndDoesNotStarve(t *testing.T) {
	pool := testPool(t)

	// pop=500 -> demand = 500*0.005 = 2.5/tick (mig 136, GrainConsumptionPerCitizenPerTick
	// 0.5→0.005). grainStock=0.5 (mig 136, NearjordGrainPerTick 50→0.5 — its own
	// ÷100 calibration, not grain's ÷43.2) leaves a 2.0-unit shortfall after grain
	// alone — well inside one animal's 166.67 grain-equivalent (mig 136,
	// livestockFoodValue 200→166.67), so ceil(2.0/166.67)=1 closes it.
	const population = 500
	f := newFoodFixture(t, pool, "livestock-covers", population)
	seedFoodStock(t, pool, f, /*grain*/ 0.5, /*fish*/ 0, /*livestock*/ 3)

	h := newFoodTickHandler(pool)
	if err := h.Handle(context.Background(), events.ScheduledEvent{
		ID: 900101, WorldID: f.worldID, DueTick: f.tick,
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if got := foodStockOf(t, pool, f.settlementID, GoodLivestock); got != 2 {
		t.Errorf("livestock amount = %v, want 2 (one whole animal slaughtered to cover the 2.0-unit shortfall left after grain)", got)
	}
	if got := foodStockOf(t, pool, f.settlementID, GoodGrain); got != 0 {
		t.Errorf("grain stock = %v, want exactly 0 (grain alone covered only part of demand)", got)
	}
	if got := foodUnmetOf(t, pool, f.settlementID); got != 0 {
		t.Errorf("food_unmet_amount = %v, want 0 (livestock covered the shortfall, city does not starve)", got)
	}
}

// TestFoodTick_LivestockFallback_IdempotentOnReplay: a worker replay of the
// SAME ScheduledFoodTick event (crash/timeout between commit and markDone,
// CLAUDE.md "Events") must not slaughter a second animal for the same day's
// already-covered shortfall.
func TestFoodTick_LivestockFallback_IdempotentOnReplay(t *testing.T) {
	pool := testPool(t)

	const population = 500
	f := newFoodFixture(t, pool, "livestock-idempotent", population)
	seedFoodStock(t, pool, f, /*grain*/ 0.5, /*fish*/ 0, /*livestock*/ 3) // 50 → 0.5 (mig 136, NearjordGrainPerTick)

	h := newFoodTickHandler(pool)
	evt := events.ScheduledEvent{ID: 900102, WorldID: f.worldID, DueTick: f.tick}

	if err := h.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle (first run): %v", err)
	}
	if got := foodStockOf(t, pool, f.settlementID, GoodLivestock); got != 2 {
		t.Fatalf("after first run: livestock = %v, want 2", got)
	}

	if err := h.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle (replay): %v", err)
	}
	if got := foodStockOf(t, pool, f.settlementID, GoodLivestock); got != 2 {
		t.Errorf("after replay: livestock = %v, want still 2 (no double slaughter)", got)
	}
}
