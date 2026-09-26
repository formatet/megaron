package combat

// R4 (megaron_plan_skeppsuppdrag_landsatt.md): a naval unit's plain march (no
// intent) must end next to a settlement of its own — otherwise it would drift
// to open sea and become a "positioned" ship no order can ever reach again
// (R3), short of the one-time R6 sweep.

import (
	"context"
	"errors"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestStartMarch_NavalPlainMarchToOpenSeaRejected(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
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
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"open-sea-sailor-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	// Open sea, no settlement anywhere near the target.
	for _, tl := range []struct{ q, r int }{{0, 0}, {1, 0}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'coastal_sea')`,
			worldID, tl.q, tl.r,
		); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'garrison', NULL) RETURNING id`,
		worldID, ownerID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create garrisoned ship: %v", err)
	}
	// A garrisoned ship normally has settlement_id set; the naval origin
	// resolution in StartMarch needs a real settlement to derive a departure
	// sea hex from, so give it one right at the lane's start.
	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, -1, 0, 'plains') RETURNING q`,
		worldID,
	).Scan(new(int)); err != nil {
		t.Fatalf("insert home map tile: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, -1, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create home province: %v", err)
	}
	var homeID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Home', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, provinceID, ownerID,
	).Scan(&homeID); err != nil {
		t.Fatalf("create home settlement: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE units SET settlement_id = $1 WHERE id = $2`, homeID, shipID); err != nil {
		t.Fatalf("garrison ship at home: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	_, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: shipID,
		TargetQ: 1, TargetR: 0,
	}, nil)
	if err == nil {
		t.Fatal("StartMarch(plain naval march to open sea) succeeded, want a rejection — R4")
	}
	var rej *OrderReject
	if !errors.As(err, &rej) {
		t.Fatalf("error is not an *OrderReject: %v", err)
	}
	if rej.Status != 422 {
		t.Errorf("status = %d, want 422", rej.Status)
	}
}

func TestStartMarch_NavalPlainMarchToFriendlyPortDocks(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
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
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"port-sailor-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	// Home settlement at (-1,0); sea lane (0,0)-(1,0); destination port
	// settlement at (2,0), adjacent to the sea target (1,0).
	tiles := []struct {
		q, r    int
		terrain string
	}{
		{-1, 0, "plains"}, {0, 0, "coastal_sea"}, {1, 0, "coastal_sea"}, {2, 0, "plains"},
	}
	for _, tl := range tiles {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, tl.q, tl.r, tl.terrain,
		); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}
	var homeProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, -1, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&homeProvinceID); err != nil {
		t.Fatalf("create home province: %v", err)
	}
	var homeID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Home', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, homeProvinceID, ownerID,
	).Scan(&homeID); err != nil {
		t.Fatalf("create home settlement: %v", err)
	}
	var destProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 2, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&destProvinceID); err != nil {
		t.Fatalf("create dest province: %v", err)
	}
	var destID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Colony', 'achaean', $3, 'colony', false) RETURNING id`,
		worldID, destProvinceID, ownerID,
	).Scan(&destID); err != nil {
		t.Fatalf("create destination settlement: %v", err)
	}

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'garrison', $3) RETURNING id`,
		worldID, ownerID, homeID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create garrisoned ship: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	res, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: shipID,
		TargetQ: 1, TargetR: 0,
	}, nil)
	if err != nil {
		t.Fatalf("StartMarch(plain naval march to a hex next to a friendly port) failed — R4 should accept this: %v", err)
	}
	if res.TargetQ != 1 || res.TargetR != 0 {
		t.Errorf("target = (%d,%d), want (1,0)", res.TargetQ, res.TargetR)
	}

	// Run the arrival directly — proves R4's docking half too: the ship must
	// end up garrison AT the destination settlement, not "positioned" at the
	// sea hex it actually walked to.
	h := newArrivalHandler(pool, nil)
	runFieldArrival(t, pool, h, worldID, shipID)

	var status string
	var settlementID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT status, settlement_id FROM units WHERE id = $1`, shipID,
	).Scan(&status, &settlementID); err != nil {
		t.Fatalf("load ship after arrival: %v", err)
	}
	if status != "garrison" {
		t.Errorf("ship status = %q, want \"garrison\" — R4: a plain march to a hex next to a friendly port must dock there, not sit positioned at sea", status)
	}
	if settlementID == nil || *settlementID != destID {
		t.Errorf("ship settlement_id = %v, want %v (the destination port)", settlementID, destID)
	}
}
