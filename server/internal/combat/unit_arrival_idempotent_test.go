package combat

// G2 idempotency regression for UnitArrivalHandler.Handle (ScheduledUnitArrival).
//
// unit_arrival.go's own doc comment: "Idempotency: the arriving unit is
// fetched with FOR UPDATE and the handler exits early if status != 'marching'.
// ON CONFLICT DO NOTHING is used for projection inserts. Re-running the
// handler is therefore safe." This is a DB integration test (real Postgres,
// gated by DATABASE_URL) proving that guard on the plain peaceful-garrison
// path (arriveGarrison): a marching unit carrying silver arrives at its own
// settlement, the purse is credited AND the unit flips to status='garrison'
// in the same transaction — so the status guard on replay is what stops a
// second credit, not a separate claim table. Drives the SAME
// ScheduledUnitArrival event through Handle twice and asserts the
// settlement's silver is credited exactly once.

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

func TestUnitArrivalHandler_ReplayIsIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 500) RETURNING id`,
		"unit-arrival-idem-"+uuid.NewString(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"unit-arrival-owner-"+uuid.NewString(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	// Two plains tiles the unit walks between — origin (0,0) has no
	// settlement, destination (2,0) is the owner's own capital.
	for _, tl := range []struct{ q, r int }{{0, 0}, {1, 0}, {2, 0}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1,$2,$3,'plains')`,
			worldID, tl.q, tl.r); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}

	var capitalProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1,2,0,'plains') RETURNING id`,
		worldID).Scan(&capitalProvinceID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	var capitalID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state)
		 VALUES ($1,$2,'Capital City','achaean',$3,'capital',true,'active') RETURNING id`,
		worldID, capitalProvinceID, ownerID).Scan(&capitalID); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'silver', 20, 0, 1000000, 0)`,
		capitalID,
	); err != nil {
		t.Fatalf("seed capital silver: %v", err)
	}

	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r,
		    target_q, target_r, departs_at, arrives_at, carried_silver)
		 VALUES ($1,$2,'spearman','land',100,0,'marching',0,0, 2,0, now(), now(), 50)
		 RETURNING id`,
		worldID, ownerID).Scan(&unitID); err != nil {
		t.Fatalf("create marching unit: %v", err)
	}

	payload, err := json.Marshal(unit.ScheduledUnitArrivalPayload{UnitID: unitID, WorldID: worldID})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	evt := events.ScheduledEvent{ID: 1, WorldID: worldID, Payload: payload}

	clk := clock.NewTestClock(time.Now())
	h := NewUnitArrivalHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clk), clk, economy.SitosConfig{})

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (first run): %v", err)
	}

	var statusAfterFirst string
	var silverAfterFirst float64
	var carriedAfterFirst float64
	if err := pool.QueryRow(ctx, `SELECT status, carried_silver FROM units WHERE id = $1`, unitID).Scan(&statusAfterFirst, &carriedAfterFirst); err != nil {
		t.Fatalf("read unit after first run: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`,
		capitalID,
	).Scan(&silverAfterFirst); err != nil {
		t.Fatalf("read capital silver after first run: %v", err)
	}
	if statusAfterFirst != "garrison" {
		t.Fatalf("unit status after first run = %q, want garrison — fixture does not exercise the handler", statusAfterFirst)
	}
	if silverAfterFirst != 70 {
		t.Fatalf("capital silver after first run = %v, want 70 (20 seed + 50 carried) — fixture does not exercise the handler", silverAfterFirst)
	}
	if carriedAfterFirst != 0 {
		t.Fatalf("unit carried_silver after first run = %v, want 0 (cleared on arrival)", carriedAfterFirst)
	}

	// Replay the SAME event.
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (replay): %v", err)
	}

	var silverAfterReplay float64
	if err := pool.QueryRow(ctx,
		`SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`,
		capitalID,
	).Scan(&silverAfterReplay); err != nil {
		t.Fatalf("read capital silver after replay: %v", err)
	}
	if silverAfterReplay != 70 {
		t.Errorf("capital silver after replay = %v, want still 70 (a non-idempotent handler would double-credit the purse to 120)", silverAfterReplay)
	}
}
