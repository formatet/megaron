package messenger

// Acceptance 1 (megaron_plan_rorelse_sparad_vag.md §6), write site 5:
// MarchRecallHandler.Handle (this package's own march_recall.go) saves the
// recalled/redirected leg's own route, and reads the unit's CURRENT position
// through the outbound route it saved at dispatch (R6.b) instead of
// re-walking it — proven by driving a unit through the REAL combat.StartMarch
// dispatch (not a hand-built fixture row) and then the real Handle().
//
// march_recall_test.go's own fixtures deliberately hand-insert marching units
// with no depart_tick/arrive_tick/march_route at all — that is the pre-153
// shape, and combat.LoadActiveRoute correctly falls back to the old re-walk
// for them (invariant 2). This file is the OTHER half: a unit that DOES have
// a saved route.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// setupMarchRecallRouteWorld builds the same mountain-detour land map used by
// combat's own march_route_test.go fixtures, plus a capital settlement at
// (0,0) (insertRecallMessenger needs one to resolve hex_q/hex_r from).
//
//	(0,0) plains ── (1,0) mountain (blocked) ── (2,0) plains
//	   |                                            |
//	(0,1) plains ── (1,1) plains ── (2,1) plains ────
func setupMarchRecallRouteWorld(t *testing.T) (pool *pgxpool.Pool, worldID, ownerID uuid.UUID) {
	t.Helper()
	pool = testPool(t)
	ctx := context.Background()

	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"route-recall-tester-"+uuid.New().String(),
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

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create origin province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Home', 'achaean', $3, 'capital', true)`,
		worldID, provinceID, ownerID,
	); err != nil {
		t.Fatalf("create home settlement: %v", err)
	}
	return pool, worldID, ownerID
}

// wantRouteFor mirrors combat's own test helper of the same name (unexported,
// package-local — this package cannot import combat's _test.go file) — a
// freshly reloaded tile graph, the real FindPath + StepHours + BuildRoute.
func wantRouteFor(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID, origin, target province.MapPosition, category string, departTick, arriveTick int) combat.StoredRoute {
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
	route, ok := combat.BuildRoute(path, steps, departTick, arriveTick)
	if !ok {
		t.Fatalf("oracle BuildRoute rejected path %v", path)
	}
	return route
}

func TestAcceptance1_MarchRecallHandler_RedirectSavesRoute(t *testing.T) {
	pool, worldID, ownerID := setupMarchRecallRouteWorld(t)
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

	res, err := combat.StartMarch(ctx, pool, scheduler, eventStore, clk, combat.MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: unitID,
		TargetQ: 2, TargetR: 0,
	}, nil)
	if err != nil {
		t.Fatalf("StartMarch: %v", err)
	}
	outboundWant := wantRouteFor(t, pool, worldID,
		province.MapPosition{Q: 0, R: 0}, province.MapPosition{Q: 2, R: 0}, "land",
		res.ArrivalTick-res.DurationTicks, res.ArrivalTick)
	if len(outboundWant.Hexes) < 3 {
		t.Fatalf("test fixture is too weak: oracle outbound path %v has no real detour", outboundWant.Hexes)
	}
	expectFromHex := outboundWant.Hexes[1] // val A: already entered at t=StartTick*1000

	// A dedup guard: StartMarch's own ScheduledUnitArrival must be marked
	// processed before the recall leg schedules its own at the same due_tick
	// (mirrors combat's march_start_land_test.go markUnitArrivalProcessed).
	if _, err := pool.Exec(ctx,
		`UPDATE scheduled_events SET processed_at = now()
		 WHERE world_id = $1 AND (payload->>'unit_id')::uuid = $2 AND processed_at IS NULL`,
		worldID, unitID,
	); err != nil {
		t.Fatalf("mark unit arrival processed: %v", err)
	}

	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM settlements WHERE world_id = $1 AND owner_id = $2 AND is_capital = true`,
		worldID, ownerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("load home settlement: %v", err)
	}
	var messengerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO messengers
		     (world_id, sender_id, origin_id, destination_id, message_text, status, kind, hex_q, hex_r, dest_q, dest_r, arrives_at)
		 VALUES ($1,$2,$3,NULL,'Redirect order','outbound','recall',0,0,2,1,$4)
		 RETURNING id`,
		worldID, ownerID, settlementID, clk.Now(),
	).Scan(&messengerID); err != nil {
		t.Fatalf("insert recall messenger: %v", err)
	}

	h := NewMarchRecallHandler(pool, scheduler, eventStore, nil, clk)
	newTargetQ, newTargetR := 2, 1
	payload := MarchRecallPayload{
		WorldID: worldID, UnitID: unitID, MessengerID: messengerID, Mode: "redirect",
		NewTargetQ: &newTargetQ, NewTargetR: &newTargetR,
	}
	raw, _ := json.Marshal(payload)
	if err := h.Handle(ctx, events.ScheduledEvent{Payload: raw}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	var q, r int
	var departTick, arriveTick *int
	var marchRouteRaw []byte
	if err := pool.QueryRow(ctx,
		`SELECT q, r, depart_tick, arrive_tick, march_route FROM units WHERE id = $1`, unitID,
	).Scan(&q, &r, &departTick, &arriveTick, &marchRouteRaw); err != nil {
		t.Fatalf("load unit after redirect: %v", err)
	}
	if q != expectFromHex[0] || r != expectFromHex[1] {
		t.Errorf("unit caught at (%d,%d), want %v (the saved-route answer, not the old re-walk's origin)", q, r, expectFromHex)
	}
	if departTick == nil || arriveTick == nil {
		t.Fatal("depart_tick/arrive_tick is NULL after redirect")
	}
	if marchRouteRaw == nil {
		t.Fatal("march_route is NULL, want the new leg's saved route")
	}
	var got combat.StoredRoute
	if err := json.Unmarshal(marchRouteRaw, &got); err != nil {
		t.Fatalf("unmarshal march_route: %v", err)
	}
	want := wantRouteFor(t, pool, worldID,
		province.MapPosition{Q: expectFromHex[0], R: expectFromHex[1]},
		province.MapPosition{Q: newTargetQ, R: newTargetR}, "land",
		*departTick, *arriveTick)
	if len(got.Hexes) != len(want.Hexes) {
		t.Fatalf("saved route has %d hexes, want %d: got=%v want=%v", len(got.Hexes), len(want.Hexes), got.Hexes, want.Hexes)
	}
	for i := range want.Hexes {
		if got.Hexes[i] != want.Hexes[i] {
			t.Errorf("Hexes[%d] = %v, want %v", i, got.Hexes[i], want.Hexes[i])
		}
	}
	for i := range want.Costs {
		if got.Costs[i] != want.Costs[i] {
			t.Errorf("Costs[%d] = %d, want %d", i, got.Costs[i], want.Costs[i])
		}
	}
}

