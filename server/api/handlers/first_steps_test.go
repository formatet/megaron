package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type firstStepsFixture struct {
	t     *testing.T
	pool  *pgxpool.Pool
	world uuid.UUID
	svc   *auth.Service
}

func newFirstStepsFixture(t *testing.T) *firstStepsFixture {
	t.Helper()
	pool := settlementsOverviewTestPool(t)
	ctx := context.Background()
	return &firstStepsFixture{t, pool, settlementsOverviewTestWorld(t, pool, ctx), auth.NewService(pool, "test-secret")}
}

// wanax registers a player who has joined the world (a record, no city yet).
func (f *firstStepsFixture) wanax() uuid.UUID {
	f.t.Helper()
	id, _ := settlementsOverviewRegisterPlayer(f.t, f.svc, context.Background(), "fs-"+uuid.New().String())
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO player_world_records (player_id, world_id, status) VALUES ($1,$2,'active')`, id, f.world); err != nil {
		f.t.Fatal(err)
	}
	return id
}

// found gives the player a capital (pop 800 = 8 gubbar) and stamps founded_tick.
func (f *firstStepsFixture) found(p uuid.UUID, q int) uuid.UUID {
	f.t.Helper()
	city := settlementsOverviewSeedSettlement(f.t, f.pool, context.Background(), f.world, p, "Petras", true, q, 0)
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE player_world_records SET founded_tick = 1000 WHERE player_id=$1 AND world_id=$2`, p, f.world); err != nil {
		f.t.Fatal(err)
	}
	return city
}

