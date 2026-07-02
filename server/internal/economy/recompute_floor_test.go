package economy

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// recomputeFixture builds an active world + one settlement whose catchment is
// entirely mountain_limestone (no grain rule, no deposits) so the only
// producible good is the unconditional timber trickle — this forces
// RecomputeProduction down the "no grain-producing catchment" fallback branch
// (recompute.go:242), which writes a pure-consumption negative rate for grain.
func recomputeFixture(t *testing.T, currentTick, pop int, grainAmount, grainRate float64) (settlementID uuid.UUID) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-recompute-%'`,
	); err != nil {
		t.Fatalf("archive leftover test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-recompute-"+uuid.New().String(), currentTick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"recompute-"+uuid.New().String(), "recompute-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'mountain_limestone') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}

	// Six adjacent catchment tiles, all mountain_limestone with no deposits:
	// only the unconditional timber trickle rule matches, grain never does.
	for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'mountain_limestone')`,
			worldID, d[0], d[1],
		); err != nil {
			t.Fatalf("seed catchment tile: %v", err)
		}
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Recomputeville', 'achaean', $3, 'capital', true, $4) RETURNING id`,
		worldID, provinceID, ownerID, pop,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'grain', $2, $3, 1000, $4)`,
		settlementID, grainAmount, grainRate, currentTick,
	); err != nil {
		t.Fatalf("seed grain row: %v", err)
	}

	return settlementID
}

func readGrainAmount(t *testing.T, settlementID uuid.UUID) float64 {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	var amount float64
	if err := pool.QueryRow(ctx,
		`SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'`,
		settlementID,
	).Scan(&amount); err != nil {
		t.Fatalf("read grain amount: %v", err)
	}
	return amount
}

// TestRecomputeProduction_GrainFloorsAtZero: a settlement whose grain row was
// settled deep into negative territory (large deficit rate, calc_tick far in
// the past) must never have that negative value permanented as its new base.
// Reproduces the Aias/Sardis catch-22: settled() itself has no floor, so
// RecomputeProduction's settle-and-overwrite must apply one.
func TestRecomputeProduction_GrainFloorsAtZero(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	const tick = 100
	// Old row settled at rate -1000/tick over 10 ticks → settled() = 5 - 10000, deeply negative.
	settlementID := recomputeFixture(t, tick, /*pop*/ 1000, /*grainAmount*/ 5, /*grainRate*/ -1000)
	if _, err := pool.Exec(ctx,
		`UPDATE settlement_goods SET calc_tick = $1 WHERE settlement_id = $2 AND good_key = 'grain'`,
		tick-10, settlementID,
	); err != nil {
		t.Fatalf("backdate calc_tick: %v", err)
	}

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	got := readGrainAmount(t, settlementID)
	if got < 0 {
		t.Errorf("grain amount must floor at 0, got %.4f (negative base was permanented)", got)
	}
}

// TestRecomputeProduction_HealthyStockUnaffectedByFloor: a settlement with a
// positive settled amount under cap must keep the correctly settled value —
// the GREATEST(0, …) floor must not clip legitimate positive stock.
func TestRecomputeProduction_HealthyStockUnaffectedByFloor(t *testing.T) {
	const tick = 100
	// Positive rate, small elapsed ticks → settled() = 50 + 2*5 = 60, well under cap 1000.
	settlementID := recomputeFixture(t, tick, /*pop*/ 100, /*grainAmount*/ 50, /*grainRate*/ 2)
	pool := testPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`UPDATE settlement_goods SET calc_tick = $1 WHERE settlement_id = $2 AND good_key = 'grain'`,
		tick-5, settlementID,
	); err != nil {
		t.Fatalf("backdate calc_tick: %v", err)
	}

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	got := readGrainAmount(t, settlementID)
	const want = 60.0
	if got != want {
		t.Errorf("healthy settled stock: want %.4f, got %.4f (floor should not clip positive values)", want, got)
	}
}
