package handlers

// The Builder's Blessing (bönesystemet v3 steg 5a): what is left of a build takes three
// quarters of the time, once per build; a build begun under the blessing gets the cut from
// its start; the finish event it replaces does nothing.

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

// build queues a harbour with the given days of work in the city and starts it (base pace 1/day).
func (f alphaFixture) build(t *testing.T, city uuid.UUID, work int) uuid.UUID {
	t.Helper()
	var q uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO build_queue(world_id,settlement_id,building_type,complete_at,work_remaining) VALUES($1,$2,'harbour',now(),$3) RETURNING id`, f.world, city, work).Scan(&q); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `SELECT sync_build_queue($1)`, city); err != nil {
		t.Fatal(err)
	}
	return q
}

func (f alphaFixture) setTick(t *testing.T, tick int) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `UPDATE worlds SET current_tick=$2 WHERE id=$1`, f.world, tick); err != nil {
		t.Fatal(err)
	}
}

// due is the tick the pending BuildComplete of this queue row falls on (0 = none).
func (f alphaFixture) due(t *testing.T, q uuid.UUID) int {
	t.Helper()
	var due int
	rows, err := f.pool.Query(context.Background(), `SELECT due_tick FROM scheduled_events WHERE event_type='BuildComplete' AND payload->>'build_queue_id'=$1 AND processed_at IS NULL`, q.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
		if err := rows.Scan(&due); err != nil {
			t.Fatal(err)
		}
	}
	if n > 1 {
		t.Fatalf("%d live BuildComplete events for one build", n)
	}
	return due
}

func (f alphaFixture) buildSpedUp(t *testing.T) []map[string]any {
	t.Helper()
	var raw []byte
	if err := f.pool.QueryRow(context.Background(), `SELECT payload FROM events WHERE world_id=$1 AND event_type='RiteCast' ORDER BY id DESC LIMIT 1`, f.world).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	_ = json.Unmarshal(raw, &p)
	eff, _ := p["effect"].(map[string]any)
	var out []map[string]any
	for _, x := range eff["sped_up"].([]any) {
		out = append(out, x.(map[string]any))
	}
	return out
}

func (f alphaFixture) cityBuilding(city uuid.UUID, scope string) map[string]any {
	return map[string]any{"prayer": religion.PrayerVelchanosBuilding, "target_scope": scope}
}

func TestBuilderBlessing_CutsWhatIsLeftToThreeQuarters(t *testing.T) {
	f := newAlphaFixture(t)
	q := f.build(t, f.cities[0], 12) // from tick 100: due 112
	if got := f.due(t, q); got != 112 {
		t.Fatalf("setup: due %d", got)
	}
	f.setTick(t, 104) // 8 days left
	if r := f.castBody(f.cities[0], f.cityBuilding(f.cities[0], "city")); r.Code != 200 {
		t.Fatalf("cast %d %s", r.Code, r.Body.String())
	}
	if got := f.due(t, q); got != 104+6 { // ceil(0.75*8) = 6 of the 8 left, NOT 0.75 of the whole 12
		t.Fatalf("due %d, want 110", got)
	}
	sped := f.buildSpedUp(t)
	if len(sped) != 1 || sped[0]["kind"] != "build" || sped[0]["id"] != q.String() || sped[0]["old_due_tick"] != float64(112) || sped[0]["new_due_tick"] != float64(110) {
		t.Fatalf("RiteCast.sped_up = %v", sped)
	}
	// the old completion does nothing: it was retired, and a stale copy cannot finish the build
	var stale []byte
	if err := f.pool.QueryRow(context.Background(), `SELECT payload FROM scheduled_events WHERE event_type='BuildComplete' AND payload->>'build_queue_id'=$1 AND processed_at IS NOT NULL ORDER BY id LIMIT 1`, q.String()).Scan(&stale); err != nil {
		t.Fatal(err)
	}
	h := combat.NewBuildCompleteHandler(f.pool, events.NewStore(f.pool), nil)
	if err := h.Handle(context.Background(), events.ScheduledEvent{WorldID: f.world, Payload: stale}); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM build_queue WHERE id=$1`, q).Scan(&n)
	if n != 1 {
		t.Fatal("the superseded BuildComplete finished the build")
	}
}

