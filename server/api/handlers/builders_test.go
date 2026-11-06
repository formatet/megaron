package handlers

// More hands = faster work (mig 245, megaron_byggare_verkstad.md): shipyard rate =
// placed workers capped at 3, Workshop builders set the construction pace (base 1x),
// and the build queue runs ONE active build with the rest waiting.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func placeShipyardWorkers(t *testing.T, pool *pgxpool.Pool, city uuid.UUID, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		if _, err := pool.Exec(context.Background(), `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES($1,$2,'building','shipyard','shipbuilding')`, city, i); err != nil {
			t.Fatal(err)
		}
	}
}

func shipDue(t *testing.T, f shipRepairFixture) (due int, tick int, ok bool) {
	t.Helper()
	ctx := context.Background()
	if err := f.pool.QueryRow(ctx, `SELECT current_tick FROM worlds WHERE id=$1`, f.worldID).Scan(&tick); err != nil {
		t.Fatal(err)
	}
	err := f.pool.QueryRow(ctx, `SELECT due_tick FROM scheduled_events WHERE event_type='ShipyardWorkComplete' AND payload->>'unit_id'=$1 AND processed_at IS NULL`, f.shipID.String()).Scan(&due)
	return due, tick, err == nil
}

// W=12 with 1/2/3 placed workers finishes after ceil(12/n) = 12/6/4 days; a fourth
// worker on a level-3 yard changes nothing (cap 3); nobody = no timer at all.
func TestShipyardRateIsPlacedCountCappedAtThree(t *testing.T) {
	for _, c := range []struct{ placed, days int }{{1, 12}, {2, 6}, {3, 4}, {4, 4}} {
		f := newShipRepairFixture(t, true, map[string]float64{"timber": 1000, "silver": 1000})
		ctx := context.Background()
		if _, err := f.pool.Exec(ctx, `UPDATE buildings SET level=3 WHERE settlement_id=$1`, f.settlementID); err != nil {
			t.Fatal(err)
		}
		if r := f.postRepair(t); r.Code != http.StatusAccepted {
			t.Fatal(r.Body.String())
		}
		placeShipyardWorkers(t, f.pool, f.settlementID, c.placed)
		if err := economy.StartShipyardWork(ctx, f.pool, f.shipID, 12); err != nil {
			t.Fatal(err)
		}
		due, now, ok := shipDue(t, f)
		if !ok || due-now != c.days {
			t.Fatalf("%d workers: finish in %d days (ok=%v), want %d", c.placed, due-now, ok, c.days)
		}
	}
	f := newShipRepairFixture(t, true, map[string]float64{"timber": 1000, "silver": 1000})
	if r := f.postRepair(t); r.Code != http.StatusAccepted {
		t.Fatal(r.Body.String())
	}
	if _, _, ok := shipDue(t, f); ok {
		t.Fatal("an empty yard must not arm a timer")
	}
}

