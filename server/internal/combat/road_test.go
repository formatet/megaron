package combat

// Vägar V2 (megaron_plan_vagar.md): order → stone caravan + army → build →
// RoadEdgeBuilt, with the refusals, both arrival orders and the failure modes.
// Fixture: setupLandMarchWorldWithHome — home city at (0,0), plains (0,1),
// (1,1), (2,1), (2,0), a mountain at (1,0). The road under test joins (1,1)-(2,1).

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/transport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type roadFixture struct {
	pool        *pgxpool.Pool
	world       uuid.UUID
	owner, home uuid.UUID
	army        uuid.UUID
	clk         clock.Clock
	sched       *events.Scheduler
	store       *events.Store
	hub         *fakeBroadcaster
	arrival     *UnitArrivalHandler
	sites       *RoadSiteHandler
}

func newRoadFixture(t *testing.T, stone float64) *roadFixture {
	t.Helper()
	pool, world, owner, home := setupLandMarchWorldWithHome(t)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Now())
	f := &roadFixture{pool: pool, world: world, owner: owner, home: home, clk: clk,
		sched: events.NewScheduler(pool, clk), store: events.NewStore(pool), hub: &fakeBroadcaster{}}
	f.arrival = newArrivalHandler(pool, nil)
	f.sites = NewRoadSiteHandler(pool, f.sched, f.store, f.hub)
	for _, tl := range []struct {
		q, r int
		t    string
	}{{3, 1, "river_ford"}, {3, 0, "hills"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1,$2,$3,$4)`, world, tl.q, tl.r, tl.t); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'stone', $2, 0, 1000000, current_world_tick())
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET amount = $2, rate = 0, calc_tick = current_world_tick()`,
		home, stone); err != nil {
		t.Fatalf("seed stone: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'positioned', 0, 1) RETURNING id`, world, owner,
	).Scan(&f.army); err != nil {
		t.Fatalf("create army: %v", err)
	}
	return f
}

func (f *roadFixture) stone(t *testing.T) float64 {
	t.Helper()
	var s float64
	if err := f.pool.QueryRow(context.Background(),
		`SELECT settled(amount, rate, calc_tick) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'stone'`, f.home).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (f *roadFixture) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f *roadFixture) order(fromQ, fromR, toQ, toR int) BuildRoadOrder {
	return BuildRoadOrder{WorldID: f.world, PlayerID: f.owner, UnitID: f.army, FromQ: fromQ, FromR: fromR, ToQ: toQ, ToR: toR}
}