func TestBuilderBlessing_OnlyOncePerBuild(t *testing.T) {
	f := newAlphaFixture(t)
	q := f.build(t, f.cities[0], 12)
	f.setTick(t, 104)
	if r := f.castBody(f.cities[0], f.cityBuilding(f.cities[0], "city")); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	// a second prayer over the same build (the realm, said from the other city's temple) cuts nothing more
	f.setTick(t, 105)
	if r := f.castBody(f.cities[1], f.cityBuilding(f.cities[1], "realm")); r.Code != 200 {
		t.Fatalf("realm cast %d %s", r.Code, r.Body.String())
	}
	if got := f.due(t, q); got != 110 {
		t.Fatalf("a second blessing moved the finish to %d, want it left at 110", got)
	}
	if sped := f.buildSpedUp(t); len(sped) != 0 {
		t.Fatalf("second cast reported %v", sped)
	}
}

func TestBuilderBlessing_ABuildBegunDuringItIsCutFromItsStartAndNotAfter(t *testing.T) {
	f := newAlphaFixture(t)
	first := f.build(t, f.cities[0], 4)  // due 104
	second := f.build(t, f.cities[0], 8) // waits
	if got := f.due(t, second); got != 0 {
		t.Fatalf("a waiting build has a timer: %d", got)
	}
	if r := f.castBody(f.cities[0], f.cityBuilding(f.cities[0], "city")); r.Code != 200 { // tick 100, until 112
		t.Fatal(r.Body.String())
	}
	if got := f.due(t, second); got != 0 {
		t.Fatalf("a waiting place was cut before it started: %d", got)
	}
	f.setTick(t, 104) // the first is done; the second starts under the blessing
	f.finishBuild(t, first)
	if got := f.due(t, second); got != 104+6 { // 8 days of work cut to 6
		t.Fatalf("second build due %d, want 110", got)
	}
	// a third begun after the blessing has ended is normal
	third := f.build(t, f.cities[0], 8) // waits behind the second
	f.setTick(t, 113)                   // the blessing ended at 112; the second is finished late and the third starts now
	f.finishBuild(t, second)
	if got := f.due(t, third); got != 113+8 {
		t.Fatalf("a build begun after the blessing: due %d, want 121", got)
	}
}

func (f alphaFixture) finishBuild(t *testing.T, q uuid.UUID) {
	t.Helper()
	var payload []byte
	if err := f.pool.QueryRow(context.Background(), `SELECT payload FROM scheduled_events WHERE event_type='BuildComplete' AND payload->>'build_queue_id'=$1 AND processed_at IS NULL`, q.String()).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	h := combat.NewBuildCompleteHandler(f.pool, events.NewStore(f.pool), nil)
	if err := h.Handle(context.Background(), events.ScheduledEvent{WorldID: f.world, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	_, _ = f.pool.Exec(context.Background(), `UPDATE scheduled_events SET processed_at=now() WHERE event_type='BuildComplete' AND payload->>'build_queue_id'=$1 AND processed_at IS NULL`, q.String())
}

func TestBuilderBlessing_ScopesOwnerAndNeverBelowOneDay(t *testing.T) {
	f := newAlphaFixture(t)
	mine := f.build(t, f.cities[0], 12)
	other := f.build(t, f.cities[1], 12)
	// a foreign Wanax's city with a build
	ctx := context.Background()
	var stranger, prov, theirs uuid.UUID
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, uuid.NewString()).Scan(&stranger))
	must(f.pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,7,7,'plains') RETURNING id`, f.world).Scan(&prov))
	must(f.pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type) VALUES($1,$2,'Rival','minoan',$3,'capital') RETURNING id`, f.world, prov, stranger).Scan(&theirs))
	foreign := f.build(t, theirs, 12)
	oneLeft := f.build(t, f.cities[1], 1) // waits behind `other`
	_ = oneLeft
	if r := f.castBody(f.cities[0], f.cityBuilding(f.cities[0], "city")); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if f.due(t, mine) != 109 || f.due(t, other) != 112 || f.due(t, foreign) != 112 {
		t.Fatalf("city scope: mine %d other city %d foreign %d, want 109/112/112", f.due(t, mine), f.due(t, other), f.due(t, foreign))
	}
	f.clearRest(t)
	f.setTick(t, 101)
	if r := f.castBody(f.cities[1], f.cityBuilding(f.cities[1], "realm")); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if f.due(t, other) != 101+9 || f.due(t, foreign) != 112 { // 11 left -> ceil(8.25) = 9; the stranger's build is never touched
		t.Fatalf("realm scope: other city %d foreign %d, want 110/112", f.due(t, other), f.due(t, foreign))
	}
}