// Changing the crew mid-job settles the old rate first: no work is lost or counted twice.
func TestShipyardCrewChangeMidJobKeepsRemainingWork(t *testing.T) {
	f := newShipRepairFixture(t, true, map[string]float64{"timber": 1000, "silver": 1000})
	ctx := context.Background()
	exec := func(q string, a ...any) {
		t.Helper()
		if _, err := f.pool.Exec(ctx, q, a...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE buildings SET level=3 WHERE settlement_id=$1`, f.settlementID)
	if r := f.postRepair(t); r.Code != http.StatusAccepted {
		t.Fatal(r.Body.String())
	}
	placeShipyardWorkers(t, f.pool, f.settlementID, 1)
	if err := economy.StartShipyardWork(ctx, f.pool, f.shipID, 12); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE worlds SET current_tick=current_tick+4 WHERE id=$1`, f.worldID) // 1x for 4 days -> 8 left
	exec(`INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES($1,2,'building','shipyard','shipbuilding'),($1,3,'building','shipyard','shipbuilding')`, f.settlementID)
	w, err := economy.LoadShipyardWork(ctx, f.pool, f.shipID)
	if err != nil || w.Remaining == nil || *w.Remaining != 8 {
		t.Fatalf("after the 1x stretch 8 must remain, got %+v err=%v", w, err)
	}
	due, now, _ := shipDue(t, f)
	if due-now != 3 { // 3 workers now: ceil(8/3)
		t.Fatalf("8 left at 3 workers = ceil(8/3)=3 days, got %d", due-now)
	}
	exec(`UPDATE worlds SET current_tick=current_tick+2 WHERE id=$1`, f.worldID) // 3x for 2 days -> 2 left
	exec(`DELETE FROM settlement_placement WHERE settlement_id=$1`, f.settlementID)
	if w, err = economy.LoadShipyardWork(ctx, f.pool, f.shipID); err != nil || w.Remaining == nil || *w.Remaining != 2 || !w.Waiting {
		t.Fatalf("stopping must keep 2 days of work, got %+v err=%v", w, err)
	}
}

type queueFixture struct {
	pool     *pgxpool.Pool
	world    uuid.UUID
	city     uuid.UUID
	province uuid.UUID
	router   *chi.Mux
	token    string
}

func newQueueFixture(t *testing.T) queueFixture {
	t.Helper()
	pool := settlementsOverviewTestPool(t)
	ctx := context.Background()
	wid := settlementsOverviewTestWorld(t, pool, ctx)
	svc := auth.NewService(pool, "test-secret")
	owner, token := settlementsOverviewRegisterPlayer(t, svc, ctx, "builders-"+uuid.New().String())
	sid := settlementsOverviewSeedSettlement(t, pool, ctx, wid, owner, "Workshopia", true, 0, 0)
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'timber',200,0,5000,1000),($1,'stone',200,0,5000,1000)`, sid); err != nil {
		t.Fatal(err)
	}
	var pid uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT province_id FROM settlements WHERE id=$1`, sid).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	ph := NewProvinceHandler(pool, nil, clock.NewTestClock(time.Unix(0, 0)), nil, nil)
	r := chi.NewRouter()
	r.Use(auth.Middleware(svc))
	r.Get("/worlds/{worldID}/provinces/{provinceID}", ph.Get)
	r.Post("/worlds/{worldID}/provinces/{provinceID}/build", ph.Build)
	r.Delete("/worlds/{worldID}/provinces/{provinceID}/build-queue/{queueID}", ph.CancelBuild)
	return queueFixture{pool, wid, sid, pid, r, token}
}

func (f queueFixture) call(t *testing.T, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, "/worlds/"+f.world.String()+"/provinces/"+f.province.String()+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (f queueFixture) build(t *testing.T, typ string) (int, string) {
	t.Helper()
	code, body := f.call(t, "POST", "/build", `{"building_type":"`+typ+`"}`)
	id, _ := body["queue_id"].(string)
	if code != 201 {
		t.Logf("build %s -> %d %v", typ, code, body)
	}
	return code, id
}

// events pending for this city's builds: queue id -> due tick
func (f queueFixture) pending(t *testing.T) map[string]int {
	t.Helper()
	rows, err := f.pool.Query(context.Background(), `SELECT payload->>'build_queue_id', due_tick FROM scheduled_events WHERE world_id=$1 AND event_type='BuildComplete' AND processed_at IS NULL`, f.world)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var due int
		if err := rows.Scan(&id, &due); err != nil {
			t.Fatal(err)
		}
		out[id] = due
	}
	return out
}

