package economy

// End-to-end (real DB) coverage for the livestock fallback tier
// (megaron_plan_foda_konsistens.md S1): the pure-function shape is proven in
// food_consumption_split_test.go, this file proves RecomputeProduction's
// actual DB write — the discrete slaughter debit and its same-tick
// idempotency guard, neither of which the pure function touches.

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func seedLivestock(t *testing.T, settlementID uuid.UUID, amount float64, calcTick int) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'livestock', $2, 0, 1000000, $3)`,
		settlementID, amount, calcTick,
	); err != nil {
		t.Fatalf("seed livestock row: %v", err)
	}
}

func readLivestockAmount(t *testing.T, settlementID uuid.UUID) float64 {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	var amount float64
	if err := pool.QueryRow(ctx,
		`SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'livestock'`,
		settlementID,
	).Scan(&amount); err != nil {
		t.Fatalf("read livestock amount: %v", err)
	}
	return amount
}

// TestRecomputeProduction_LivestockFallback_CoversShortfallAndDoesNotStarve:
// a settlement with a grain shortfall (mountain catchment, no grain/fish
// production beyond the flat nearjord trickle), zero fish, and a herd of 3
// must slaughter exactly one animal (ceil(200/200)) to cover what nearjord
// doesn't — grain lands at net 0 (does not starve) instead of draining
// further negative.
func TestRecomputeProduction_LivestockFallback_CoversShortfallAndDoesNotStarve(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	const tick = 100
	// pop=500 → demand = 500*0.5 = 250/tick. Mountain catchment: no grain/fish
	// rule matches, so grainProd is JUST NearjordGrainPerTick (50, P1) —
	// demand must exceed that flat trickle for a shortfall to exist at all,
	// and by enough to need exactly one animal: unmet = 250-50 = 200 = ceil(200/200).
	settlementID := recomputeFixture(t, tick, /*pop*/ 500, /*grainAmount*/ 0, /*grainRate*/ 0)
	seedLivestock(t, settlementID, 3, tick-1)

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	gotLivestock := readLivestockAmount(t, settlementID)
	if gotLivestock != 2 {
		t.Errorf("livestock amount = %v, want 2 (one whole animal slaughtered to cover the 200-unit shortfall left after nearjord)", gotLivestock)
	}

	var grainRate float64
	if err := pool.QueryRow(ctx,
		`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'`,
		settlementID,
	).Scan(&grainRate); err != nil {
		t.Fatalf("read grain rate: %v", err)
	}
	if grainRate != 0 {
		t.Errorf("grain rate = %v, want exactly 0 (livestock covered the shortfall, city does not starve)", grainRate)
	}
}

// TestRecomputeProduction_LivestockFallback_IdempotentWithinSameTick:
// RecomputeProduction is called many times a day (build/train/colonize), not
// once. A second call within the SAME tick must not slaughter a second
// animal for the same day's already-covered shortfall.
func TestRecomputeProduction_LivestockFallback_IdempotentWithinSameTick(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	const tick = 100
	settlementID := recomputeFixture(t, tick, /*pop*/ 500, /*grainAmount*/ 0, /*grainRate*/ 0)
	seedLivestock(t, settlementID, 3, tick-1)

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction (first call): %v", err)
	}
	if got := readLivestockAmount(t, settlementID); got != 2 {
		t.Fatalf("after first call: livestock = %v, want 2", got)
	}

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction (second call): %v", err)
	}
	if got := readLivestockAmount(t, settlementID); got != 2 {
		t.Errorf("after second same-tick call: livestock = %v, want still 2 (no double slaughter)", got)
	}
}
