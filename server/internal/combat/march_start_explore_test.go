package combat

// P7 soak fix (2026-07-19, "explore kräver garrison — moment 22 för skepp till
// havs"): before this fix, StartMarch flatly rejected intent=explore whenever
// the ordering unit was already field-positioned (no settlement_id) — a ship
// that had sailed out (e.g. via a plain march, or one that just dropped cargo
// via the P7 Unload fix) had no way to issue a further explore order short of
// sailing all the way home first. This test drives StartMarch directly against
// a field-positioned ship and proves the order is now accepted, resolving
// "home" to the player's nearest owned settlement instead of demanding the
// unit already be standing in one.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
)

func TestStartMarch_ExploreFromFieldPositionResolvesNearestOwnedHome(t *testing.T) {
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

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"far-explorer-"+uuid.New().String(), "far-explorer-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	// The player's one settlement, far from the ship — nearestOwnedSettlement
	// must still find it; it's the ONLY candidate, so "nearest" is trivial here.
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
		 VALUES ($1, $2, 'Home Port', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, capitalProvinceID, ownerID,
	).Scan(&capitalID); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	// Ship's current field position (5,0) and the explore target (6,0) — both
	// open sea, one hop apart, nowhere near the capital (proves this fix
	// doesn't require any path back to base at dispatch time — only that a
	// settlement exists to resolve as home).
	for _, tl := range []struct {
		q, r    int
		terrain string
	}{
		{5, 0, "coastal_sea"},
		{6, 0, "coastal_sea"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, tl.q, tl.r, tl.terrain,
		); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'positioned', 5, 0) RETURNING id`,
		worldID, ownerID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create field-positioned ship: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	res, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: shipID,
		TargetQ: 6, TargetR: 0, Intent: "explore",
	}, nil)
	if err != nil {
		t.Fatalf("StartMarch(explore, field-positioned ship) failed — the P7 moment-22 bug is back: %v", err)
	}
	if res.TargetQ != 6 || res.TargetR != 0 {
		t.Errorf("MarchStarted target = (%d,%d), want (6,0)", res.TargetQ, res.TargetR)
	}

	var status, marchIntent string
	var homeSettlementID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT status, COALESCE(march_intent, ''), home_settlement_id FROM units WHERE id = $1`, shipID,
	).Scan(&status, &marchIntent, &homeSettlementID); err != nil {
		t.Fatalf("load ship after dispatch: %v", err)
	}
	if status != "marching" {
		t.Errorf("ship status = %q, want \"marching\"", status)
	}
	if marchIntent != "explore" {
		t.Errorf("march_intent = %q, want \"explore\"", marchIntent)
	}
	if homeSettlementID == nil || *homeSettlementID != capitalID {
		t.Errorf("home_settlement_id = %v, want %v (the player's only, nearest owned settlement)", homeSettlementID, capitalID)
	}
}

// TestStartMarch_ExploreFromFieldPositionRejectedWithNoSettlement verifies the
// remaining, honest edge case: a field-positioned unit whose owner holds NO
// settlement at all still gets a clear, actionable rejection instead of a nil
// panic or a silently wrong "home".
func TestStartMarch_ExploreFromFieldPositionRejectedWithNoSettlement(t *testing.T) {
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

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"homeless-"+uuid.New().String(), "homeless-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	for _, tl := range []struct {
		q, r    int
		terrain string
	}{
		{5, 0, "coastal_sea"},
		{6, 0, "coastal_sea"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, tl.q, tl.r, tl.terrain,
		); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'positioned', 5, 0) RETURNING id`,
		worldID, ownerID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create field-positioned ship: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	_, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: shipID,
		TargetQ: 6, TargetR: 0, Intent: "explore",
	}, nil)
	if err == nil {
		t.Fatal("StartMarch(explore, no settlements at all) succeeded, want a rejection")
	}
	var rej *OrderReject
	if !errors.As(err, &rej) {
		t.Fatalf("error is not an *OrderReject: %v", err)
	}
	if rej.Status != 422 {
		t.Errorf("rejection status = %d, want 422", rej.Status)
	}
}
