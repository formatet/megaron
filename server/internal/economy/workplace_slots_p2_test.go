package economy

// P2 (megaron_plan_fysisk_gubbemodell.md): building level → workplace SLOTS,
// an absolute headcount, not a share of the city. This is the integration-level
// rött-före the plan calls for: RecomputeProduction end-to-end, not just the
// LaborCapacity unit tests in labor_capacity_test.go. Before P2, a building's
// output scaled with population (P1's own postmortem: "P1 gav 3x men inte
// bromsen"); after P2, two identically-built settlements that differ ONLY in
// population must produce the SAME rate for a building-gated good, because
// both are capped at the same fixed slot count.

import (
	"context"
	"testing"
)

// TestRecomputeProduction_BuildingSlotCapIsPopulationInvariant is the actual
// bug P2 fixes, exercised through the real pipeline (LoadWorkplaceSlots →
// LaborCapacity → RecomputeProduction), not a hand-computed formula. Stone via
// Stonequarry is used because its production_rule is terrain- and
// deposit-independent (NULL terrain_type, NULL requires_deposit) — the fixture
// only needs a building, not a specific deposit hex.
func TestRecomputeProduction_BuildingSlotCapIsPopulationInvariant(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 100

	rateAt := func(pop int) float64 {
		settlementID := seedFullRingFixture(t, tick, pop, "plains")
		if _, err := pool.Exec(ctx,
			`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'stonequarry', 1)`,
			settlementID,
		); err != nil {
			t.Fatalf("seed stonequarry: %v", err)
		}
		// Full staffing under P4 = one gubbe per building slot, not a weight —
		// stonequarry level 1's cap is WorkplaceSlots("stonequarry",1) = 2.
		placeBuildingGubbe(t, pool, settlementID, 1, "stonequarry", "stone")
		placeBuildingGubbe(t, pool, settlementID, 2, "stonequarry", "stone")
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		defer tx.Rollback(ctx)
		if err := RecomputeProduction(ctx, tx, settlementID); err != nil {
			t.Fatalf("RecomputeProduction: %v", err)
		}
		var rate float64
		if err := tx.QueryRow(ctx,
			`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'stone'`,
			settlementID,
		).Scan(&rate); err != nil {
			t.Fatalf("read stone rate: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
		return rate
	}

	small := rateAt(100)
	large := rateAt(5000)

	if small <= 0 {
		t.Fatalf("a level-1 stonequarry staffed at full weight must produce SOME stone, got %v", small)
	}
	// 50x the population, weight and building unchanged. Pre-P2 this rate would
	// scale ~50x too (BuildingLaborPerLevel was a flat share of laborPool);
	// post-P2 both settlements are capped at the SAME 2 slots (stonequarry
	// level 1), so the rate must be identical regardless of city size.
	if diff := large - small; diff > 1e-6*small || diff < -1e-6*small {
		t.Errorf("stone rate must be population-invariant once capped by workplace slots: "+
			"pop=100 gave %v, pop=5000 gave %v (a %.1fx difference — the slot cap did not hold)",
			small, large, large/small)
	}
}