func TestBuilderBlessing_CostAndTargets(t *testing.T) {
	for _, c := range []struct {
		scope  string
		factor float64
	}{{"city", 2}, {"realm", 4}} {
		f := newAlphaFixture(t)
		timber, wine := f.stock(t, f.cities[0], "timber"), f.stock(t, f.cities[0], "wine")
		if r := f.castBody(f.cities[0], f.cityBuilding(f.cities[0], c.scope)); r.Code != 200 {
			t.Fatal(r.Body.String())
		}
		if got := timber - f.stock(t, f.cities[0], "timber"); got != 3*c.factor {
			t.Fatalf("%s timber paid %v want %v", c.scope, got, 3*c.factor)
		}
		if got := wine - f.stock(t, f.cities[0], "wine"); got != 2*c.factor {
			t.Fatalf("%s wine paid %v want %v", c.scope, got, 2*c.factor)
		}
		var from, until int
		_ = f.pool.QueryRow(context.Background(), `SELECT from_tick,until_tick FROM prayer_blessings WHERE world_id=$1 AND effect=$2`, f.world, religion.EffectBlessedBuilding).Scan(&from, &until)
		if from != 100 || until != 112 {
			t.Fatalf("row %d..%d want 100..112", from, until)
		}
	}
	f := newAlphaFixture(t)
	for name, body := range map[string]map[string]any{
		"unit step": {"prayer": religion.PrayerVelchanosBuilding, "target_scope": "unit", "target": uuid.NewString()},
		"for ever":  {"prayer": religion.PrayerVelchanosBuilding, "target_scope": "permanent", "target": uuid.NewString()},
		"no scope":  {"prayer": religion.PrayerVelchanosBuilding},
	} {
		if r := f.castBody(f.cities[0], body); r.Code != 400 {
			t.Fatalf("%s: %d %s", name, r.Code, r.Body.String())
		}
	}
	if r := f.castBody(f.cities[0], f.cityBuilding(f.cities[0], "city")); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	f.clearRest(t)
	if r := f.castBody(f.cities[0], f.cityBuilding(f.cities[0], "city")); r.Code != 409 {
		t.Fatalf("the same city twice: %d", r.Code)
	}
}

func TestBuilderBlessing_ShipsUnderConstructionAreCutToo(t *testing.T) {
	f := newAlphaFixture(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := f.pool.Exec(ctx, `UPDATE settlements SET population=500, state='active' WHERE id=$1`, f.cities[0])
	must(err)
	_, err = f.pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'shipyard',1)`, f.cities[0])
	must(err)
	placeShipyardWorkers(t, f.pool, f.cities[0], 1)
	var ship uuid.UUID
	must(f.pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,type,category,size,crew,status,q,r) VALUES($1,$2,$3,$3,'galley','naval',1,20,'forming',0,0) RETURNING id`, f.world, f.player, f.cities[0]).Scan(&ship))
	must(economy.StartShipyardWork(ctx, f.pool, ship, 8))
	shipDue := func() int {
		var due int
		_ = f.pool.QueryRow(ctx, `SELECT due_tick FROM scheduled_events WHERE event_type='ShipyardWorkComplete' AND payload->>'unit_id'=$1 AND processed_at IS NULL`, ship.String()).Scan(&due)
		return due
	}
	if shipDue() != 108 {
		t.Fatalf("setup: ship due %d", shipDue())
	}
	if r := f.castBody(f.cities[0], f.cityBuilding(f.cities[0], "city")); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if shipDue() != 106 {
		t.Fatalf("ship due %d, want 106 (6 of the 8 days)", shipDue())
	}
	sped := f.buildSpedUp(t)
	if len(sped) != 1 || sped[0]["kind"] != "ship" || sped[0]["id"] != ship.String() || sped[0]["new_due_tick"] != float64(106) {
		t.Fatalf("sped_up %v", sped)
	}
	// once only: the yard re-deriving its rate does not cut again
	_, err = f.pool.Exec(ctx, `SELECT sync_shipyard_work($1)`, ship)
	must(err)
	if shipDue() != 106 {
		t.Fatalf("a re-sync cut the ship again: %d", shipDue())
	}
}

// The SQL cut agrees with the Go constant for every work/rate: days left become
// max(1, ceil(BuildTimeFactor*T)), and a cut never adds work.
func TestBuilderBlessing_SQLCutMatchesTheFactor(t *testing.T) {
	f := newAlphaFixture(t)
	for rate := 1; rate <= 3; rate++ {
		for work := 1; work <= 40; work++ {
			var got int
			if err := f.pool.QueryRow(context.Background(), `SELECT blessed_build_work($1,$2)`, work, rate).Scan(&got); err != nil {
				t.Fatal(err)
			}
			days := (work + rate - 1) / rate
			want := max(1, int(math.Ceil(religion.BuildTimeFactor*float64(days)-1e-9)))
			if gotDays := (got + rate - 1) / rate; gotDays != want || got > work || got < 1 {
				t.Fatalf("work %d rate %d: cut to %d (%d days), want %d days", work, rate, got, gotDays, want)
			}
		}
	}
}