func (f queueFixture) finish(t *testing.T, id string) {
	t.Helper()
	var payload []byte
	if err := f.pool.QueryRow(context.Background(), `SELECT payload FROM scheduled_events WHERE event_type='BuildComplete' AND payload->>'build_queue_id'=$1 AND processed_at IS NULL`, id).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	h := combat.NewBuildCompleteHandler(f.pool, events.NewStore(f.pool), nil)
	if err := h.Handle(context.Background(), events.ScheduledEvent{WorldID: f.world, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	f.pool.Exec(context.Background(), `UPDATE scheduled_events SET processed_at=now() WHERE event_type='BuildComplete' AND payload->>'build_queue_id'=$1 AND processed_at IS NULL`, id) // the worker's job
}

// One active build, two waiting, a fourth is refused; the next starts only when the
// one before it is done or cancelled; cancelling a waiting build touches nobody else.
func TestBuildQueueRunsOneActiveAndTwoWaiting(t *testing.T) {
	f := newQueueFixture(t)
	_, a := f.build(t, "market")
	_, b := f.build(t, "stable")
	_, c := f.build(t, "pen")
	code, _ := f.call(t, "POST", "/build", `{"building_type":"silo"}`)
	if code != 422 {
		t.Fatalf("4th build must be refused, got %d", code)
	}
	p := f.pending(t)
	if len(p) != 1 || p[a] != 1008 { // market: 8 days at base pace from tick 1000
		t.Fatalf("only the first build may have a timer (market due 1008): %v", p)
	}
	// finishing the first starts the second at the current tick
	f.pool.Exec(context.Background(), `UPDATE worlds SET current_tick=1008 WHERE id=$1`, f.world)
	f.finish(t, a)
	p = f.pending(t)
	if len(p) != 1 || p[b] != 1008+8 { // stable: 8 days
		t.Fatalf("second build must start when the first completes: %v", p)
	}
	// cancel the WAITING third: the active second is untouched
	if code, body := f.call(t, "DELETE", "/build-queue/"+c, ""); code != 200 {
		t.Fatalf("cancel waiting: %d %v", code, body)
	}
	if p2 := f.pending(t); len(p2) != 1 || p2[b] != 1016 {
		t.Fatalf("cancelling a waiting build must not move the active one: %v", p2)
	}
	// queue another, then cancel the ACTIVE one: the waiting one starts
	_, d := f.build(t, "pen")
	f.pool.Exec(context.Background(), `UPDATE worlds SET current_tick=1010 WHERE id=$1`, f.world)
	if code, _ := f.call(t, "DELETE", "/build-queue/"+b, ""); code != 200 && code != 204 {
		t.Fatalf("cancel active: %d", code)
	}
	if p3 := f.pending(t); len(p3) != 1 || p3[d] != 1010+4 { // pen: 4 days
		t.Fatalf("cancelling the active build must start the next: %v", p3)
	}
	var timber float64
	f.pool.QueryRow(context.Background(), `SELECT settled(amount,rate,calc_tick) FROM settlement_goods WHERE settlement_id=$1 AND good_key='timber'`, f.city).Scan(&timber)
	// paid: market 10 + stable 12 + pen 6 + pen 6; refunded: pen 6 (waiting) + stable 12 (active)
	if want := 200.0 - 10 - 6; timber != want {
		t.Fatalf("timber %g, want %g (cancel refunds as before)", timber, want)
	}
}

// Workshop builders: 0/1/2/3(/4) placed -> 12 days of work take 12/12/6/4(/4) days;
// no Workshop at all is also base pace.
func TestWorkshopBuildersSetBuildPace(t *testing.T) {
	for _, c := range []struct {
		workshop bool
		placed   int
		days     int
		work     int // 0 = 12
	}{{false, 0, 12, 0}, {true, 0, 12, 0}, {true, 1, 12, 0}, {true, 2, 6, 0}, {true, 3, 4, 0}, {true, 4, 4, 0}, {true, 3, 4, 10}} { // 10 days at 3x = ceil(10/3) = 4, never 3
		f := newQueueFixture(t)
		ctx := context.Background()
		if c.workshop {
			if _, err := f.pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'workshop',3)`, f.city); err != nil {
				t.Fatal(err)
			}
		}
		work := 12
		if c.work != 0 {
			work = c.work
		}
		var q uuid.UUID
		if err := f.pool.QueryRow(ctx, `INSERT INTO build_queue(world_id,settlement_id,building_type,complete_at,work_remaining) VALUES($1,$2,'harbour',now(),$3) RETURNING id`, f.world, f.city, work).Scan(&q); err != nil {
			t.Fatal(err)
		}
		for i := 1; i <= c.placed; i++ {
			if _, err := f.pool.Exec(ctx, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES($1,$2,'building','workshop','building')`, f.city, i); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := f.pool.Exec(ctx, `SELECT sync_build_queue($1)`, f.city); err != nil {
			t.Fatal(err)
		}
		if due := f.pending(t)[q.String()]; due != 1000+c.days {
			t.Fatalf("workshop=%v builders=%d: due %d, want %d", c.workshop, c.placed, due, 1000+c.days)
		}
	}
}

// Builders arriving or leaving mid-build re-time the build without losing work.
func TestWorkshopCrewChangeMidBuildKeepsWork(t *testing.T) {
	f := newQueueFixture(t)
	ctx := context.Background()
	f.pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'workshop',3)`, f.city)
	_, a := f.build(t, "market")                                                 // 8 days at tick 1000, base pace
	f.pool.Exec(ctx, `UPDATE worlds SET current_tick=1004 WHERE id=$1`, f.world) // 4 left
	f.pool.Exec(ctx, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES($1,1,'building','workshop','building'),($1,2,'building','workshop','building')`, f.city)
	if due := f.pending(t)[a]; due != 1004+2 { // 4 left at 2x
		t.Fatalf("due %d, want 1006", due)
	}
	f.pool.Exec(ctx, `UPDATE worlds SET current_tick=1005 WHERE id=$1`, f.world) // 2 left
	f.pool.Exec(ctx, `DELETE FROM settlement_placement WHERE settlement_id=$1`, f.city)
	if due := f.pending(t)[a]; due != 1005+2 { // 2 left at base pace
		t.Fatalf("due %d, want 1007", due)
	}
	// the superseded timer is dead: a stale event cannot finish the build
	var stale string
	f.pool.QueryRow(ctx, `SELECT payload::text FROM scheduled_events WHERE payload->>'build_queue_id'=$1 AND processed_at IS NOT NULL ORDER BY id LIMIT 1`, a).Scan(&stale)
	h := combat.NewBuildCompleteHandler(f.pool, events.NewStore(f.pool), nil)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.world, Payload: []byte(stale)}); err != nil {
		t.Fatal(err)
	}
	var n int
	f.pool.QueryRow(ctx, `SELECT count(*) FROM build_queue WHERE id=$1`, a).Scan(&n)
	if n != 1 {
		t.Fatal("a stale BuildComplete must not finish the build")
	}
}

