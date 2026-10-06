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
	"formatet/megaron/server/internal/tick"
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

// setupLandMarchWorldWithHome is setupLandMarchWorld plus a home settlement
// at (0,0) — needed by any write site whose flow resolves a "home" to return
// to (explore's auto-return, dispatchReturnHome).
func setupLandMarchWorldWithHome(t *testing.T) (pool *pgxpool.Pool, worldID, ownerID, settlementID uuid.UUID) {
	t.Helper()
	pool, worldID, ownerID = setupLandMarchWorld(t)
	ctx := context.Background()

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create home province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Home', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, provinceID, ownerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create home settlement: %v", err)
	}
	return pool, worldID, ownerID, settlementID
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

// TestAcceptance1_DispatchReturnHome_SavesRoute: write site 2
// (unit_arrival.go dispatchReturnHome, reached through the real
// explore-mission flow: StartMarch(intent=explore) then the arrival handler
// turning the unit for home) saves the return leg's own detour route.
func TestAcceptance1_DispatchReturnHome_SavesRoute(t *testing.T) {
	pool, worldID, ownerID, settlementID := setupLandMarchWorldWithHome(t)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'garrison', $3) RETURNING id`,
		worldID, ownerID, settlementID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create garrisoned land unit: %v", err)
	}

	// Explore covers an area now (megaron_plan_upptackarexpeditionen.md):
	// its first leg goes to the nearest unseen hex there. Make (2,0) the only
	// unseen one, so the unit stands at (2,0) when the area is used up and
	// turns home from there — the detour this test is about.
	if _, err := pool.Exec(ctx,
		`INSERT INTO player_scouted_tiles (world_id, player_id, q, r)
		 SELECT $1, $2, q, r FROM map_tiles WHERE world_id = $1 AND NOT (q = 2 AND r = 0)`,
		worldID, ownerID,
	); err != nil {
		t.Fatalf("scout all but (2,0): %v", err)
	}
	if _, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: unitID,
		TargetQ: 2, TargetR: 0, Intent: "explore",
	}, nil); err != nil {
		t.Fatalf("StartMarch(explore): %v", err)
	}

	markUnitArrivalProcessed(t, pool, worldID, unitID)
	h := newArrivalHandler(pool, nil)
	runFieldArrival(t, pool, h, worldID, unitID) // reaches (2,0), turns for home via dispatchReturnHome

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, unitID).Scan(&status); err != nil {
		t.Fatalf("load unit after outbound arrival: %v", err)
	}
	if status != "marching" {
		t.Fatalf("unit status after reaching explore target = %q, want \"marching\" (turning for home)", status)
	}

	raw, departTick, arriveTick := loadMarchRoute(t, pool, unitID)
	if departTick == nil || arriveTick == nil {
		t.Fatal("depart_tick/arrive_tick is NULL on the return leg")
	}
	want := wantRouteFor(t, pool, worldID,
		province.MapPosition{Q: 2, R: 0}, province.MapPosition{Q: 0, R: 0}, "land",
		*departTick, *arriveTick)
	assertRouteMatches(t, raw, departTick, arriveTick, want)
	if len(want.Hexes) < 3 {
		t.Fatalf("test fixture is too weak: oracle return path %v has no real detour to prove against a straight line", want.Hexes)
	}
}

// TestAcceptance1_MarchShipToNearestOwnPort_SavesRoute: write site 3
// (ship_hull.go marchShipToNearestOwnPort, the damaged/captured-ship return
// leg) saves its own route. Called directly (it is itself the "real
// function" the plan names — its only callers are deep inside battle
// resolution, which this test does not need to reproduce to exercise it
// honestly).
func TestAcceptance1_MarchShipToNearestOwnPort_SavesRoute(t *testing.T) {
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
		"port-tester-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	// (0,0)/(1,0)/(2,0): a sea lane. (3,0): the home settlement's land hex,
	// adjacent to (2,0) — its harbour.
	for _, tl := range []struct {
		q, r    int
		terrain string
	}{
		{0, 0, "coastal_sea"}, {1, 0, "coastal_sea"}, {2, 0, "coastal_sea"}, {3, 0, "plains"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, tl.q, tl.r, tl.terrain,
		); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}
	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 3, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create home province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Home Port', 'achaean', $3, 'capital', true)`,
		worldID, provinceID, ownerID,
	); err != nil {
		t.Fatalf("create home settlement: %v", err)
	}

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'positioned', 0, 0) RETURNING id`,
		worldID, ownerID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create damaged ship: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	const tickIndex = 5

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := marchShipToNearestOwnPort(ctx, tx, clk, scheduler, worldID, shipID, ownerID, "galley", tickIndex, "damaged_return"); err != nil {
		tx.Rollback(ctx)
		t.Fatalf("marchShipToNearestOwnPort: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	raw, departTick, arriveTick := loadMarchRoute(t, pool, shipID)
	if departTick == nil || arriveTick == nil {
		t.Fatal("depart_tick/arrive_tick is NULL after marchShipToNearestOwnPort")
	}
	if *departTick != tickIndex {
		t.Errorf("depart_tick = %d, want %d (tickIndex)", *departTick, tickIndex)
	}
	want := wantRouteFor(t, pool, worldID,
		province.MapPosition{Q: 0, R: 0}, province.MapPosition{Q: 2, R: 0}, "naval",
		*departTick, *arriveTick)
	assertRouteMatches(t, raw, departTick, arriveTick, want)
}

// TestAcceptance1_ExecuteRecall_RedirectSavesRoute: write site 4
// (recall_redirect.go ExecuteRecall) plus read site (a) — same function.
// A redirect is issued the instant after dispatch (clk never advances), so
// "current position" is read at Milli == the outbound route's own StartTick*1000
// exactly. That is the R1 "val A" boundary: the unit has already left the
// start hex (instantly) and entered Hexes[1] of the OUTBOUND route — a
// different answer than the old province.InterpolatePosition would give at
// the same instant (frac=0 -> the origin hex). This proves ExecuteRecall is
// reading through the saved route (R6.a), not the old code, AND that the new
// leg's own route is saved correctly (R5).
func TestAcceptance1_ExecuteRecall_RedirectSavesRoute(t *testing.T) {
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
	outboundDepart := res.ArrivalTick - res.DurationTicks
	outboundWant := wantRouteFor(t, pool, worldID,
		province.MapPosition{Q: 0, R: 0}, province.MapPosition{Q: 2, R: 0}, "land",
		outboundDepart, res.ArrivalTick)
	if len(outboundWant.Hexes) < 3 {
		t.Fatalf("test fixture is too weak: oracle outbound path %v has no real detour", outboundWant.Hexes)
	}
	expectFromHex := outboundWant.Hexes[1] // val A: already entered at t=StartTick*1000

	markUnitArrivalProcessed(t, pool, worldID, unitID)
	newTargetQ, newTargetR := 2, 1
	applied, err := ExecuteRecall(ctx, pool, scheduler, eventStore, clk, RecallOrder{
		WorldID: worldID, UnitID: unitID, Mode: "redirect",
		NewTargetQ: &newTargetQ, NewTargetR: &newTargetR,
	})
	if err != nil {
		t.Fatalf("ExecuteRecall(redirect): %v", err)
	}
	if applied == nil {
		t.Fatal("ExecuteRecall returned (nil, nil) — unit was no longer marching?")
	}
	if applied.FromQ != expectFromHex[0] || applied.FromR != expectFromHex[1] {
		t.Errorf("ExecuteRecall read current position (%d,%d), want %v (the saved-route answer, not the old re-walk's origin)",
			applied.FromQ, applied.FromR, expectFromHex)
	}

	raw, departTick, arriveTick := loadMarchRoute(t, pool, unitID)
	if departTick == nil || arriveTick == nil {
		t.Fatal("depart_tick/arrive_tick is NULL after redirect")
	}
	want := wantRouteFor(t, pool, worldID,
		province.MapPosition{Q: expectFromHex[0], R: expectFromHex[1]},
		province.MapPosition{Q: newTargetQ, R: newTargetR}, "land",
		*departTick, *arriveTick)
	assertRouteMatches(t, raw, departTick, arriveTick, want)
}

// setupFordMarchWorld builds the real-terrain analogue of movement's own T1/T7
// fixture: (0,0) plains -> (1,0) river_ford (steep, 2.5h) -> (2,0) plains
// (0.75h) — a real ford hex, passable for land, dominating the journey's cost
// exactly like movement_test.go's T1. Used for acceptance 2 (megaron_plan_
// rorelse_sparad_vag.md §6): a chosen instant where the OLD floor-based
// interpolation and the core disagree on which hex the unit occupies.
func setupFordMarchWorld(t *testing.T) (pool *pgxpool.Pool, worldID, ownerID uuid.UUID) {
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
		"ford-tester-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	for _, tl := range []struct {
		q, r    int
		terrain string
	}{
		{0, 0, "plains"}, {1, 0, "river_ford"}, {2, 0, "plains"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, tl.q, tl.r, tl.terrain,
		); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}
	return pool, worldID, ownerID
}

// alignWorldTickAnchor pins worlds.current_tick/last_tick_at to clk's own
// timeline: tick 0 starts exactly at clk.Now(). A fast unit test never runs
// the real tick worker, so the DB's last_tick_at default (real wall time at
// world creation) would otherwise be unrelated to the TestClock a march is
// dispatched against — tick.Anchor.MilliAt needs the two aligned to mean
// anything.
func alignWorldTickAnchor(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID, clk *clock.TestClock) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE worlds SET current_tick = 0, last_tick_at = $1 WHERE id = $2`, clk.Now(), worldID,
	); err != nil {
		t.Fatalf("align world tick anchor: %v", err)
	}
}

