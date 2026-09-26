package combat

// R6 (megaron_plan_skeppsuppdrag_landsatt.md): the one-time deploy transition
// that sends a stray 'positioned' ship home once ships at sea stop taking
// orders (R3). Three scenarios: swept home, left alone (active patrol), left
// alone (stranded owner, R6's own exception).

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func sweepTestWorld(t *testing.T) uuid.UUID {
	t.Helper()
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
	return worldID
}

func TestSweepShipsAtSeaOnDeploy_PositionedShipSentHome(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID := sweepTestWorld(t)

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"sweep-owner-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	tiles := []struct {
		q, r    int
		terrain string
	}{
		{0, 0, "plains"}, {1, 0, "coastal_sea"}, {2, 0, "coastal_sea"},
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
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
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

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'positioned', 2, 0) RETURNING id`,
		worldID, ownerID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create positioned ship: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		hub:        &fakeBroadcaster{},
		scheduler:  events.NewScheduler(pool, clk),
		clk:        clk,
	}

	swept, err := SweepShipsAtSeaOnDeploy(ctx, pool, h)
	if err != nil {
		t.Fatalf("SweepShipsAtSeaOnDeploy: %v", err)
	}
	if swept != 1 {
		t.Errorf("swept = %d, want 1", swept)
	}

	var status string
	var marchIntent *string
	var homeSettlementID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT status, march_intent, home_settlement_id FROM units WHERE id = $1`, shipID,
	).Scan(&status, &marchIntent, &homeSettlementID); err != nil {
		t.Fatalf("load ship after sweep: %v", err)
	}
	if status != "marching" {
		t.Errorf("status = %q, want \"marching\" (sent home)", status)
	}
	if marchIntent == nil || *marchIntent != "explore_return" {
		t.Errorf("march_intent = %v, want \"explore_return\"", marchIntent)
	}
	if homeSettlementID == nil || *homeSettlementID != homeID {
		t.Errorf("home_settlement_id = %v, want %v", homeSettlementID, homeID)
	}

	// Idempotent: running it again finds nothing left to sweep.
	swept2, err := SweepShipsAtSeaOnDeploy(ctx, pool, h)
	if err != nil {
		t.Fatalf("SweepShipsAtSeaOnDeploy (2nd run): %v", err)
	}
	if swept2 != 0 {
		t.Errorf("2nd run swept = %d, want 0 (already sent home)", swept2)
	}
}

func TestSweepShipsAtSeaOnDeploy_ActivePatrolLeftAlone(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID := sweepTestWorld(t)

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"patrol-owner-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 5, 0, 'coastal_sea')`,
		worldID,
	); err != nil {
		t.Fatalf("insert map tile: %v", err)
	}

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, stance, sentry_q, sentry_r)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'positioned', 5, 0, 'sentry', 5, 0) RETURNING id`,
		worldID, ownerID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create patrolling ship: %v", err)
	}
	// A pending ScheduledSentryReturn row — this ship is actively on patrol;
	// its own timer already returns it home, R6 must not touch it.
	if _, err := pool.Exec(ctx,
		`INSERT INTO scheduled_events (world_id, event_type, payload, process_after, due_tick)
		 VALUES ($1, $2, $3::jsonb, now(), 100)`,
		worldID, string(events.ScheduledSentryReturn),
		`{"unit_id":"`+shipID.String()+`","world_id":"`+worldID.String()+`"}`,
	); err != nil {
		t.Fatalf("insert pending sentry return: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		hub:        &fakeBroadcaster{},
		scheduler:  events.NewScheduler(pool, clk),
		clk:        clk,
	}

	swept, err := SweepShipsAtSeaOnDeploy(ctx, pool, h)
	if err != nil {
		t.Fatalf("SweepShipsAtSeaOnDeploy: %v", err)
	}
	if swept != 0 {
		t.Errorf("swept = %d, want 0 — a patrolling ship must be left for its own timer", swept)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, shipID).Scan(&status); err != nil {
		t.Fatalf("load ship: %v", err)
	}
	if status != "positioned" {
		t.Errorf("status = %q, want still \"positioned\" (on patrol)", status)
	}
}

func TestSweepShipsAtSeaOnDeploy_StrandedOwnerLeftAlone(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID := sweepTestWorld(t)

	// R6's own exception: the owner holds NO active settlement at all — there
	// is nowhere to route this ship home to.
	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"castaway-owner-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 5, 0, 'coastal_sea')`,
		worldID,
	); err != nil {
		t.Fatalf("insert map tile: %v", err)
	}

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'positioned', 5, 0) RETURNING id`,
		worldID, ownerID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create stranded ship: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		hub:        &fakeBroadcaster{},
		scheduler:  events.NewScheduler(pool, clk),
		clk:        clk,
	}

	swept, err := SweepShipsAtSeaOnDeploy(ctx, pool, h)
	if err != nil {
		t.Fatalf("SweepShipsAtSeaOnDeploy: %v", err)
	}
	if swept != 0 {
		t.Errorf("swept = %d, want 0 — a stranded owner's ship has nowhere to go", swept)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, shipID).Scan(&status); err != nil {
		t.Fatalf("load ship: %v", err)
	}
	if status != "positioned" {
		t.Errorf("status = %q, want still \"positioned\" (left for the stranded exception)", status)
	}
}
