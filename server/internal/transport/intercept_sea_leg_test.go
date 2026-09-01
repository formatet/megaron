package transport

// Slice C — a land caravan whose origin/dest have no contiguous land route (an
// island-to-island leg, a sund, or post-flod a river gap) used to be permanently
// uninterceptable: province.InterpolatePosition returns ok=false when FindPath
// can't resolve the category, and Handle's `continue` silently skipped it forever.
// Timothy 2026-07-30 (locked): only messengers may be uninterceptable — everything
// else is a real, physical, seizable object. These tests prove the straight-line
// fallback in intercept.go puts such a caravan on a hex a sentry can actually reach,
// and that a caravan WITH a valid land route is completely unaffected (still comes
// from the real A* path, never the fallback).

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// newActiveTestWorld archives any leftover active world (the DB enforces at
// most one active world at a time, migration 063 one_active_world) and creates a
// fresh one, cleaned up (archived) at the end of the test. Mirrors the convention
// in transport_test.go's newFixture — kept local to this file per the slice's
// non-scope rule (transport_test.go is owned by a parallel branch).
//
// The archiving sweep is deliberately unfiltered, like every other fixture in
// the repo (transport_test.go:49, intercept_seize_idempotent_test.go:29,
// combat/battle_test.go's newBattleFixture). Until 2026-09-01 it carried an
// extra `AND name LIKE 'test-world-%'` — the only such narrowing in the
// codebase, and directly contrary to the convention the comment above claims
// to mirror. An earlier package in a `go test ./...` run leaves behind an
// active world under some other name, so both tests in this file died on
// `duplicate key value violates unique constraint "one_active_world"` whenever
// the suite ran end-to-end against a FRESH database. They passed in isolation,
// and passed again on a re-run of an already-populated DB, which is why the
// breakage stayed invisible: the suite only told the truth on a clean rig, and
// that is the one case nobody re-ran.
func newActiveTestWorld(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
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
	return worldID
}

func newTestPlayer(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"p-"+uuid.New().String(),
	).Scan(&id); err != nil {
		t.Fatalf("create test player: %v", err)
	}
	return id
}

// newCapitalFor gives owner a capital settlement at (q,r) so seize() has
// somewhere to credit the loot.
func newCapitalFor(t *testing.T, pool *pgxpool.Pool, worldID, owner uuid.UUID, q, r int) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var prov, capital uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, 'plains') RETURNING id`,
		worldID, q, r,
	).Scan(&prov); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1,$2,'Capital','achaean',$3,'capital',true,'active',5000) RETURNING id`,
		worldID, prov, owner,
	).Scan(&capital); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}
	return capital
}

