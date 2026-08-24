package economy

// megaron_plan_sten_stock.md §6.1 criterion 3: stone must not reach its
// storage cap over a 40-tick run, even fully staffed. Uses the same real-DB
// rig every other economy test in this package uses (testPool + RecomputeProduction
// against Postgres) — "den befintliga ekonomiriggen" the plan refers to is
// this pattern, not a separate script.
//
// Rött-före is not meaningful here in the usual "flip a constant" sense:
// goodCap() is a flat 1,000,000 for every good (recompute.go), so even the
// UNMIGRATED rate (576/tick × 40 = 23,040) stays under cap by more than an
// order of magnitude — the cap was never the actual problem migration 129
// fixes (see the plan's §1: the sink is the 750-stone build catalogue, not
// the storage cap). This test exists as the regression guard the plan asks
// for, not as a criterion that migration 129 makes pass where it previously
// failed.

import (
	"context"
	"testing"
)

func TestFullyStaffedStonequarry_DoesNotReachCapIn40Ticks(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	settlementID := seedFullRingFixture(t, 0, 100, "plains")
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'stonequarry', 1)`,
		settlementID,
	); err != nil {
		t.Fatalf("seed stonequarry: %v", err)
	}
	placeBuildingGubbe(t, pool, settlementID, 1, "stonequarry", "stone")
	placeBuildingGubbe(t, pool, settlementID, 2, "stonequarry", "stone")

	var worldID string
	if err := pool.QueryRow(ctx,
		`SELECT world_id FROM settlements WHERE id = $1`, settlementID,
	).Scan(&worldID); err != nil {
		t.Fatalf("read world id: %v", err)
	}

	const runTicks = 40
	for tick := 1; tick <= runTicks; tick++ {
		if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick = $2 WHERE id = $1`, worldID, tick); err != nil {
			t.Fatalf("advance world to tick %d: %v", tick, err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx at tick %d: %v", tick, err)
		}
		if err := RecomputeProduction(ctx, tx, settlementID); err != nil {
			tx.Rollback(ctx)
			t.Fatalf("RecomputeProduction at tick %d: %v", tick, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit tick %d: %v", tick, err)
		}
	}

	var amount, cap float64
	if err := pool.QueryRow(ctx,
		`SELECT settled(amount, rate, calc_tick), cap FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'stone'`,
		settlementID,
	).Scan(&amount, &cap); err != nil {
		t.Fatalf("read stone stock: %v", err)
	}

	if amount >= cap {
		t.Errorf("stone reached its storage cap (%v) after %d ticks (amount=%v) — "+
			"a fully staffed stonequarry must not peg the cap in this window", cap, runTicks, amount)
	}
}