// place runs the order's synchronous half in its own tx (what the API does).
func (f *roadFixture) place(t *testing.T, o BuildRoadOrder) (uuid.UUID, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	site, _, err := PlaceRoadSite(ctx, tx, f.sched, f.clk, o, f.home)
	if err != nil {
		_ = tx.Rollback(ctx)
		return uuid.Nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	o.SiteID = site
	return site, nil
}

// fire runs every unprocessed scheduled event of the given types, in id order,
// until none are left (handlers enqueue more). Due ticks are ignored: this
// stands in for "time passed".
func (f *roadFixture) fire(t *testing.T, types ...events.ScheduledEventType) int {
	t.Helper()
	ctx := context.Background()
	total := 0
	for round := 0; round < 10; round++ {
		rows, err := f.pool.Query(ctx,
			`SELECT id, event_type, payload, due_tick FROM scheduled_events
			  WHERE world_id = $1 AND processed_at IS NULL AND event_type = ANY($2) ORDER BY id`, f.world, typeStrings(types))
		if err != nil {
			t.Fatal(err)
		}
		var evs []events.ScheduledEvent
		for rows.Next() {
			var e events.ScheduledEvent
			var et string
			if err := rows.Scan(&e.ID, &et, &e.Payload, &e.DueTick); err != nil {
				t.Fatal(err)
			}
			e.EventType, e.WorldID = events.ScheduledEventType(et), f.world
			evs = append(evs, e)
		}
		rows.Close()
		if len(evs) == 0 {
			return total
		}
		for _, e := range evs {
			if _, err := f.pool.Exec(ctx, `UPDATE scheduled_events SET processed_at = now() WHERE id = $1`, e.ID); err != nil {
				t.Fatal(err)
			}
			var herr error
			switch e.EventType {
			case events.ScheduledTransportArrival:
				herr = transport.NewArrivalHandler(f.pool, nil, f.store).Handle(ctx, e)
			case events.ScheduledRoadSite:
				herr = f.sites.Handle(ctx, e)
			default:
				t.Fatalf("fire: unsupported type %s", e.EventType)
			}
			if herr != nil {
				t.Fatalf("handler %s: %v", e.EventType, herr)
			}
			total++
		}
	}
	t.Fatal("fire: did not settle")
	return total
}

func typeStrings(ts []events.ScheduledEventType) []string {
	out := make([]string, len(ts))
	for i, x := range ts {
		out[i] = string(x)
	}
	return out
}

// firePhase runs the pending RoadSite events of ONE phase, once (no looping),
// so a test can stop between "check" and "build".
func (f *roadFixture) firePhase(t *testing.T, phase string) {
	t.Helper()
	ctx := context.Background()
	rows, err := f.pool.Query(ctx,
		`SELECT id, payload FROM scheduled_events
		  WHERE world_id = $1 AND processed_at IS NULL AND event_type = 'RoadSite' AND payload->>'phase' = $2 ORDER BY id`, f.world, phase)
	if err != nil {
		t.Fatal(err)
	}
	var evs []events.ScheduledEvent
	for rows.Next() {
		var e events.ScheduledEvent
		if err := rows.Scan(&e.ID, &e.Payload); err != nil {
			t.Fatal(err)
		}
		e.WorldID = f.world
		evs = append(evs, e)
	}
	rows.Close()
	for _, e := range evs {
		if _, err := f.pool.Exec(ctx, `UPDATE scheduled_events SET processed_at = now() WHERE id = $1`, e.ID); err != nil {
			t.Fatal(err)
		}
		if err := f.sites.Handle(ctx, e); err != nil {
			t.Fatalf("road site %s: %v", phase, err)
		}
	}
}

func (f *roadFixture) edges(t *testing.T) int {
	return f.count(t, `SELECT count(*) FROM road_edges WHERE world_id = $1`, f.world)
}

func (f *roadFixture) siteState(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var s string
	if err := f.pool.QueryRow(context.Background(), `SELECT state FROM road_sites WHERE id = $1`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// The happy path with the CARAVAN arriving first: order → caravan arrives →
// (army marches and arrives) → build starts → edge finished. Plains edge: 8 stone, 1 tick.
func TestRoad_CaravanFirst_BuildsOneEdge(t *testing.T) {
	f := newRoadFixture(t, 100)
	ctx := context.Background()
	o := f.order(1, 1, 2, 1)

	site, err := f.place(t, o)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	o.SiteID = site
	if got := f.stone(t); got != 92 {
		t.Fatalf("home stone after order = %v, want 100-8", got)
	}
	var kind, status string
	var destID *uuid.UUID
	var destQ, destR int
	var qty float64
	if err := f.pool.QueryRow(ctx,
		`SELECT t.kind, t.status, t.dest_id, t.dest_q, t.dest_r, g.quantity
		   FROM road_sites rs JOIN transports t ON t.id = rs.stone_transport_id
		   JOIN transport_goods g ON g.transport_id = t.id AND g.good_key = 'stone'
		  WHERE rs.id = $1`, site).Scan(&kind, &status, &destID, &destQ, &destR, &qty); err != nil {
		t.Fatalf("load stone caravan: %v", err)
	}
	if kind != "road_stone" || status != "in_transit" || destID != nil || destQ != 1 || destR != 1 || qty != 8 {
		t.Fatalf("caravan = %s/%s dest=%v (%d,%d) qty=%v; want road_stone/in_transit, NULL city, hex (1,1), 8 stone", kind, status, destID, destQ, destR, qty)
	}

	// Caravan arrives: stone flagged, nothing is built yet (the army is not there).
	f.fire(t, events.ScheduledTransportArrival, events.ScheduledRoadSite)
	if f.siteState(t, site) != "waiting" || f.edges(t) != 0 {
		t.Fatalf("with only the stone there the site must still wait (state %s, edges %d)", f.siteState(t, site), f.edges(t))
	}
	if f.count(t, `SELECT count(*) FROM road_sites WHERE id = $1 AND stone_arrived AND NOT army_arrived`, site) != 1 {
		t.Fatal("stone_arrived should be set and army_arrived not")
	}

	// The Runner reaches the army: it marches to (1,1) and arrives there.
	if err := ExecuteBuildRoad(ctx, f.pool, f.sched, f.store, f.clk, o); err != nil {
		t.Fatalf("ExecuteBuildRoad: %v", err)
	}
	if f.siteState(t, site) != "waiting" {
		t.Fatalf("the march that walks the army to its road must not cancel the site, state=%s", f.siteState(t, site))
	}
	markUnitArrivalProcessed(t, f.pool, f.world, f.army)
	runFieldArrival(t, f.pool, f.arrival, f.world, f.army)
	f.firePhase(t, "check") // both there → building, the build event queued
	if f.siteState(t, site) != "building" {
		t.Fatalf("both there: state = %s, want building", f.siteState(t, site))
	}
	var due, created int
	if err := f.pool.QueryRow(ctx, `SELECT build_due_tick, created_tick FROM road_sites WHERE id = $1`, site).Scan(&due, &created); err != nil {
		t.Fatal(err)
	}
	if due-created != 1 {
		t.Fatalf("a plains edge for 100 men takes 1 tick, got %d", due-created)
	}
	f.firePhase(t, "build")
	if f.siteState(t, site) != "done" || f.edges(t) != 1 {
		t.Fatalf("after the build: state=%s edges=%d, want done/1", f.siteState(t, site), f.edges(t))
	}
	if n := f.count(t, `SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'RoadEdgeBuilt'`, site); n != 1 {
		t.Fatalf("RoadEdgeBuilt events = %d, want 1", n)
	}
	if len(f.hub.notified) == 0 || f.hub.notified[len(f.hub.notified)-1] != "RoadEdgeBuilt" {
		t.Fatalf("owner notifications = %v, want RoadEdgeBuilt last", f.hub.notified)
	}

	// The next trip over the edge is shorter by the army factor (0.75), both ways.
	for _, dir := range [][4]int{{1, 1, 2, 1}, {2, 1, 1, 1}} {
		_, cost, ok, err := province.FindPath(ctx, f.pool, f.world,
			province.MapPosition{Q: dir[0], R: dir[1]}, province.MapPosition{Q: dir[2], R: dir[3]}, "land")
		if err != nil || !ok || cost != 0.75 {
			t.Fatalf("land cost %v over the new road (ok=%v err=%v), want 0.75", cost, ok, err)
		}
	}
}

// ARMY first: it already stands at "from" when the Runner reaches it, so the
// flag is set at delivery; the caravan arriving later starts the build. Exactly
// one edge, and replaying the build handler adds nothing.
func TestRoad_ArmyFirst_BuildsOneEdge_AndIsIdempotent(t *testing.T) {
	f := newRoadFixture(t, 100)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE units SET q = 1, r = 1 WHERE id = $1`, f.army); err != nil {
		t.Fatal(err)
	}
	o := f.order(1, 1, 2, 1)
	site, err := f.place(t, o)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	o.SiteID = site
	if err := ExecuteBuildRoad(ctx, f.pool, f.sched, f.store, f.clk, o); err != nil {
		t.Fatalf("ExecuteBuildRoad: %v", err)
	}
	f.fire(t, events.ScheduledRoadSite)
	if f.siteState(t, site) != "waiting" {
		t.Fatalf("army there, no stone yet: state = %s, want waiting", f.siteState(t, site))
	}
	f.fire(t, events.ScheduledTransportArrival, events.ScheduledRoadSite)
	if f.siteState(t, site) != "done" || f.edges(t) != 1 {
		t.Fatalf("state=%s edges=%d, want done/1", f.siteState(t, site), f.edges(t))
	}

	// Replay: a second build event for the same site, and a second check, change nothing.
	for _, phase := range []string{"build", "check"} {
		payload := []byte(`{"site_id":"` + site.String() + `","phase":"` + phase + `"}`)
		if err := f.sites.Handle(ctx, events.ScheduledEvent{ID: 1, WorldID: f.world, Payload: payload}); err != nil {
			t.Fatalf("replay %s: %v", phase, err)
		}
	}
	if f.edges(t) != 1 || f.count(t, `SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'RoadEdgeBuilt'`, site) != 1 {
		t.Fatal("replaying the handler must not add an edge or an event")
	}
}

// Every refusal answers with a reason and leaves no trace: stone untouched, no site, no caravan.
func TestRoad_Refusals_LeaveNoTrace(t *testing.T) {
	cases := []struct {
		name     string
		stone    float64
		o        func(*roadFixture) BuildRoadOrder
		contains string
		prep     func(*testing.T, *roadFixture)
	}{
		{"into a ford", 100, func(f *roadFixture) BuildRoadOrder { return f.order(2, 1, 3, 1) }, "ford", nil},
		{"into a mountain", 100, func(f *roadFixture) BuildRoadOrder { return f.order(0, 0, 1, 0) }, "mountain", nil},
		{"not neighbours", 100, func(f *roadFixture) BuildRoadOrder { return f.order(0, 1, 2, 1) }, "neighbour", nil},
		{"off the map", 100, func(f *roadFixture) BuildRoadOrder { return f.order(2, 1, 2, 2) }, "not on the map", nil},
		{"too little stone", 5, func(f *roadFixture) BuildRoadOrder { return f.order(1, 1, 2, 1) }, "5 of 8 stone", nil},
		{"hills cost 16", 12, func(f *roadFixture) BuildRoadOrder { return f.order(2, 0, 3, 0) }, "12 of 16 stone", nil},
		{"already built", 100, func(f *roadFixture) BuildRoadOrder { return f.order(1, 1, 2, 1) }, "already joins",
			func(t *testing.T, f *roadFixture) {
				if _, err := f.pool.Exec(context.Background(),
					`INSERT INTO road_edges (world_id,q1,r1,q2,r2,built_tick) VALUES ($1,1,1,2,1,0)`, f.world); err != nil {
					t.Fatal(err)
				}
			}},
		{"a ship", 100, func(f *roadFixture) BuildRoadOrder { return f.order(1, 1, 2, 1) }, "land army",
			func(t *testing.T, f *roadFixture) {
				if _, err := f.pool.Exec(context.Background(), `UPDATE units SET type = 'galley', category = 'naval' WHERE id = $1`, f.army); err != nil {
					t.Fatal(err)
				}
			}},
		{"a marching army", 100, func(f *roadFixture) BuildRoadOrder { return f.order(1, 1, 2, 1) }, "marching",
			func(t *testing.T, f *roadFixture) {
				if _, err := f.pool.Exec(context.Background(), `UPDATE units SET status = 'marching' WHERE id = $1`, f.army); err != nil {
					t.Fatal(err)
				}
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newRoadFixture(t, c.stone)
			if c.prep != nil {
				c.prep(t, f)
			}
			_, err := f.place(t, c.o(f))
			var rej *OrderReject
			if !errors.As(err, &rej) {
				t.Fatalf("err = %v, want an OrderReject", err)
			}
			if rej.Status < 400 || rej.Status >= 500 || !strings.Contains(rej.Reason, c.contains) {
				t.Fatalf("refusal = %d %q, want a 4xx containing %q", rej.Status, rej.Reason, c.contains)
			}
			if got := f.stone(t); got != c.stone {
				t.Fatalf("a refused order must not touch the stone: %v, want %v", got, c.stone)
			}
			if n := f.count(t, `SELECT count(*) FROM road_sites WHERE world_id = $1`, f.world); n != 0 {
				t.Fatalf("a refused order left %d road site(s)", n)
			}
			if n := f.count(t, `SELECT count(*) FROM transports WHERE world_id = $1`, f.world); n != 0 {
				t.Fatalf("a refused order left %d transport(s)", n)
			}
		})
	}
}

// A caravan that never arrives (intercepted) means no road, the stone is gone,
// and the site stays visible as waiting (the failure mode must not be silent).
func TestRoad_InterceptedCaravan_NoRoad_SiteStaysWaiting(t *testing.T) {
	f := newRoadFixture(t, 100)
	ctx := context.Background()
	o := f.order(1, 1, 2, 1)
	site, err := f.place(t, o)
	if err != nil {
		t.Fatal(err)
	}
	o.SiteID = site
	if _, err := f.pool.Exec(ctx, `UPDATE transports SET status = 'intercepted' WHERE id = (SELECT stone_transport_id FROM road_sites WHERE id = $1)`, site); err != nil {
		t.Fatal(err)
	}
	f.fire(t, events.ScheduledTransportArrival) // arrival of an intercepted caravan is a no-op
	if err := ExecuteBuildRoad(ctx, f.pool, f.sched, f.store, f.clk, o); err != nil {
		t.Fatal(err)
	}
	markUnitArrivalProcessed(t, f.pool, f.world, f.army)
	runFieldArrival(t, f.pool, f.arrival, f.world, f.army)
	f.fire(t, events.ScheduledRoadSite)
	if f.edges(t) != 0 || f.siteState(t, site) != "waiting" {
		t.Fatalf("no stone, no road: edges=%d state=%s", f.edges(t), f.siteState(t, site))
	}
	if got := f.stone(t); got != 92 {
		t.Fatalf("stone is not refunded (B3): %v, want 92", got)
	}
	if f.count(t, `SELECT count(*) FROM road_sites WHERE id = $1 AND NOT stone_arrived AND army_arrived`, site) != 1 {
		t.Fatal("the site should show: army arrived, stone not")
	}
}

// A new order to the army cancels its road (stone lost); the build handler then builds nothing.
func TestRoad_NewOrderCancelsSite(t *testing.T) {
	f := newRoadFixture(t, 100)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE units SET q = 1, r = 1 WHERE id = $1`, f.army); err != nil {
		t.Fatal(err)
	}
	o := f.order(1, 1, 2, 1)
	site, err := f.place(t, o)
	if err != nil {
		t.Fatal(err)
	}
	o.SiteID = site
	if err := ExecuteBuildRoad(ctx, f.pool, f.sched, f.store, f.clk, o); err != nil {
		t.Fatal(err)
	}
	if _, err := StartMarch(ctx, f.pool, f.sched, f.store, f.clk, MarchOrder{
		WorldID: f.world, PlayerID: f.owner, UnitID: f.army, TargetQ: 2, TargetR: 1,
	}, nil); err != nil {
		t.Fatalf("StartMarch: %v", err)
	}
	if f.siteState(t, site) != "cancelled" {
		t.Fatalf("a new march order must cancel the road site, state=%s", f.siteState(t, site))
	}
	f.fire(t, events.ScheduledTransportArrival, events.ScheduledRoadSite)
	if f.edges(t) != 0 {
		t.Fatal("a cancelled site builds nothing")
	}
}

// The army is gone from its post when the build finishes: no road, a visible
// failure, the stone stays lost.
func TestRoad_ArmyLeavesBeforeFinish_NoRoad(t *testing.T) {
	f := newRoadFixture(t, 100)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE units SET q = 1, r = 1 WHERE id = $1`, f.army); err != nil {
		t.Fatal(err)
	}
	o := f.order(1, 1, 2, 1)
	site, err := f.place(t, o)
	if err != nil {
		t.Fatal(err)
	}
	o.SiteID = site
	if err := ExecuteBuildRoad(ctx, f.pool, f.sched, f.store, f.clk, o); err != nil {
		t.Fatal(err)
	}
	f.fire(t, events.ScheduledTransportArrival)
	f.firePhase(t, "check")
	if f.siteState(t, site) != "building" {
		t.Fatalf("state = %s, want building", f.siteState(t, site))
	}
	// The army is wiped out before the build event fires.
	if _, err := f.pool.Exec(ctx, `UPDATE units SET status = 'disbanded', q = NULL, r = NULL WHERE id = $1`, f.army); err != nil {
		t.Fatal(err)
	}
	f.firePhase(t, "build")
	if f.edges(t) != 0 || f.siteState(t, site) != "cancelled" {
		t.Fatalf("army gone: edges=%d state=%s, want 0/cancelled", f.edges(t), f.siteState(t, site))
	}
	found := false
	for i, k := range f.hub.notified {
		if k == "OrderFailed" {
			found = true
			if m, ok := f.hub.payloads[i].(map[string]any); !ok || m["verb"] != "build_road" {
				t.Fatalf("OrderFailed payload = %v, want verb build_road", f.hub.payloads[i])
			}
		}
	}
	if !found {
		t.Fatalf("the owner must be told the road failed, notifications: %v", f.hub.notified)
	}
}

// The army is drawn into a battle: same outcome — a visible failure, no road.
func TestRoad_ArmyInBattle_NoRoad(t *testing.T) {
	f := newRoadFixture(t, 100)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE units SET q = 1, r = 1 WHERE id = $1`, f.army); err != nil {
		t.Fatal(err)
	}
	o := f.order(1, 1, 2, 1)
	site, err := f.place(t, o)
	if err != nil {
		t.Fatal(err)
	}
	o.SiteID = site
	if err := ExecuteBuildRoad(ctx, f.pool, f.sched, f.store, f.clk, o); err != nil {
		t.Fatal(err)
	}
	f.fire(t, events.ScheduledTransportArrival)
	f.firePhase(t, "check")
	var battle uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO battles (world_id, q, r, status, started_tick, current_tick, seed) VALUES ($1, 1, 1, 'active', 0, 0, 1) RETURNING id`, f.world,
	).Scan(&battle); err != nil {
		t.Fatalf("insert battle: %v", err)
	}
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO battle_participants (battle_id, unit_id, owner_id, side, joined_tick, initial_size, current_size)
		 VALUES ($1, $2, $3, 'defender', 0, 100, 100)`, battle, f.army, f.owner); err != nil {
		t.Fatalf("insert participant: %v", err)
	}
	f.firePhase(t, "build")
	if f.edges(t) != 0 || f.siteState(t, site) != "cancelled" {
		t.Fatalf("battle: edges=%d state=%s, want 0/cancelled", f.edges(t), f.siteState(t, site))
	}
}
