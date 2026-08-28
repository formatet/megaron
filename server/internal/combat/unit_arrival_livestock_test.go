package combat

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// TestFoundColony_SeedsStartingHerd verifies S1d
// (megaron_plan_foda_konsistens.md §S1d, Timothy 2026-08-07: "om det är något
// nomader har så är det boskap") on the colony founding path: a new colony
// starts with a livestock stock instead of zero, matching the genesis
// metropolis path. The figure is economy.FoundingHerdLivestock (a
// calibration ratt, not a lock).
func TestFoundColony_SeedsStartingHerd(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"colonizer-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var capitalProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&capitalProvinceID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Capital City', 'achaean', $3, 'capital', true)`,
		worldID, capitalProvinceID, ownerID,
	); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	var colonyProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 10, 10, 'plains') RETURNING id`,
		worldID,
	).Scan(&colonyProvinceID); err != nil {
		t.Fatalf("create colony province: %v", err)
	}

	const unitSize = 50
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', $3, 'marching', 10, 10) RETURNING id`,
		worldID, ownerID, unitSize,
	).Scan(&unitID); err != nil {
		t.Fatalf("create colonizing unit: %v", err)
	}

	colonyName := "Herdhaven"
	u := unitRow{
		id:         unitID,
		ownerID:    ownerID,
		utype:      "spearman",
		category:   "land",
		size:       unitSize,
		status:     "marching",
		q:          10,
		r:          10,
		colonyName: &colonyName,
	}

	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		hub:        nil,
		scheduler:  nil,
		clk:        clock.NewTestClock(time.Now()),
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	if err := h.foundColony(ctx, tx, u, colonyProvinceID, 10, 10, worldID); err != nil {
		t.Fatalf("foundColony: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM settlements WHERE world_id = $1 AND province_id = $2`,
		worldID, colonyProvinceID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("load colony settlement: %v", err)
	}

	var amount float64
	if err := pool.QueryRow(ctx,
		`SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='livestock'`,
		settlementID,
	).Scan(&amount); err != nil {
		t.Fatalf("load livestock amount: %v", err)
	}
	if amount != float64(economy.FoundingHerdLivestock) {
		t.Errorf("expected livestock=%d at colony founding, got %v", economy.FoundingHerdLivestock, amount)
	}
}