func (f *firstStepsFixture) exec(q string, a ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(context.Background(), q, a...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *firstStepsFixture) place(city uuid.UUID, n int) {
	for i := 1; i <= n; i++ {
		f.exec(`INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES($1,$2,'building','shipyard','shipbuilding')`, city, i)
	}
}

func (f *firstStepsFixture) do(p uuid.UUID, method, path string) (int, firstStepsOut) {
	f.t.Helper()
	h := NewFirstStepsHandler(f.pool)
	r := chi.NewRouter()
	r.Get("/worlds/{worldID}/first-steps", asPlayer(p, h.Get))
	r.Post("/worlds/{worldID}/first-steps/hide", asPlayer(p, h.Hide))
	r.Post("/worlds/{worldID}/first-steps/show", asPlayer(p, h.Show))
	req := httptest.NewRequest(method, "/worlds/"+f.world.String()+"/first-steps"+path, strings.NewReader(""))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var out firstStepsOut
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (f *firstStepsFixture) get(p uuid.UUID) firstStepsOut {
	f.t.Helper()
	code, out := f.do(p, http.MethodGet, "")
	if code != http.StatusOK {
		f.t.Fatalf("GET first-steps: %d", code)
	}
	return out
}

func stepDone(o firstStepsOut, key string) bool {
	for _, s := range o.Steps {
		if s.Key == key {
			return s.Done
		}
	}
	return false
}

func TestFirstSteps_BeforeFoundingOnlyFoundIsOpen(t *testing.T) {
	f := newFirstStepsFixture(t)
	p := f.wanax()
	o := f.get(p)
	if o.Total != 4 || o.DoneCount != 0 || o.Next != "found" || len(o.Steps) != 4 {
		t.Fatalf("unfounded: %+v", o)
	}
	if stepDone(o, "work") || stepDone(o, "build") || stepDone(o, "explore") {
		t.Fatalf("nothing but found can be done before founding: %+v", o.Steps)
	}
	if strings.Contains(o.Steps[1].Text, "in .") || !strings.Contains(o.Steps[1].Text, "your city") {
		t.Fatalf("no empty city name before founding: %q", o.Steps[1].Text)
	}
}

func TestFirstSteps_WorkIsDoneWhenNoIdleGubbarRemain(t *testing.T) {
	f := newFirstStepsFixture(t)
	p := f.wanax()
	city := f.found(p, 0)
	o := f.get(p)
	if !stepDone(o, "found") || stepDone(o, "work") || o.Next != "work" {
		t.Fatalf("8 idle gubbar: work open, got %+v", o)
	}
	if !strings.Contains(o.Steps[1].Text, "Petras") {
		t.Fatalf("work text names the city: %q", o.Steps[1].Text)
	}
	f.place(city, 7)
	if o = f.get(p); stepDone(o, "work") {
		t.Fatal("one gubbe still idle: work must stay open")
	}
}

func TestFirstSteps_WorkBoundaryAtExactlyZero(t *testing.T) {
	f := newFirstStepsFixture(t)
	p := f.wanax()
	city := f.found(p, 0)
	f.place(city, 8) // pop 800 → exactly 8 gubbar, 0 idle
	if o := f.get(p); !stepDone(o, "work") {
		t.Fatalf("0 idle: work done, got %+v", o)
	}
}

func TestFirstSteps_BuildNeedsAnOrderNotTheStartFarm(t *testing.T) {
	f := newFirstStepsFixture(t)
	p := f.wanax()
	city := f.found(p, 0)
	f.exec(`INSERT INTO buildings (settlement_id, building_type, level, hex_q, hex_r) VALUES ($1,'farm',1,0,0)`, city)
	if o := f.get(p); stepDone(o, "build") {
		t.Fatalf("the free start farm must not count: %+v", o)
	}
	// An order: a running BuildComplete job, still unprocessed.
	f.exec(`INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,priority)
	        VALUES($1,'BuildComplete',jsonb_build_object('settlement_id',$2::uuid,'build_queue_id',$3::uuid,'building_type','granary'),now(),1010,10)`,
		f.world, city, uuid.New())
	o := f.get(p)
	if !stepDone(o, "build") {
		t.Fatalf("a build order completes the step: %+v", o)
	}
	var b firstStep
	for _, s := range o.Steps {
		if s.Key == "build" {
			b = s
		}
	}
	if b.ReadyTick == nil || *b.ReadyTick != 1010 || !strings.Contains(b.Text, FirstStepsWhenToken) {
		t.Fatalf("building in progress carries ready_tick and a {when} line: %+v", b)
	}
	// Complete everything else: the wait line is what the card shows.
	f.place(city, 8)
	f.exec(`INSERT INTO units(world_id,owner_id,type,category) VALUES($1,$2,'infantry','land')`, f.world, p)
	f.exec(`INSERT INTO events(stream_id,stream_type,event_type,payload,world_id,world_tick) SELECT id,'unit','UnitMarchOrdered','{}',$1,1005 FROM units WHERE owner_id=$2`, f.world, p)
	o = f.get(p)
	if o.DoneCount != 4 || o.Next != "build" {
		t.Fatalf("all done but building still going up: show the wait line, got next=%q done=%d", o.Next, o.DoneCount)
	}
	// Another Wanax finishing a building must not close my wait line.
	rival := f.wanax()
	rcity := f.found(rival, 5)
	f.exec(`INSERT INTO events(stream_id,stream_type,event_type,payload,world_id) VALUES($1,'province','BuildComplete','{}',$2)`, rcity, f.world)
	if o = f.get(p); o.Next != "build" {
		t.Fatalf("a rival's completed building must not count as mine: next=%q", o.Next)
	}
	// Completed: closing line.
	f.exec(`INSERT INTO events(stream_id,stream_type,event_type,payload,world_id) VALUES($1,'province','BuildComplete','{}',$2)`, city, f.world)
	f.exec(`UPDATE scheduled_events SET processed_at=now() WHERE event_type='BuildComplete' AND world_id=$1`, f.world)
	o = f.get(p)
	if o.DoneCount != 4 || o.Next != "" || o.ClosingText == "" {
		t.Fatalf("finished: closing, got next=%q %+v", o.Next, o)
	}
	for _, s := range o.Steps {
		if s.ReadyTick != nil {
			t.Fatalf("no ready_tick once built: %+v", s)
		}
	}
}

func TestFirstSteps_QueuedBuildCountsAsOrdered(t *testing.T) {
	f := newFirstStepsFixture(t)
	p := f.wanax()
	city := f.found(p, 0)
	f.exec(`INSERT INTO build_queue (settlement_id, world_id, building_type, complete_at, work_remaining) VALUES ($1,$2,'granary', now() + interval '1 hour', 10)`, city, f.world)
	if o := f.get(p); !stepDone(o, "build") {
		t.Fatalf("a queued order counts: %+v", o)
	}
}

func TestFirstSteps_ExploreIsOwnMarchAfterFoundingOnly(t *testing.T) {
	f := newFirstStepsFixture(t)
	p, rival := f.wanax(), f.wanax()
	f.found(p, 0)
	f.found(rival, 1)
	mine := uuid.New()
	theirs := uuid.New()
	f.exec(`INSERT INTO units(id,world_id,owner_id,type,category) VALUES($1,$3,$4,'infantry','land'),($2,$3,$5,'infantry','land')`, mine, theirs, f.world, p, rival)
	march := func(unit uuid.UUID, tick int) {
		f.exec(`INSERT INTO events(stream_id,stream_type,event_type,payload,world_id,world_tick) VALUES($1,'unit','UnitMarchOrdered','{}',$2,$3)`, unit, f.world, tick)
	}
	march(theirs, 1005)
	if o := f.get(p); stepDone(o, "explore") {
		t.Fatal("another Wanax's march must not complete my explore")
	}
	march(mine, 999) // before founding_tick 1000: the Host's founding march
	if o := f.get(p); stepDone(o, "explore") {
		t.Fatal("a march before founding must not count")
	}
	march(mine, 1005)
	if o := f.get(p); !stepDone(o, "explore") {
		t.Fatalf("my march after founding completes explore: %+v", o)
	}
	if o := f.get(rival); !stepDone(o, "explore") {
		t.Fatal("and the rival's own march completes theirs")
	}
}

func TestFirstSteps_ExploreCountsAMarchOfAUnitSinceGone(t *testing.T) {
	f := newFirstStepsFixture(t)
	p := f.wanax()
	f.found(p, 0)
	gone := uuid.New() // a colonising unit is disbanded on arrival — only UnitFormed remains
	f.exec(`INSERT INTO events(stream_id,stream_type,event_type,payload,world_id) VALUES($1,'unit','UnitFormed',jsonb_build_object('owner_id',$2::text),$3)`, gone, p, f.world)
	f.exec(`INSERT INTO events(stream_id,stream_type,event_type,payload,world_id,world_tick) VALUES($1,'unit','UnitMarchOrdered','{}',$2,1005)`, gone, f.world)
	if o := f.get(p); !stepDone(o, "explore") {
		t.Fatalf("owner via UnitFormed: %+v", o)
	}
}

func TestFirstSteps_HideShowRoundTripAndIsolation(t *testing.T) {
	f := newFirstStepsFixture(t)
	a, b := f.wanax(), f.wanax()
	f.found(a, 0)
	if o := f.get(a); o.Hidden {
		t.Fatal("starts visible")
	}
	if code, _ := f.do(a, http.MethodPost, "/hide"); code != http.StatusOK {
		t.Fatalf("hide: %d", code)
	}
	if !f.get(a).Hidden {
		t.Fatal("hidden after hide")
	}
	if f.get(b).Hidden {
		t.Fatal("A hiding must not hide B's card")
	}
	if code, _ := f.do(a, http.MethodPost, "/show"); code != http.StatusOK {
		t.Fatalf("show: %d", code)
	}
	if f.get(a).Hidden {
		t.Fatal("visible after show")
	}
	// B reads only B: A's city does not count for B.
	if ob := f.get(b); stepDone(ob, "found") {
		t.Fatalf("B must not see A's founding: %+v", ob)
	}
	// Not in this world: nothing to flip.
	stranger, _ := settlementsOverviewRegisterPlayer(t, f.svc, context.Background(), "fs-x-"+uuid.New().String())
	if code, _ := f.do(stranger, http.MethodPost, "/hide"); code != http.StatusNotFound {
		t.Fatalf("hide without a record: want 404, got %d", code)
	}
}

func TestFirstSteps_AnAbandonedCityIsNotAHome(t *testing.T) {
	f := newFirstStepsFixture(t)
	p := f.wanax()
	city := f.found(p, 0)
	f.exec(`UPDATE settlements SET state='abandoned' WHERE id=$1`, city)
	if o := f.get(p); stepDone(o, "found") || o.Next != "found" {
		t.Fatalf("only an active settlement counts as founded: %+v", o)
	}
}

// Timothy: the advice covers only the first steps and ends when they are done.
// Growth leaves a free citizen later (kharis applyDecay); that must not bring
// the card back, and a lost city must not either.
func TestFirstSteps_CompletionIsLatched(t *testing.T) {
	f := newFirstStepsFixture(t)
	p := f.wanax()
	city := f.found(p, 0)
	f.place(city, 8)
	f.exec(`INSERT INTO events(stream_id,stream_type,event_type,payload,world_id) VALUES($1,'province','BuildComplete','{}',$2)`, city, f.world)
	f.exec(`INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,priority,processed_at)
	        VALUES($1,'BuildComplete',jsonb_build_object('settlement_id',$2::uuid,'build_queue_id',$3::uuid,'building_type','granary'),now(),1010,10,now())`, f.world, city, uuid.New())
	unit := uuid.New()
	f.exec(`INSERT INTO units(id,world_id,owner_id,type,category) VALUES($1,$2,$3,'infantry','land')`, unit, f.world, p)
	f.exec(`INSERT INTO events(stream_id,stream_type,event_type,payload,world_id,world_tick) VALUES($1,'unit','UnitMarchOrdered','{}',$2,1005)`, unit, f.world)
	if o := f.get(p); o.Next != "" || o.DoneCount != 4 {
		t.Fatalf("all four done: closing, got %+v", o)
	}
	f.exec(`UPDATE settlements SET population = population + 100 WHERE id=$1`, city) // a free citizen appears
	o := f.get(p)
	if o.Next != "" || o.DoneCount != 4 || !stepDone(o, "work") {
		t.Fatalf("growth must not bring the advice back: %+v", o)
	}
	f.exec(`UPDATE settlements SET state='abandoned' WHERE id=$1`, city) // city lost
	if o = f.get(p); o.Next != "" || !stepDone(o, "found") {
		t.Fatalf("a lost city must not bring the advice back: %+v", o)
	}
}

// Before completion a step that flips back stays useful.
func TestFirstSteps_StepsMayFlipBackBeforeCompletion(t *testing.T) {
	f := newFirstStepsFixture(t)
	p := f.wanax()
	city := f.found(p, 0)
	f.place(city, 8)
	if !stepDone(f.get(p), "work") {
		t.Fatal("work done with 8 placed")
	}
	f.exec(`UPDATE settlements SET population = population + 100 WHERE id=$1`, city)
	if o := f.get(p); stepDone(o, "work") || o.Next != "work" {
		t.Fatalf("not yet complete: a new free citizen reopens work, got %+v", o)
	}
}

// A completed player costs no step queries: the latched answer is returned from
// the record alone (proved by deleting everything the rules would read).
func TestFirstSteps_CompletedPlayerReadsNoState(t *testing.T) {
	f := newFirstStepsFixture(t)
	p := f.wanax()
	f.exec(`UPDATE player_world_records SET first_steps_completed = true WHERE player_id=$1 AND world_id=$2`, p, f.world)
	if o := f.get(p); o.Next != "" || o.DoneCount != 4 || len(o.Steps) != 4 {
		t.Fatalf("completed: %+v", o)
	}
}
