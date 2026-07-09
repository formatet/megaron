package combat

// Explore order (auto-return) — Fas 1 (megaron_todo.md). A naval unit sent
// with intent=explore reaches its target, then automatically dispatches a
// return leg to the settlement it departed from (captured at dispatch as
// home_settlement_id, since a normal march dispatch nulls settlement_id).
// This test drives the real resolve() dispatcher across both arrivals —
// target arrival (turns for home) and the return arrival (re-garrisons).

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
)

func TestExploreOrder_AutoReturn(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// Same leftover-detritus guard as TestFoundColony_UnitDisbandsIntoPopulace:
	// current_world_tick() (used by exploreArrived to schedule the return leg)
	// reads the single active world, enforced by a partial unique index.
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
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
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"explorer-"+uuid.New().String(), "explorer-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	// Map: a coastal capital at (0,0), open sea running east to (3,0).
	// axialDirs puts (1,0) as (0,0)'s first neighbour, so NearestSeaNeighbor
	// resolves the harbour hex deterministically to (1,0).
	tiles := []struct {
		q, r    int
		terrain string
	}{
		{0, 0, "plains"},
		{1, 0, "coastal_sea"},
		{2, 0, "coastal_sea"},
		{3, 0, "coastal_sea"},
	}
	for _, tl := range tiles {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, tl.q, tl.r, tl.terrain,
		); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}

	var capitalProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&capitalProvinceID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	var capitalID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Capital City', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, capitalProvinceID, ownerID,
	).Scan(&capitalID); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	// The exploring unit: a galley already at the harbour hex (1,0) — the
	// position a real dispatch would have left it at — marching to (3,0) with
	// intent=explore and home_settlement_id=capital (captured by unit.go
	// March before settlement_id was nulled).
	var unitID uuid.UUID
	arrivesAt := time.Now()
	if err := pool.QueryRow(ctx,
		`INSERT INTO units
		   (world_id, owner_id, type, category, size, crew, status, q, r,
		    target_q, target_r, departs_at, arrives_at, march_intent, home_settlement_id)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'marching', 1, 0,
		         3, 0, now(), $3, 'explore', $4)
		 RETURNING id`,
		worldID, ownerID, arrivesAt, capitalID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create exploring unit: %v", err)
	}

	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		hub:        nil,
		scheduler:  events.NewScheduler(pool, clock.NewTestClock(time.Now())),
		clk:        clock.NewTestClock(time.Now()),
	}

	// ── Arrival 1: reaches the explore target, turns for home ──────────────
	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	if err := h.resolve(ctx, tx1, unitID, worldID); err != nil {
		tx1.Rollback(ctx)
		t.Fatalf("resolve (target arrival): %v", err)
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit tx1: %v", err)
	}

	var status, marchIntent string
	var q, r, targetQ, targetR int
	var settlementID *uuid.UUID
	var homeSettlementID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT status, COALESCE(march_intent, ''), q, r, target_q, target_r, settlement_id, home_settlement_id
		 FROM units WHERE id = $1`, unitID,
	).Scan(&status, &marchIntent, &q, &r, &targetQ, &targetR, &settlementID, &homeSettlementID); err != nil {
		t.Fatalf("load unit after target arrival: %v", err)
	}
	if status != "marching" {
		t.Errorf("after target arrival: status = %q, want %q", status, "marching")
	}
	if marchIntent != "explore_return" {
		t.Errorf("after target arrival: march_intent = %q, want %q", marchIntent, "explore_return")
	}
	if q != 3 || r != 0 {
		t.Errorf("after target arrival: position = (%d,%d), want (3,0) — the explore target", q, r)
	}
	if targetQ != 1 || targetR != 0 {
		t.Errorf("after target arrival: return target = (%d,%d), want (1,0) — the home departure hex", targetQ, targetR)
	}
	if settlementID != nil {
		t.Errorf("after target arrival: settlement_id = %v, want nil (still in transit)", *settlementID)
	}
	if homeSettlementID == nil || *homeSettlementID != capitalID {
		t.Errorf("after target arrival: home_settlement_id = %v, want %v (kept for the return leg)", homeSettlementID, capitalID)
	}

	// A scheduled_events row must exist for the return arrival — otherwise the
	// unit would be stranded mid-transit forever.
	var scheduledCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM scheduled_events
		 WHERE world_id = $1 AND event_type = 'UnitArrival'
		   AND (payload->>'unit_id')::uuid = $2 AND processed_at IS NULL`,
		worldID, unitID,
	).Scan(&scheduledCount); err != nil {
		t.Fatalf("count scheduled return arrival: %v", err)
	}
	if scheduledCount != 1 {
		t.Errorf("scheduled return-arrival events = %d, want 1", scheduledCount)
	}

	// ── Arrival 2: the return leg completes — re-garrison at home ──────────
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx2: %v", err)
	}
	if err := h.resolve(ctx, tx2, unitID, worldID); err != nil {
		tx2.Rollback(ctx)
		t.Fatalf("resolve (return arrival): %v", err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("commit tx2: %v", err)
	}

	if err := pool.QueryRow(ctx,
		`SELECT status, COALESCE(march_intent, ''), settlement_id, home_settlement_id FROM units WHERE id = $1`, unitID,
	).Scan(&status, &marchIntent, &settlementID, &homeSettlementID); err != nil {
		t.Fatalf("load unit after return arrival: %v", err)
	}
	if status != "garrison" {
		t.Errorf("after return arrival: status = %q, want %q", status, "garrison")
	}
	if marchIntent != "" {
		t.Errorf("after return arrival: march_intent = %q, want empty (cleared)", marchIntent)
	}
	if settlementID == nil || *settlementID != capitalID {
		t.Errorf("after return arrival: settlement_id = %v, want %v (re-garrisoned at home)", settlementID, capitalID)
	}
	if homeSettlementID != nil {
		t.Errorf("after return arrival: home_settlement_id = %v, want nil (cleared)", *homeSettlementID)
	}
}
