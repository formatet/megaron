package combat

// Naval sentry patrol order — a ship sent with intent=sentry reaches its
// coastal_sea target and HOLDS there (positioned + sentry stance), watching the
// approaches, until its patrol timer (ScheduledSentryReturn) fires and turns it
// home via the shared dispatchReturnHome. This drives the real dispatcher across
// the whole lifecycle — arrival (post sentry) → timer (turn home) → return
// arrival (re-garrison) — plus the idempotency guard on the timer.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
)

func TestSentryOrder_PostHoldAutoReturn(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// Single-active-world invariant (same guard as the explore/colonize tests).
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
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"sentry-"+uuid.New().String(), "sentry-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	// Coastal capital at (0,0), open sea running east to (3,0). axialDirs puts
	// (1,0) as (0,0)'s first neighbour, so NearestSeaNeighbor resolves the harbour
	// (return) hex deterministically to (1,0).
	tiles := []struct {
		q, r    int
		terrain string
	}{
		{0, 0, "plains"}, {1, 0, "coastal_sea"}, {2, 0, "coastal_sea"}, {3, 0, "coastal_sea"},
	}
	for _, tl := range tiles {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1,$2,$3,$4)`,
			worldID, tl.q, tl.r, tl.terrain); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}

	var capitalProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1,0,0,'plains') RETURNING id`,
		worldID).Scan(&capitalProvinceID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	var capitalID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1,$2,'Capital City','achaean',$3,'capital',true) RETURNING id`,
		worldID, capitalProvinceID, ownerID).Scan(&capitalID); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	// A galley already at the harbour hex (1,0) marching to (3,0) with
	// intent=sentry and home_settlement_id=capital (captured by March dispatch).
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r,
		    target_q, target_r, departs_at, arrives_at, march_intent, home_settlement_id)
		 VALUES ($1,$2,'galley','naval',1,20,'marching',1,0, 3,0, now(), now(), 'sentry', $3)
		 RETURNING id`,
		worldID, ownerID, capitalID).Scan(&unitID); err != nil {
		t.Fatalf("create sentry unit: %v", err)
	}

	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		scheduler:  events.NewScheduler(pool, clock.NewTestClock(time.Now())),
		clk:        clock.NewTestClock(time.Now()),
	}

	// ── Arrival: reaches the patrol hex and HOLDS on sentry ────────────────
	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	if err := h.resolve(ctx, tx1, unitID, worldID); err != nil {
		tx1.Rollback(ctx)
		t.Fatalf("resolve (sentry arrival): %v", err)
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit tx1: %v", err)
	}

	var status, marchIntent, stance string
	var q, r int
	var sentryQ, sentryR *int
	var homeSettlementID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT status, COALESCE(march_intent,''), COALESCE(stance,''), q, r, sentry_q, sentry_r, home_settlement_id
		 FROM units WHERE id=$1`, unitID,
	).Scan(&status, &marchIntent, &stance, &q, &r, &sentryQ, &sentryR, &homeSettlementID); err != nil {
		t.Fatalf("load unit after sentry arrival: %v", err)
	}
	if status != "positioned" {
		t.Errorf("after sentry arrival: status = %q, want positioned", status)
	}
	if stance != "sentry" {
		t.Errorf("after sentry arrival: stance = %q, want sentry", stance)
	}
	if q != 3 || r != 0 {
		t.Errorf("after sentry arrival: position = (%d,%d), want (3,0) — the patrol hex", q, r)
	}
	if sentryQ == nil || sentryR == nil || *sentryQ != 3 || *sentryR != 0 {
		t.Errorf("after sentry arrival: sentry hex = (%v,%v), want (3,0)", sentryQ, sentryR)
	}
	if marchIntent != "" {
		t.Errorf("after sentry arrival: march_intent = %q, want cleared (it is holding, not marching)", marchIntent)
	}
	if homeSettlementID == nil || *homeSettlementID != capitalID {
		t.Errorf("after sentry arrival: home_settlement_id = %v, want %v (kept for the patrol timer)", homeSettlementID, capitalID)
	}

	// A ScheduledSentryReturn timer must be armed — otherwise the ship patrols forever.
	var timerCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM scheduled_events
		 WHERE world_id=$1 AND event_type='SentryReturn' AND (payload->>'unit_id')::uuid=$2 AND processed_at IS NULL`,
		worldID, unitID).Scan(&timerCount); err != nil {
		t.Fatalf("count sentry-return timer: %v", err)
	}
	if timerCount != 1 {
		t.Errorf("armed sentry-return timers = %d, want 1", timerCount)
	}

	// ── Timer fires: HandleSentryReturn turns the ship home ────────────────
	payloadBytes, _ := json.Marshal(unit.ScheduledUnitArrivalPayload{UnitID: unitID, WorldID: worldID})
	evt := events.ScheduledEvent{WorldID: worldID, EventType: events.ScheduledSentryReturn, Payload: payloadBytes}
	if err := h.HandleSentryReturn(ctx, evt); err != nil {
		t.Fatalf("HandleSentryReturn (timer fire): %v", err)
	}

	var targetQ, targetR int
	if err := pool.QueryRow(ctx,
		`SELECT status, COALESCE(march_intent,''), COALESCE(stance,''), target_q, target_r FROM units WHERE id=$1`, unitID,
	).Scan(&status, &marchIntent, &stance, &targetQ, &targetR); err != nil {
		t.Fatalf("load unit after timer: %v", err)
	}
	if status != "marching" {
		t.Errorf("after timer: status = %q, want marching (turning home)", status)
	}
	if marchIntent != "explore_return" {
		t.Errorf("after timer: march_intent = %q, want explore_return (reused return leg)", marchIntent)
	}
	if stance != "" {
		t.Errorf("after timer: stance = %q, want cleared (no longer holding)", stance)
	}
	if targetQ != 1 || targetR != 0 {
		t.Errorf("after timer: return target = (%d,%d), want (1,0) — the home departure hex", targetQ, targetR)
	}

	// Idempotent: firing the timer again is a no-op (unit already marching home).
	if err := h.HandleSentryReturn(ctx, evt); err != nil {
		t.Fatalf("HandleSentryReturn (2nd, idempotent): %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, unitID).Scan(&status); err != nil {
		t.Fatalf("reload after 2nd timer: %v", err)
	}
	if status != "marching" {
		t.Errorf("after 2nd timer fire: status = %q, want still marching (no-op)", status)
	}

	// ── Return arrival: re-garrison at home ────────────────────────────────
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

	var settlementID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT status, settlement_id FROM units WHERE id=$1`, unitID,
	).Scan(&status, &settlementID); err != nil {
		t.Fatalf("load unit after return arrival: %v", err)
	}
	if status != "garrison" {
		t.Errorf("after return arrival: status = %q, want garrison", status)
	}
	if settlementID == nil || *settlementID != capitalID {
		t.Errorf("after return arrival: settlement_id = %v, want %v (re-garrisoned home)", settlementID, capitalID)
	}
}
