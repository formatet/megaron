package economy

import (
	"context"
	"testing"
)

// TestRecomputeProduction_ParkedGoodNeverProduced covers mig 114
// (Temenos_varutaxonomi_sol.md §4.2): horses is parked (goods.status =
// 'parked') even though its production_rule (stable, terrain-agnostic,
// migration 008) is fully satisfied here — a level-1 stable exists on the
// settlement. RecomputeProduction must exclude it from base_potential
// entirely and null any rate it already carried, the same "fell out of the
// production set" path recompute_stale_rate_test.go exercises for a
// catchment change — here the good itself was withdrawn instead.
func TestRecomputeProduction_ParkedGoodNeverProduced(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	const tick = 100
	settlementID := recomputeFixture(t, tick, /*pop*/ 100, /*grainAmount*/ 50, /*grainRate*/ 0)

	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'stable', 1)`,
		settlementID,
	); err != nil {
		t.Fatalf("seed stable building: %v", err)
	}

	// A stale horses rate, as if it had been producing before it was parked.
	seedStaleGood(t, settlementID, "horses", /*amount*/ 5, /*rate*/ 3, /*calcTick*/ tick-2)

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	horsesAmount, horsesRate := readGood(t, settlementID, "horses")
	if horsesRate != 0 {
		t.Errorf("parked good horses must never produce despite a satisfied production_rule (stable), got rate %.4f", horsesRate)
	}
	const wantAmount = 5.0 + 3.0*2 // settled at the OLD rate before nulling, same as any stale good
	if horsesAmount != wantAmount {
		t.Errorf("horses amount must settle at the old rate before nulling: want %.4f, got %.4f", wantAmount, horsesAmount)
	}

	// The row must still exist in the catalog — parked, not deleted.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM goods WHERE key = 'horses'`).Scan(&status); err != nil {
		t.Fatalf("read horses status: %v", err)
	}
	if status != "parked" {
		t.Errorf("horses must be status='parked' in the goods catalog, got %q", status)
	}
}

// TestLaborAllocProducible mirrors the LaborAlloc handler's own "producible"
// query gate (api/handlers/province.go) at the CatchmentBasePotential level:
// a parked good must never appear in base_potential, matching the invariant
// LaborAlloc relies on to reject an allocation to it with 422.
func TestCatchmentBasePotential_ExcludesParkedGood(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	const tick = 100
	settlementID := recomputeFixture(t, tick, /*pop*/ 100, /*grainAmount*/ 50, /*grainRate*/ 0)

	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'stable', 1)`,
		settlementID,
	); err != nil {
		t.Fatalf("seed stable building: %v", err)
	}

	pots, err := CatchmentBasePotential(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("CatchmentBasePotential: %v", err)
	}
	if bp, ok := pots["horses"]; ok {
		t.Errorf("CatchmentBasePotential must exclude parked good horses, got base_potential %.4f", bp)
	}
}
