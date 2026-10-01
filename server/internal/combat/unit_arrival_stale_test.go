package combat

// Regression: an arrival queued for a march that has since been REPLACED —
// recalled, redirected, or ended and re-ordered — must be a no-op while the
// unit is marching again. Recall/redirect enqueue a new arrival and never
// cancel the old one; before the fix the old one fired at its original tick,
// found status='marching' and resolved the arrival at the NEW target early
// (a recalled unit far from home "arrived" home the instant its outbound
// march would have landed). Perplexity-triage of the movement engine,
// VERIFIERING 2026-10-01.
//
// DB integration test (real Postgres, gated by DATABASE_URL).

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
)

func TestUnitArrivalHandler_SupersededArrivalIsNoOp(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 10) RETURNING id`,
		"unit-arrival-stale-"+uuid.NewString(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"unit-arrival-stale-owner-"+uuid.NewString(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	// The unit as ExecuteRecall leaves it at tick 8: turned for home at (4,0),
	// due there at tick 20. Its outbound arrival was queued for tick 10.
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r,
		    target_q, target_r, departs_at, arrives_at, depart_tick, arrive_tick)
		 VALUES ($1,$2,'infantry','land',10,0,'marching',4,0, 0,0, now(), now(), 8, 20)
		 RETURNING id`,
		worldID, ownerID).Scan(&unitID); err != nil {
		t.Fatalf("create recalled unit: %v", err)
	}

	stale := 10
	payload, err := json.Marshal(unit.ScheduledUnitArrivalPayload{UnitID: unitID, WorldID: worldID, ArriveTick: &stale})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	evt := events.ScheduledEvent{ID: 1, WorldID: worldID, Payload: payload}

	clk := clock.NewTestClock(time.Now())
	h := NewUnitArrivalHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clk), clk, economy.SitosConfig{})

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle on a superseded arrival: got %v, want nil", err)
	}

	var status string
	var q, r int
	if err := pool.QueryRow(ctx, `SELECT status, q, r FROM units WHERE id = $1`, unitID).Scan(&status, &q, &r); err != nil {
		t.Fatalf("read unit after handle: %v", err)
	}
	if status != "marching" || q != 4 || r != 0 {
		t.Errorf("after the superseded arrival: status=%q at (%d,%d), want still marching at (4,0) — "+
			"the old arrival must not land the unit at its new target early", status, q, r)
	}
}
