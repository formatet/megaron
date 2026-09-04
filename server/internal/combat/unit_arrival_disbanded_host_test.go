package combat

// Regression: a queued UnitArrival that fires AFTER its unit stopped existing
// as a marching unit must be a silent no-op, not a dead letter.
//
// Live 2026-09-03, world 2e252f65, Polyidos' nomadic host a9fb29c8:
//   tick 1  march ordered (2,21) → (1,22), arrival scheduled for tick 3
//   tick 2  the player founds the metropolis instead — found_metropolis.go:283
//           sets the host to status='disbanded', q=NULL, r=NULL, and nothing
//           cancels the queued arrival
//   tick 3  the arrival fires; resolve() scanned q/r straight into plain ints,
//           the NULL blew up the load, the worker retried three times and
//           dead-lettered it, and NotifyDeadLetter told the player their march
//           had suffered a "system fault"
// The founding had succeeded. Only the notification said otherwise — which is
// what made it dangerous: silent-but-wrong for a player who got their city
// anyway, and it would have hidden a real failure the day founding did NOT go
// through.
//
// The "already resolved" guard (status != 'marching' → return nil) existed the
// whole time; it just sat three lines below the scan that killed the handler.
// This test pins the ORDER of those two steps, not the guard itself —
// TestUnitArrivalHandler_ReplayIsIdempotent already covers the guard on the
// ordinary garrison path, where q/r survive arrival and the NULL never appears.
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

func TestUnitArrivalHandler_DisbandedHostWithNullPositionIsNoOp(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 3) RETURNING id`,
		"unit-arrival-disbanded-"+uuid.NewString(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"unit-arrival-disbanded-owner-"+uuid.NewString(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	// The host exactly as found_metropolis.go leaves it: dissolved into the
	// metropolis it founded, no position, but its march target still on the row.
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r,
		    target_q, target_r, departs_at, arrives_at)
		 VALUES ($1,$2,'nomadic_host','land',1,0,'disbanded',NULL,NULL, 1,22, now(), now())
		 RETURNING id`,
		worldID, ownerID).Scan(&unitID); err != nil {
		t.Fatalf("create disbanded host: %v", err)
	}

	payload, err := json.Marshal(unit.ScheduledUnitArrivalPayload{UnitID: unitID, WorldID: worldID})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	evt := events.ScheduledEvent{ID: 1, WorldID: worldID, Payload: payload}

	clk := clock.NewTestClock(time.Now())
	h := NewUnitArrivalHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clk), clk, economy.SitosConfig{})

	// Before the fix this returned "load arriving unit: … cannot scan NULL …",
	// which the worker counts as a failure — three of them and the player is
	// told their march hit a system fault.
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle on a disbanded host with NULL position: got %v, want nil "+
			"(a superseded arrival is a no-op, not a dead letter)", err)
	}

	// And it must stay a no-op: the row is untouched.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, unitID).Scan(&status); err != nil {
		t.Fatalf("read unit after handle: %v", err)
	}
	if status != "disbanded" {
		t.Errorf("unit status after handle = %q, want disbanded (the arrival must not resurrect it)", status)
	}
}