// TestAcceptance2_ExecuteRecall_CatchesWhereTimeSays: at 30% of a 3-tick
// march dominated by the ford's cost, the OLD floor-based interpolation
// (idx=floor(0.3*2)=0) says the unit is still at the ORIGIN — but the core
// says it already entered the ford at departure (val A: the cost of a hex is
// the time spent standing in it, and the ford's b_0 lands at ~77% of the
// journey, well after 30%). ExecuteRecall must start the home leg from the
// core's answer, not the old one.
func TestAcceptance2_ExecuteRecall_CatchesWhereTimeSays(t *testing.T) {
	pool, worldID, ownerID := setupFordMarchWorld(t)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Now())
	alignWorldTickAnchor(t, pool, worldID, clk)
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
	if res.DurationTicks != 3 {
		t.Fatalf("test fixture assumption broken: march took %d ticks, want 3 (2.5h ford + 0.75h plains, rounds to 3)", res.DurationTicks)
	}

	// 30% of the 3-tick journey.
	clk.Advance(time.Duration(0.3*3*float64(tick.TickSeconds)) * time.Second)

	applied, err := ExecuteRecall(ctx, pool, scheduler, eventStore, clk, RecallOrder{
		WorldID: worldID, UnitID: unitID, Mode: "recall",
	})
	if err != nil {
		t.Fatalf("ExecuteRecall: %v", err)
	}
	if applied == nil {
		t.Fatal("ExecuteRecall returned (nil, nil) — unit was no longer marching?")
	}
	if applied.FromQ != 1 || applied.FromR != 0 {
		t.Errorf("ExecuteRecall caught the unit at (%d,%d), want (1,0) (the ford — val A, already entered at departure)", applied.FromQ, applied.FromR)
	}
}