// setupFordMarchRecallWorld is combat's own setupFordMarchWorld fixture
// (real-terrain T1/T7 analogue: (0,0) plains -> (1,0) river_ford (2.5h) ->
// (2,0) plains (0.75h), dominated by the ford) plus a home settlement, which
// insertRecallMessenger-style dispatch needs to resolve an order origin from.
func setupFordMarchRecallWorld(t *testing.T) (pool *pgxpool.Pool, worldID, ownerID uuid.UUID) {
	t.Helper()
	pool = testPool(t)
	ctx := context.Background()

	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"ford-recall-tester-"+uuid.New().String(),
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

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create origin province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Home', 'achaean', $3, 'capital', true)`,
		worldID, provinceID, ownerID,
	); err != nil {
		t.Fatalf("create home settlement: %v", err)
	}
	return pool, worldID, ownerID
}

// TestAcceptance2_MarchRecallHandler_CatchesWhereTimeSays is combat's
// TestAcceptance2_ExecuteRecall_CatchesWhereTimeSays for write site 5 (the
// budburen recall path, R6.b): same ford fixture, same 30%-through instant,
// same expected divergence from the old floor-based interpolation.
func TestAcceptance2_MarchRecallHandler_CatchesWhereTimeSays(t *testing.T) {
	pool, worldID, ownerID := setupFordMarchRecallWorld(t)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Now())
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET current_tick = 0, last_tick_at = $1 WHERE id = $2`, clk.Now(), worldID,
	); err != nil {
		t.Fatalf("align world tick anchor: %v", err)
	}
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

	res, err := combat.StartMarch(ctx, pool, scheduler, eventStore, clk, combat.MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: unitID,
		TargetQ: 2, TargetR: 0,
	}, nil)
	if err != nil {
		t.Fatalf("StartMarch: %v", err)
	}
	if res.DurationTicks != 3 {
		t.Fatalf("test fixture assumption broken: march took %d ticks, want 3", res.DurationTicks)
	}

	clk.Advance(time.Duration(0.3*3*float64(tick.TickSeconds)) * time.Second) // 30% through

	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM settlements WHERE world_id = $1 AND owner_id = $2 AND is_capital = true`,
		worldID, ownerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("load home settlement: %v", err)
	}
	var messengerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO messengers
		     (world_id, sender_id, origin_id, destination_id, message_text, status, kind, hex_q, hex_r, dest_q, dest_r, arrives_at)
		 VALUES ($1,$2,$3,NULL,'Recall order','outbound','recall',0,0,0,0,$4)
		 RETURNING id`,
		worldID, ownerID, settlementID, clk.Now(),
	).Scan(&messengerID); err != nil {
		t.Fatalf("insert recall messenger: %v", err)
	}

	h := NewMarchRecallHandler(pool, scheduler, eventStore, nil, clk)
	payload := MarchRecallPayload{WorldID: worldID, UnitID: unitID, MessengerID: messengerID, Mode: "recall"}
	raw, _ := json.Marshal(payload)
	if err := h.Handle(ctx, events.ScheduledEvent{Payload: raw}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	var q, r int
	if err := pool.QueryRow(ctx, `SELECT q, r FROM units WHERE id = $1`, unitID).Scan(&q, &r); err != nil {
		t.Fatalf("load unit after recall: %v", err)
	}
	if q != 1 || r != 0 {
		t.Errorf("MarchRecallHandler caught the unit at (%d,%d), want (1,0) (the ford — val A, already entered at departure)", q, r)
	}
}
