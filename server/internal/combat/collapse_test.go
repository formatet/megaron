package combat

// Regression test for the r6 legibility audit (2026-07-24, megaron_todo.md):
// collapseSettlement disbanded the garrison and dispossessed the settlement
// entirely silently — its only player-reachable signals were a gossip.Broadcast
// to NEARBY settlement owners (never guaranteed to reach the affected Wanax,
// and reaching no one at all when this was their last city) and an audit-only
// CityCollapsed event (chronicle/province-stream, never surfaced to a client).
// This test asserts the owner now gets a direct CityCollapsed notification via
// the hub, mirroring notifyUnitLoss (upkeep.go) / FieldBattleWon-Lost
// (unit_arrival_field.go).

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestCollapseSettlement_NotifiesOwner(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// The fixture world must be ACTIVE (it used to be 'archived'). collapseSettlement
	// calls economy.RecomputeProduction, which since fix/recompute-stale-rate no longer
	// returns early for a settlement with zero producible goods — it falls through to
	// the grain-consumption upsert, and that writes calc_tick = current_world_tick().
	// That function reads the single globally-active world, so with no active world it
	// returns NULL, the INSERT trips settlement_goods.calc_tick NOT NULL, and the whole
	// collapse transaction aborts (25P02) — masked, because collapse.go discards
	// RecomputeProduction's error. Same reasoning and same leftover-sweep + archive
	// cleanup as unit_arrival_colonize_test.go; see its comment for why we archive
	// rather than delete.
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 100) RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})
	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"collapser-"+uuid.New().String(), "collapser-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 20, 20, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Doomed City', 'achaean', $3, 'capital', true, 90) RETURNING id`,
		worldID, provinceID, ownerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}
	// A garrison unit to verify it's disbanded by the collapse.
	var garrisonID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, settlement_id, type, category, size, status, q, r)
		 VALUES ($1, $2, $3, 'spearman', 'land', 40, 'garrison', 20, 20) RETURNING id`,
		worldID, ownerID, settlementID,
	).Scan(&garrisonID); err != nil {
		t.Fatalf("create garrison unit: %v", err)
	}

	fb := &fakeBroadcaster{}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	if err := collapseSettlement(ctx, tx, events.NewStore(pool), nil, fb,
		settlementID, worldID, "starvation"); err != nil {
		t.Fatalf("collapseSettlement: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	found := false
	for _, kind := range fb.notified {
		if kind == "CityCollapsed" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a CityCollapsed notification to the owner, got kinds %v", fb.notified)
	}

	var garrisonStatus string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM units WHERE id = $1`, garrisonID,
	).Scan(&garrisonStatus); err != nil {
		t.Fatalf("load garrison after collapse: %v", err)
	}
	if garrisonStatus != "disbanded" {
		t.Errorf("expected garrison status=disbanded, got %q", garrisonStatus)
	}
}
