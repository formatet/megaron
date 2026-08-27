package combat

// Rött-före fall 2 (megaron_plan_utfodringsordningen.md §6.2): a settlement
// whose grain stock covers the garrison's upkeep but not the garrison PLUS
// the population's daily food. Kanon (Timothy 2026-08-25): "ALLT SOM STADEN
// FÖRSÖRJER ÄTER FÖRE BEFOLKNINGEN" — the garrison must be fully fed and the
// population takes the shortfall, never the other way around.
//
// This is an ordering test, not a single-handler test: UpkeepTick (Plikt,
// priority 50) and FoodTick (Föda, priority 55) both draw from the same
// settlement_goods grain stock, and the invariant is that Plikt's debit runs
// to completion BEFORE Föda ever reads what's left. Lives in package combat
// (not economy) because UpkeepHandler is defined here and G1 forbids economy
// from importing combat; driving both handlers in the real tick order is only
// possible from a package that can see both.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestFoodOrdering_GarrisonFedInFullBeforePopulationTakesShortfall(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "food-ordering") // capitalID: population 1000, demand 5/tick (500 → 5, mig 136, GrainConsumptionPerCitizenPerTick ÷100)

	// Exactly enough grain for the garrison's own upkeep (0.5, see
	// upkeep_idempotent_test.go's UnitUpkeep(spearman, land, 100, garrison) = 0.5
	// grain — 50 → 0.5, mig 136, UpkeepSpecs grain ÷100) plus a partial serving
	// for the population — NOT enough for both in full. 2.0 = 0.5 (army) + 1.5
	// (leaves the population 3.5 short of its 5). 200/50/150/350/500 all → ÷100
	// (mig 136 — both the army's and the population's grain figures share the
	// same ÷100 calibration, so the whole scenario scales uniformly).
	const seededGrain = 2.0
	seedGoods(t, pool, f.capitalID, f.tick, seededGrain, 100000)

	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
		                    q, r, settlement_id, support_settlement_id, unpaid_periods)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'garrison', 0, 0, $3, $3, 0)
		 RETURNING id`,
		f.worldID, f.owner, f.capitalID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create garrison unit: %v", err)
	}

	upkeepH := NewUpkeepHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())),
		events.NewStore(pool), &fakeBroadcaster{})
	upkeepH.soldShare = 0

	// Plikt (50) first — the day's order this test exists to pin.
	if err := upkeepH.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.tick}); err != nil {
		t.Fatalf("UpkeepTick Handle: %v", err)
	}

	var sizeAfterUpkeep int
	if err := pool.QueryRow(ctx, `SELECT size FROM units WHERE id = $1`, unitID).Scan(&sizeAfterUpkeep); err != nil {
		t.Fatalf("read unit size after upkeep: %v", err)
	}
	if sizeAfterUpkeep != 100 {
		t.Fatalf("garrison size after UpkeepTick = %d, want 100 — fixture does not exercise the handler (grain stock should cover the garrison alone)", sizeAfterUpkeep)
	}
	grainAfterUpkeep := settledAmount(t, pool, f.capitalID, "grain")
	if grainAfterUpkeep != seededGrain-0.5 {
		t.Fatalf("grain after UpkeepTick = %.1f, want %.1f", grainAfterUpkeep, seededGrain-0.5)
	}

	// Föda (55) second — reads whatever Plikt left behind.
	foodH := economy.NewFoodTickHandler(pool,
		events.NewScheduler(pool, clock.NewTestClock(time.Now())),
		events.NewStore(pool), nil)
	if err := foodH.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.tick}); err != nil {
		t.Fatalf("FoodTick Handle: %v", err)
	}

	var sizeAfterFood int
	if err := pool.QueryRow(ctx, `SELECT size FROM units WHERE id = $1`, unitID).Scan(&sizeAfterFood); err != nil {
		t.Fatalf("read unit size after food tick: %v", err)
	}
	if sizeAfterFood != 100 {
		t.Errorf("garrison size after FoodTick = %d, want unchanged 100 — the population's food shortfall must never reach back and cost the army men", sizeAfterFood)
	}

	var unmet float64
	if err := pool.QueryRow(ctx, `SELECT food_unmet_amount FROM settlements WHERE id = $1`, f.capitalID).Scan(&unmet); err != nil {
		t.Fatalf("read food_unmet_amount: %v", err)
	}
	if want := 3.5; unmet != want { // 350.0 → 3.5 (mig 136, ÷100)
		t.Errorf("food_unmet_amount = %.1f, want %.1f — the population, not the garrison, must carry the shortfall", unmet, want)
	}

	grainAfterFood := settledAmount(t, pool, f.capitalID, "grain")
	if grainAfterFood != 0 {
		t.Errorf("grain after FoodTick = %.1f, want 0 (fully exhausted feeding the population what grain remained)", grainAfterFood)
	}
}
