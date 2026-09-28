package combat

// Acceptance 1 (megaron_plan_rorelse_sparad_vag.md §6): every one of the five
// march-dispatching write sites must save the EXACT path FindPath found (and
// its per-hex costs), run through its real handler/function — never a
// hand-built intermediate state. This file grows one write site at a time,
// one per commit, per the plan's arbetsordning.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// setupLandMarchWorld builds a small land map with a mountain wall forcing a
// detour — so a saved route is provably the REAL A* path, not a straight
// line, and has more than one step to check costs against.
//
//	(0,0) plains ── (1,0) mountain (blocked) ── (2,0) plains
//	   |                                            |
//	(0,1) plains ── (1,1) plains ── (2,1) plains ────
func setupLandMarchWorld(t *testing.T) (pool *pgxpool.Pool, worldID, ownerID uuid.UUID) {
	t.Helper()
	pool = testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"route-tester-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	tiles := []struct {
		q, r    int
		terrain string
	}{
		{0, 0, "plains"}, {1, 0, "mountain_limestone"}, {2, 0, "plains"},
		{0, 1, "plains"}, {1, 1, "plains"}, {2, 1, "plains"},
	}
	for _, tl := range tiles {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, tl.q, tl.r, tl.terrain,
		); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}
	return pool, worldID, ownerID
}

// wantRouteFor is the test's own oracle: a freshly reloaded tile graph (never
// shared state with the code under test), the real FindPath + StepHours, and
// the real BuildRoute — computed independently, so a match proves the write
// site saved the actual A* result, not a coincidence.
func wantRouteFor(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID, origin, target province.MapPosition, category string, departTick, arriveTick int) StoredRoute {
	t.Helper()
	g, err := province.LoadTileGraph(context.Background(), pool, worldID)
	if err != nil {
		t.Fatalf("LoadTileGraph: %v", err)
	}
	path, _, ok := g.FindPath(origin, target, category)
	if !ok {
		t.Fatalf("oracle FindPath found no route %v -> %v", origin, target)
	}
	steps := g.StepHours(path, category)
	route, ok := BuildRoute(path, steps, departTick, arriveTick)
	if !ok {
		t.Fatalf("oracle BuildRoute rejected path %v", path)
	}
	return route
}

// loadMarchRoute reads back a unit's saved route + tick pair.
func loadMarchRoute(t *testing.T, pool *pgxpool.Pool, unitID uuid.UUID) (raw []byte, departTick, arriveTick *int) {
	t.Helper()
	if err := pool.QueryRow(context.Background(),
		`SELECT march_route, depart_tick, arrive_tick FROM units WHERE id = $1`, unitID,
	).Scan(&raw, &departTick, &arriveTick); err != nil {
		t.Fatalf("load march_route: %v", err)
	}
	return raw, departTick, arriveTick
}

// assertRouteMatches checks the saved route against the oracle: same tick
// pair as depart_tick/arrive_tick AND as the oracle's own, same hexes, same
// costs.
func assertRouteMatches(t *testing.T, raw []byte, departTick, arriveTick *int, want StoredRoute) {
	t.Helper()
	if raw == nil {
		t.Fatal("march_route is NULL, want a saved route")
	}
	if departTick == nil || arriveTick == nil {
		t.Fatal("depart_tick/arrive_tick is NULL on a marching unit")
	}
	var got StoredRoute
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal march_route: %v", err)
	}
	if got.StartTick != *departTick || got.EndTick != *arriveTick {
		t.Errorf("route ticks = %d/%d, want depart_tick/arrive_tick %d/%d", got.StartTick, got.EndTick, *departTick, *arriveTick)
	}
	if got.StartTick != want.StartTick || got.EndTick != want.EndTick {
		t.Errorf("route ticks = %d/%d, want oracle %d/%d", got.StartTick, got.EndTick, want.StartTick, want.EndTick)
	}
	if len(got.Hexes) != len(want.Hexes) {
		t.Fatalf("route has %d hexes, want %d: got=%v want=%v", len(got.Hexes), len(want.Hexes), got.Hexes, want.Hexes)
	}
	for i := range want.Hexes {
		if got.Hexes[i] != want.Hexes[i] {
			t.Errorf("Hexes[%d] = %v, want %v (full: got=%v want=%v)", i, got.Hexes[i], want.Hexes[i], got.Hexes, want.Hexes)
		}
	}
	if len(got.Costs) != len(want.Costs) {
		t.Fatalf("route has %d costs, want %d", len(got.Costs), len(want.Costs))
	}
	for i := range want.Costs {
		if got.Costs[i] != want.Costs[i] {
			t.Errorf("Costs[%d] = %d, want %d", i, got.Costs[i], want.Costs[i])
		}
	}
}

// TestAcceptance1_StartMarch_SavesRoute: write site 1 (march_start.go
// StartMarch) saves the real detour path around the mountain, matching the
// oracle exactly.
func TestAcceptance1_StartMarch_SavesRoute(t *testing.T) {
	pool, worldID, ownerID := setupLandMarchWorld(t)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'positioned', 0, 0) RETURNING id`,
		worldID, ownerID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create positioned land unit: %v", err)
	}

	res, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: unitID,
		TargetQ: 2, TargetR: 0,
	}, nil)
	if err != nil {
		t.Fatalf("StartMarch: %v", err)
	}

	raw, departTick, arriveTick := loadMarchRoute(t, pool, unitID)
	want := wantRouteFor(t, pool, worldID,
		province.MapPosition{Q: 0, R: 0}, province.MapPosition{Q: 2, R: 0}, "land",
		res.ArrivalTick-res.DurationTicks, res.ArrivalTick)
	assertRouteMatches(t, raw, departTick, arriveTick, want)
	if len(want.Hexes) < 3 {
		t.Fatalf("test fixture is too weak: oracle path %v has no real detour to prove against a straight line", want.Hexes)
	}
}

// TestAcceptance1_StartMarch_ColonizeInPlace_SavesNoRoute: origin==target has
// no real path to search — march_route must be NULL, never a degenerate
// single-hex "route".
func TestAcceptance1_StartMarch_ColonizeInPlace_SavesNoRoute(t *testing.T) {
	pool, worldID, ownerID := setupLandMarchWorld(t)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'positioned', 0, 0) RETURNING id`,
		worldID, ownerID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create positioned land unit: %v", err)
	}

	if _, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: unitID,
		TargetQ: 0, TargetR: 0, Intent: "colonize",
	}, nil); err != nil {
		t.Fatalf("StartMarch(colonize-in-place): %v", err)
	}

	raw, _, _ := loadMarchRoute(t, pool, unitID)
	if raw != nil {
		t.Errorf("march_route = %s, want NULL for colonize-in-place", raw)
	}
}
