package combat

// A ship that deserts (silver_shortage) or starves (grain_shortage) while
// carrying embarked cargo must take the cargo down with it — otherwise the
// cargo unit is stuck in 'embarked' with no ship, unreachable by march/unload/
// disband (found live 2026-07-20: Koff's galleys deserted, its 114 spearmen
// stayed embarked forever). Mirrors collapse.go's cargo cascade.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestUpkeepDesertion_CascadesEmbarkedCargo(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	const tick = 2000

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-cargo-cascade-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-cargo-cascade-"+uuid.New().String(), tick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var owner uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"cargo-cascade-"+uuid.New().String(), "cargo-cascade-"+uuid.New().String()+"@test.invalid",
	).Scan(&owner); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var prov uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&prov); err != nil {
		t.Fatalf("create province: %v", err)
	}
	var sid uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Argyros', 'achaean', $3, 'capital', true, 'active', 1000) RETURNING id`,
		worldID, prov, owner,
	).Scan(&sid); err != nil {
		t.Fatalf("create settlement: %v", err)
	}
	// Plenty of grain (ship's grain upkeep is paid), zero silver (silver upkeep
	// fails every tick → desertion after upkeepDesertionPeriods).
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'grain', 10000, 0, 100000, $2), ($1, 'silver', 0, 0, 100000, $2)`,
		sid, tick,
	); err != nil {
		t.Fatalf("seed goods: %v", err)
	}

	var cargoID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status)
		 VALUES ($1, $2, 'spearman', 'land', 50, 0, 'embarked') RETURNING id`,
		worldID, owner,
	).Scan(&cargoID); err != nil {
		t.Fatalf("create cargo unit: %v", err)
	}

	// unpaid_periods = 2: this tick's failed payment pushes it to 3 =
	// upkeepDesertionPeriods, so desertion (and the cascade) fires immediately.
	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id, unpaid_periods, cargo_unit_id)
		 VALUES ($1, $2, 'galley', 'naval', 1, 0, 'garrison', $3, 2, $4) RETURNING id`,
		worldID, owner, sid, cargoID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create ship: %v", err)
	}

	store := events.NewStore(pool)
	sched := events.NewScheduler(pool, clock.NewTestClock(time.Now()))
	h := NewUpkeepHandler(pool, sched, store, nil)

	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: worldID, DueTick: tick}); err != nil {
		t.Fatalf("upkeep Handle: %v", err)
	}

	var shipStatus, cargoStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, shipID).Scan(&shipStatus); err != nil {
		t.Fatalf("read ship status: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, cargoID).Scan(&cargoStatus); err != nil {
		t.Fatalf("read cargo status: %v", err)
	}
	if shipStatus != "disbanded" {
		t.Errorf("ship status = %q, want disbanded (silver desertion should have sunk it)", shipStatus)
	}
	if cargoStatus != "disbanded" {
		t.Errorf("cargo status = %q, want disbanded — the ship's loss must cascade to its embarked cargo, not orphan it", cargoStatus)
	}
}