func newSentryAt(t *testing.T, pool *pgxpool.Pool, worldID, owner uuid.UUID, q, r int) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, stance, q, r, sentry_q, sentry_r)
		 VALUES ($1,$2,'spearman','land',80,0,'positioned','sentry',$3,$4,$3,$4)`,
		worldID, owner, q, r); err != nil {
		t.Fatalf("create sentry: %v", err)
	}
}

// TestInterceptScan_SeaLegLandCaravanIsPlacedAndSeized is the RED-then-GREEN case
// (Slice C AK1+AK2). The world's only tiles are a 4-hex line (0,0)-(3,0) where
// (1,0) and (2,0) are deep_sea — a 'land' category caravan crossing it has no
// resolvable FindPath route (province.isPassable rejects sea for "land"), and no
// bypass exists because no other tile is in the map. Before the fix, Handle's
// `continue` skipped this caravan forever, even with a sentry sitting right on
// top of its straight-line position. After the fix it must be seized exactly like
// any other caravan.
func TestInterceptScan_SeaLegLandCaravanIsPlacedAndSeized(t *testing.T) {
	pool := testPool(t)
	worldID := newActiveTestWorld(t, pool)
	ctx := context.Background()

	// (0,0) origin plains, (1,0)/(2,0) impassable sea for a land caravan, (3,0) dest
	// plains. No tile exists off this line, so there is no bypass either.
	tiles := []struct {
		q, r    int
		terrain string
	}{
		{0, 0, "plains"},
		{1, 0, "deep_sea"},
		{2, 0, "deep_sea"},
		{3, 0, "plains"},
	}
	for _, tl := range tiles {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, tl.q, tl.r, tl.terrain,
		); err != nil {
			t.Fatalf("insert tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}

	owner := newTestPlayer(t, pool)
	raider := newTestPlayer(t, pool)
	raiderCapital := newCapitalFor(t, pool, worldID, raider, 9, 9)

	// The straight cube-lerp line from (0,0) to (3,0) at frac=1/3 lands exactly on
	// (1,0) with no rounding ambiguity (x=1.0, y=-1.0, z=0.0) — pick departs/arrives
	// so "now" sits at that fraction, and post the sentry there.
	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	departs := clk.Now().Add(-20 * time.Minute)
	arrives := clk.Now().Add(40 * time.Minute) // total 60 min, elapsed 20 min → frac 1/3
	newSentryAt(t, pool, worldID, raider, 1, 0)

	var caravan uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO transports
		   (world_id, owner_id, kind, category, origin_q, origin_r, dest_q, dest_r,
		    departs_at, arrives_at, due_tick, status, interceptable)
		 VALUES ($1,$2,'trade','land',0,0,3,0,$3,$4,1,'in_transit',true)
		 RETURNING id`,
		worldID, owner, departs, arrives,
	).Scan(&caravan); err != nil {
		t.Fatalf("create caravan: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_goods (transport_id, good_key, quantity) VALUES ($1,'silver',100)`, caravan,
	); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}

	h := NewInterceptScanHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil, clk)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: worldID, DueTick: 1}); err != nil {
		t.Fatalf("intercept scan: %v", err)
	}

	// AK1+AK2: the caravan was placed on a hex (rather than skipped) and seized —
	// same seize path, same status flip, same loot credit as an ordinary land leg.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM transports WHERE id=$1`, caravan).Scan(&status); err != nil {
		t.Fatalf("read caravan status: %v", err)
	}
	if status != "intercepted" {
		t.Fatalf("caravan status = %q, want intercepted (sea-leg caravan must no longer be permanently uninterceptable)", status)
	}

	var loot float64
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(settled(amount, rate, calc_tick), 0) FROM settlement_goods
		 WHERE settlement_id = $1 AND good_key = 'silver'`, raiderCapital,
	).Scan(&loot); err != nil {
		t.Fatalf("read raider capital silver: %v", err)
	}
	if loot != 100 {
		t.Errorf("raider capital silver = %v, want 100 (seized loot credited)", loot)
	}
}

// TestInterceptScan_PathableCaravanStillUsesRealPathNotFallback is Slice C AK3.
// origin (0,0) and dest (2,0) are only 2 hexes apart in a straight line, but the
// direct hex (1,0) is mountain (impassable), forcing A* to detour through
// (1,-1)->(2,-1). At frac=1/3 the real A* path sits at (1,-1); the straight cube
// line from (0,0) to (2,0) at the same fraction rounds to (1,0) instead — a
// different hex. A sentry posted at (1,-1) (the real path position) must catch
// the caravan; if the fallback were used instead of the real path, it would not
// (the straight line never touches (1,-1)), proving the fallback is not being
// used when a real route exists.
func TestInterceptScan_PathableCaravanStillUsesRealPathNotFallback(t *testing.T) {
	pool := testPool(t)
	worldID := newActiveTestWorld(t, pool)
	ctx := context.Background()

	tiles := []struct {
		q, r    int
		terrain string
	}{
		{0, 0, "plains"},             // origin
		{1, 0, "mountain_limestone"}, // blocks the direct 2-hex line
		{2, 0, "plains"},             // dest
		{1, -1, "plains"},            // detour
		{2, -1, "plains"},            // detour
	}
	for _, tl := range tiles {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, tl.q, tl.r, tl.terrain,
		); err != nil {
			t.Fatalf("insert tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}

	owner := newTestPlayer(t, pool)
	raider := newTestPlayer(t, pool)
	_ = newCapitalFor(t, pool, worldID, raider, 9, 9)

	// A* path is (0,0)->(1,-1)->(2,-1)->(2,0) (len 4, index 0..3). At frac=1/3,
	// idx = floor(1/3 * 3) = 1 → path[1] = (1,-1). The straight cube line for the
	// same frac rounds to (1,0) — a hex the real path never visits. Post the
	// sentry at the REAL path position so only "used the actual path" seizes it.
	clk := clock.NewTestClock(time.Unix(2_000_000, 0))
	departs := clk.Now().Add(-20 * time.Minute)
	arrives := clk.Now().Add(40 * time.Minute) // total 60 min, elapsed 20 min → frac 1/3
	newSentryAt(t, pool, worldID, raider, 1, -1)

	var caravan uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO transports
		   (world_id, owner_id, kind, category, origin_q, origin_r, dest_q, dest_r,
		    departs_at, arrives_at, due_tick, status, interceptable)
		 VALUES ($1,$2,'trade','land',0,0,2,0,$3,$4,1,'in_transit',true)
		 RETURNING id`,
		worldID, owner, departs, arrives,
	).Scan(&caravan); err != nil {
		t.Fatalf("create caravan: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_goods (transport_id, good_key, quantity) VALUES ($1,'silver',50)`, caravan,
	); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}

	h := NewInterceptScanHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil, clk)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: worldID, DueTick: 1}); err != nil {
		t.Fatalf("intercept scan: %v", err)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM transports WHERE id=$1`, caravan).Scan(&status); err != nil {
		t.Fatalf("read caravan status: %v", err)
	}
	if status != "intercepted" {
		t.Fatalf("caravan status = %q, want intercepted (sentry sits on the REAL A* path position (1,-1); "+
			"if the code used the straight-line fallback instead of the real path, it would round to (1,0) and miss this sentry)", status)
	}
}