// A Workshop can be placed in and the slots follow the level (1/2/3).
func TestWorkshopHasOnePlacePerLevel(t *testing.T) {
	for lvl, want := range map[int]int{1: 1, 2: 2, 3: 3} {
		if got := economy.WorkplaceSlots("workshop", lvl); got != want {
			t.Fatalf("workshop L%d: %d slots, want %d", lvl, got, want)
		}
	}
}

// The migration's backfill (run here against legacy-shaped rows): a build queued under
// the old rule keeps its deadline -- the oldest row finishes on the very tick the old
// timer promised, a second row that used to run in parallel waits with its remaining
// work intact, and the old fixed-tick timers are retired.
func TestBuildQueueMigrationKeepsInFlightDeadlines(t *testing.T) {
	f := newQueueFixture(t)
	ctx := context.Background()
	raw, err := os.ReadFile("../../db/migrations/245_builders_workshop.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, backfill, ok := strings.Cut(string(raw), "-- Rows queued before this migration")
	if !ok {
		t.Fatal("backfill marker missing from migration 245")
	}
	var a, b uuid.UUID
	for i, due := range []int{1005, 1007} {
		var id uuid.UUID
		if err := f.pool.QueryRow(ctx, `INSERT INTO build_queue(world_id,settlement_id,building_type,complete_at) VALUES($1,$2,$3,now()+interval '1 hour') RETURNING id`, f.world, f.city, []string{"market", "stable"}[i]).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,priority) VALUES($1,'BuildComplete',jsonb_build_object('settlement_id',$2::uuid,'build_queue_id',$3::uuid,'building_type','x'),now(),$4,10)`, f.world, f.city, id, due); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			a = id
		} else {
			b = id
		}
	}
	if _, err := f.pool.Exec(ctx, "--"+backfill); err != nil {
		t.Fatal(err)
	}
	p := f.pending(t)
	if len(p) != 1 || p[a.String()] != 1005 {
		t.Fatalf("the oldest build must still finish on tick 1005: %v", p)
	}
	var rem int
	var calc *int
	if err := f.pool.QueryRow(ctx, `SELECT work_remaining,work_calc_tick FROM build_queue WHERE id=$1`, b).Scan(&rem, &calc); err != nil || rem != 7 || calc != nil {
		t.Fatalf("the second build waits with its 7 days of work intact: rem=%d calc=%v err=%v", rem, calc, err)
	}
}
