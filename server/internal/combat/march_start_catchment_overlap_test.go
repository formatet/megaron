package combat

// Delad-catchment-grind (Timothy 2026-07-27/28, server/delad-catchment-grind):
// "finns delat catchment kan staden inte grundas." This is the dispatch-time
// half — the pre-flight in StartMarch's colonize block (march_start.go) that
// gives the harness immediate feedback instead of wasting the march. The
// arrival-time authoritative gate (unit_arrival.go's resolve, guarding
// foundColony) is covered by TestResolve_ColonizeBlockedByCatchmentOverlapFallsBackToGarrison.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// TestStartMarch_ColonizeRejectedByCatchmentOverlap proves the dispatch-time
// pre-flight: marching a garrisoned land unit with intent=colonize onto a hex
// whose 7-hex catchment overlaps a FOREIGN settlement's is refused with 422,
// before the unit ever departs. Foreign, not own, on purpose — the invariant
// is owner-agnostic (Timothy: "gäller ALLA städer oavsett ägare").
func TestStartMarch_ColonizeRejectedByCatchmentOverlap(t *testing.T) {
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

	var founderID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"founder-"+uuid.New().String(), "founder-"+uuid.New().String()+"@test.invalid",
	).Scan(&founderID); err != nil {
		t.Fatalf("create founder player: %v", err)
	}
	var neighborID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"neighbor-"+uuid.New().String(), "neighbor-"+uuid.New().String()+"@test.invalid",
	).Scan(&neighborID); err != nil {
		t.Fatalf("create neighbor player: %v", err)
	}

	// Path plains (0,0)..(0,5); founder's capital sits at the origin, the
	// colonize target is (0,5). The neighbor's settlement is at (0,6) — one
	// hex past the target, off the march path entirely, so this proves the
	// overlap check itself catches the block, not an accidental path
	// collision with the neighbor's tile.
	for r := 0; r <= 6; r++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 0, $2, 'plains')`,
			worldID, r,
		); err != nil {
			t.Fatalf("insert map tile (0,%d): %v", r, err)
		}
	}

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
		worldID, founderCapProv, founderID,
	); err != nil {
		t.Fatalf("create founder capital: %v", err)
	}

	var neighborProv uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 6, 'plains') RETURNING id`,
		worldID,
	).Scan(&neighborProv); err != nil {
		t.Fatalf("create neighbor province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Neighbor City', 'khemetiu', $3, 'capital', true)`,
		worldID, neighborProv, neighborID,
	); err != nil {
		t.Fatalf("create neighbor settlement: %v", err)
	}

	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, settlement_id)
		 SELECT $1, $2, 'spearman', 'land', 10, 'garrison', id FROM settlements
		 WHERE world_id = $1 AND owner_id = $2
		 RETURNING id`,
		worldID, founderID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create colonizing unit: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	_, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: founderID, UnitID: unitID,
		TargetQ: 0, TargetR: 5, Intent: "colonize", Name: "Overlap City",
	}, nil)
	if err == nil {
		t.Fatal("StartMarch(colonize onto a hex overlapping a foreign settlement's catchment) succeeded, want a rejection")
	}
	var rej *OrderReject
	if !errors.As(err, &rej) {
		t.Fatalf("error is not an *OrderReject: %v", err)
	}
	if rej.Status != 422 {
		t.Errorf("rejection status = %d, want 422", rej.Status)
	}
	if !strings.Contains(rej.Reason, "already farmed") {
		t.Errorf("rejection reason = %q, want it to explain the catchment overlap", rej.Reason)
	}

	// The unit must never have been dispatched.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, unitID).Scan(&status); err != nil {
		t.Fatalf("reload unit: %v", err)
	}
	if status != "garrison" {
		t.Errorf("unit status = %q after rejection, want unchanged \"garrison\"", status)
	}
}
