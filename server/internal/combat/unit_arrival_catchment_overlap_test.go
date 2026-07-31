package combat

// Delad-catchment-grind (Timothy 2026-07-27/28, server/delad-catchment-grind):
// "finns delat catchment kan staden inte grundas." This is the arrival-time,
// AUTHORITATIVE half of the gate in resolve() (unit_arrival.go), guarding the
// call into foundColony — the dispatch-time pre-flight in StartMarch
// (march_start.go) is covered by TestStartMarch_ColonizeRejectedByCatchmentOverlap.
// Mirrors the existing settlement-cap fallback test shape immediately above it
// in unit_arrival.go: on conflict, the unit just garrisons the hex instead of
// founding on top of a neighbour's fields.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestResolve_ColonizeBlockedByCatchmentOverlapFallsBackToGarrison(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

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

	mkPlayer := func(tag string) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
			tag+"-"+uuid.New().String(), tag+"-"+uuid.New().String()+"@test.invalid",
		).Scan(&id); err != nil {
			t.Fatalf("create player %s: %v", tag, err)
		}
		return id
	}
	founder := mkPlayer("founder")
	neighbor := mkPlayer("neighbor")

	// Founder's capital at (0,0) — foundColony (if reached) would look up the
	// owner's culture from it; also needed for the settlement-cap count.
	var founderCapProv uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&founderCapProv); err != nil {
		t.Fatalf("create founder capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Founder Home', 'achaean', $3, 'capital', true)`,
		worldID, founderCapProv, founder,
	); err != nil {
		t.Fatalf("create founder capital: %v", err)
	}

	// Neighbor's (foreign) settlement at (10,6) — one hex from the colonize
	// target, so their catchments overlap.
	var neighborProv uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 10, 6, 'plains') RETURNING id`,
		worldID,
	).Scan(&neighborProv); err != nil {
		t.Fatalf("create neighbor province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Neighbor City', 'khemetiu', $3, 'capital', true)`,
		worldID, neighborProv, neighbor,
	); err != nil {
		t.Fatalf("create neighbor settlement: %v", err)
	}

	// The colonize target itself: an empty province at (10,5), distance 1 from
	// the neighbor's (10,6).
	if _, err := pool.Exec(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 10, 5, 'plains')`,
		worldID,
	); err != nil {
		t.Fatalf("create target province: %v", err)
	}

	// The colonizing unit, already 'marching' and about to arrive at (10,5).
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r,
		                    target_q, target_r, march_intent)
		 VALUES ($1, $2, 'spearman', 'land', 10, 0, 'marching', 0, 0, 10, 5, 'colonize') RETURNING id`,
		worldID, founder,
	).Scan(&unitID); err != nil {
		t.Fatalf("create colonizing unit: %v", err)
	}

	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		hub:        &fakeBroadcaster{},
		scheduler:  events.NewScheduler(pool, clock.NewTestClock(time.Now())),
		clk:        clock.NewTestClock(time.Now()),
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := h.resolve(ctx, tx, unitID, worldID); err != nil {
		tx.Rollback(ctx)
		t.Fatalf("resolve: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// No colony must have been founded at the overlapping hex.
	var settlementCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE p.world_id = $1 AND p.map_q = 10 AND p.map_r = 5`,
		worldID,
	).Scan(&settlementCount); err != nil {
		t.Fatalf("count settlements at target: %v", err)
	}
	if settlementCount != 0 {
		t.Errorf("expected no settlement founded at the overlapping hex, found %d", settlementCount)
	}

	// The unit falls back to a peaceful garrison on the empty hex, same shape
	// as the settlement-cap fallback immediately above this check in
	// unit_arrival.go.
	var status string
	var q, r *int
	if err := pool.QueryRow(ctx,
		`SELECT status, q, r FROM units WHERE id = $1`, unitID,
	).Scan(&status, &q, &r); err != nil {
		t.Fatalf("reload unit: %v", err)
	}
	if status != "positioned" {
		t.Errorf("unit status = %q, want \"positioned\" (fell back to garrisoning the empty hex)", status)
	}
	if q == nil || r == nil || *q != 10 || *r != 5 {
		t.Errorf("unit position = (%v,%v), want (10,5)", q, r)
	}
}
