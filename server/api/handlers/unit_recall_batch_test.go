package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type batchRecallFixture struct {
	interceptFixture
	pool   *pgxpool.Pool
	router *chi.Mux
	clk    *clock.TestClock
	now    time.Time
	ids    []uuid.UUID
}

func setupBatchRecall(t *testing.T, n int) batchRecallFixture {
	t.Helper()
	f, router := setupInterceptWorld(t, 0, 0)
	pool := unitLoadTestPool(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTestClock(now)
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=0,last_tick_at=$2 WHERE id=$1`, f.worldID, now); err != nil {
		t.Fatal(err)
	}
	path := []province.MapPosition{}
	costs := []float64{}
	for q := 0; q <= 8; q++ {
		path = append(path, province.MapPosition{Q: q, R: 0})
		if q > 0 {
			costs = append(costs, .75)
		}
	}
	route, ok := combat.BuildRoute(path, costs, 0, 6)
	if !ok {
		t.Fatal("invalid route fixture")
	}
	raw, _ := json.Marshal(route)
	var home uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM settlements WHERE world_id=$1 AND owner_id=$2 LIMIT 1`, f.worldID, f.playerID).Scan(&home); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE units SET type='infantry',depart_tick=0,arrive_tick=6,march_route=$2,departs_at=$3,arrives_at=$4,home_settlement_id=$5,support_settlement_id=$5 WHERE id=$1`, f.unitID, raw, now, now.Add(time.Duration(6*tick.TickSeconds)*time.Second), home); err != nil {
		t.Fatal(err)
	}
	ids := []uuid.UUID{f.unitID}
	for i := 1; i < n; i++ {
		var id uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,target_q,target_r,departs_at,arrives_at,depart_tick,arrive_tick,march_route,home_settlement_id,support_settlement_id)
   SELECT world_id,owner_id,type,category,size,status,q,r,target_q,target_r,departs_at,arrives_at,depart_tick,arrive_tick,march_route,home_settlement_id,support_settlement_id FROM units WHERE id=$1 RETURNING id`, f.unitID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	router.Post("/worlds/{worldID}/units/recall", uh.RecallBatch)
	return batchRecallFixture{interceptFixture: f, pool: pool, router: router, clk: clk, now: now, ids: ids}
}
func (f batchRecallFixture) dispatch(t *testing.T, body any) ([]recallBatchResult, []recallBatchGroup) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/worlds/"+f.worldID.String()+"/units/recall", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("batch=%d %s", rr.Code, rr.Body.String())
	}
	var out struct {
		Results []recallBatchResult `json:"results"`
		Groups  []recallBatchGroup  `json:"groups"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	t.Logf("response: %s", rr.Body.String())
	return out.Results, out.Groups
}
func (f batchRecallFixture) deliveries(t *testing.T) []events.ScheduledEvent {
	t.Helper()
	rows, err := f.pool.Query(context.Background(), `SELECT id,payload,due_tick FROM scheduled_events WHERE world_id=$1 AND event_type='OrderDelivery' ORDER BY id`, f.worldID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := []events.ScheduledEvent{}
	for rows.Next() {
		e := events.ScheduledEvent{WorldID: f.worldID}
		if err := rows.Scan(&e.ID, &e.Payload, &e.DueTick); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	return out
}

func TestRecallBatch_GroupHasEveryMemberAndPartialFailures(t *testing.T) {
	f := setupBatchRecall(t, 3)
	ctx := context.Background()
	var homeUnit uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,q,r) VALUES($1,$2,'infantry','land',100,'positioned',0,0) RETURNING id`, f.worldID, f.playerID).Scan(&homeUnit); err != nil {
		t.Fatal(err)
	}
	results, groups := f.dispatch(t, map[string]any{"all": true})
	if len(results) != 4 || len(groups) != 1 || len(groups[0].UnitIDs) != 3 {
		t.Fatalf("results=%+v groups=%+v", results, groups)
	}
	for _, res := range results {
		if res.UnitID == homeUnit {
			if res.Status != "rejected" || res.Reason == "" || res.Name == "" {
				t.Fatalf("silent failure: %+v", res)
			}
		} else if res.Status != "order_dispatched" {
			t.Fatalf("not dispatched: %+v", res)
		}
	}
	deliveries := f.deliveries(t)
	if len(deliveries) != 3 {
		t.Fatalf("events=%d want3", len(deliveries))
	}
	for _, e := range deliveries {
		var p messenger.OrderDeliveryPayload
		_ = json.Unmarshal(e.Payload, &p)
		if p.Verb != messenger.RecallGroupMemberVerb || p.MessengerID != groups[0].MessengerID {
			t.Fatalf("not grouped: %+v", p)
		}
	}
	// The same batch and the old single endpoint both see the same pending Runner.
	repeat, again := f.dispatch(t, map[string]any{"unit_ids": f.ids})
	if len(again) != 0 {
		t.Fatal("duplicate batch dispatched")
	}
	for _, r := range repeat {
		if r.Status != "rejected" || r.Reason == "" {
			t.Fatal("missing double-order rejection")
		}
	}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/worlds/%s/units/%s/recall", f.worldID, f.ids[0]), bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)
	if rr.Code != 409 {
		t.Fatalf("single duplicate=%d %s", rr.Code, rr.Body.String())
	}
}

func TestRecallBatch_AllMembersReachGarrisonAndReplayIsIdempotent(t *testing.T) {
	f := setupBatchRecall(t, 3)
	ctx := context.Background()
	_, groups := f.dispatch(t, map[string]any{"unit_ids": f.ids})
	if len(groups) != 1 {
		t.Fatalf("not grouped: %+v", groups)
	}
	original := f.deliveries(t)
	sched := events.NewScheduler(f.pool, f.clk)
	store := events.NewStore(f.pool)
	delivery := messenger.NewOrderDeliveryHandler(f.pool, sched, store, nil, f.clk)
	arrival := combat.NewUnitArrivalHandler(f.pool, store, nil, sched, f.clk, economy.LoadSitosConfig())
	worker := events.NewWorker(f.pool, f.clk)
	worker.SetPollInterval(5 * time.Millisecond)
	worker.Register(events.ScheduledOrderDelivery, delivery.Handle)
	worker.Register(events.ScheduledUnitArrival, arrival.Handle)
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); worker.Run(runCtx) }()
	defer func() { cancel(); <-done }()
	for gameTick := 1; gameTick <= 12; gameTick++ {
		at := f.now.Add(time.Duration(gameTick*tick.TickSeconds) * time.Second)
		f.clk.Set(at)
		if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=$2,last_tick_at=$3 WHERE id=$1`, f.worldID, gameTick, at); err != nil {
			t.Fatal(err)
		}
		until := time.Now().Add(2 * time.Second)
		for {
			var pending int
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND due_tick<=$2 AND processed_at IS NULL AND failed_at IS NULL`, f.worldID, gameTick).Scan(&pending); err != nil {
				t.Fatal(err)
			}
			if pending == 0 {
				break
			}
			if time.Now().After(until) {
				t.Fatalf("tick%d still %d pending", gameTick, pending)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	var garrison, receipts, arrivals, bad int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM units WHERE id=ANY($1::uuid[]) AND status='garrison'`, f.ids).Scan(&garrison); err != nil {
		t.Fatal(err)
	}
	if garrison != 3 {
		t.Fatalf("garrison=%d want ALL3", garrison)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM processed_deliveries p JOIN scheduled_events se ON se.id=p.event_id WHERE se.world_id=$1 AND se.event_type='OrderDelivery'`, f.worldID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if receipts != 3 {
		t.Fatalf("receipts=%d", receipts)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='UnitArrival'`, f.worldID).Scan(&arrivals); err != nil {
		t.Fatal(err)
	}
	for _, e := range original {
		if err := delivery.Handle(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	var after int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='UnitArrival'`, f.worldID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != arrivals {
		t.Fatalf("replay created arrivals %d→%d", arrivals, after)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND failed_at IS NOT NULL`, f.worldID).Scan(&bad); err != nil {
		t.Fatal(err)
	}
	if bad != 0 {
		t.Fatalf("failed events=%d", bad)
	}
	var status string
	if err := f.pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id=$1`, groups[0].MessengerID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "arrived" {
		t.Fatalf("shared Runner status=%s", status)
	}
}

func TestRecallBatch_OldSharedMessengerStillOnlyDeliversFirst(t *testing.T) {
	f := setupBatchRecall(t, 2)
	ctx := context.Background()
	_, groups := f.dispatch(t, map[string]any{"unit_ids": f.ids})
	if len(groups) != 1 {
		t.Fatal("fixture not grouped")
	}
	e := f.deliveries(t)[0]
	var p messenger.OrderDeliveryPayload
	_ = json.Unmarshal(e.Payload, &p)
	p.Verb = "recall"
	p.RecallGroup = nil
	f.clk.Set(f.now.Add(time.Duration(groups[0].DueTick*tick.TickSeconds) * time.Second))
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=$2,last_tick_at=$3 WHERE id=$1`, f.worldID, groups[0].DueTick, f.clk.Now()); err != nil {
		t.Fatal(err)
	}
	h := messenger.NewOrderDeliveryHandler(f.pool, events.NewScheduler(f.pool, f.clk), events.NewStore(f.pool), nil, f.clk)
	for _, id := range f.ids {
		p.UnitID = id
		p.Recall = &combat.RecallOrder{WorldID: f.worldID, UnitID: id, Mode: "recall"}
		raw, _ := json.Marshal(p)
		if err := h.Handle(ctx, events.ScheduledEvent{Payload: raw}); err != nil {
			t.Fatal(err)
		}
	}
	var first, second int
	if err := f.pool.QueryRow(ctx, `SELECT target_q FROM units WHERE id=$1`, f.ids[0]).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT target_q FROM units WHERE id=$1`, f.ids[1]).Scan(&second); err != nil {
		t.Fatal(err)
	}
	if first != 0 || second != 8 {
		t.Fatalf("old shared claim changed: targets=%d/%d want0/8", first, second)
	}
}

func TestRecallBatch_FiltersAndDifferentPlaces(t *testing.T) {
	f := setupBatchRecall(t, 3)
	ctx := context.Background()
	// Equal destination/time but a different actual road must get a different Runner.
	path := []province.MapPosition{}
	costs := []float64{}
	for q := 0; q <= 8; q++ {
		path = append(path, province.MapPosition{Q: q, R: 0})
		if q > 0 {
			costs = append(costs, .75)
		}
	}
	costs[0] = 3
	for i := 1; i < len(costs); i++ {
		costs[i] = 3.0 / 7
	}
	route, ok := combat.BuildRoute(path, costs, 0, 6)
	if !ok {
		t.Fatal("route")
	}
	raw, _ := json.Marshal(route)
	if _, err := f.pool.Exec(ctx, `UPDATE units SET march_route=$2,march_intent='explore' WHERE id=$1`, f.ids[2], raw); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE units SET march_intent='explore' WHERE id=ANY($1::uuid[])`, f.ids[:2]); err != nil {
		t.Fatal(err)
	}
	results, groups := f.dispatch(t, map[string]any{"all": true, "target_q": 8, "target_r": 0, "intent": "explore"})
	if len(results) != 3 || len(groups) != 2 {
		t.Fatalf("results=%+v groups=%+v", results, groups)
	}
	for _, r := range results {
		if r.Status != "order_dispatched" {
			t.Fatalf("silent loss: %+v", r)
		}
	}
	if groups[0].Q == groups[1].Q && groups[0].R == groups[1].R && groups[0].DueTick == groups[1].DueTick {
		t.Fatal("split same physical meeting")
	}
}

func TestRecallBatch_PartialDeliveryFailureAndDeadLetterCompleteGroup(t *testing.T) {
	f := setupBatchRecall(t, 3)
	ctx := context.Background()
	_, groups := f.dispatch(t, map[string]any{"unit_ids": f.ids})
	deliveries := f.deliveries(t)
	f.clk.Set(f.now.Add(time.Duration(groups[0].DueTick*tick.TickSeconds) * time.Second))
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=$2,last_tick_at=$3 WHERE id=$1`, f.worldID, groups[0].DueTick, f.clk.Now()); err != nil {
		t.Fatal(err)
	}
	var p messenger.OrderDeliveryPayload
	json.Unmarshal(deliveries[0].Payload, &p)
	if _, err := f.pool.Exec(ctx, `UPDATE units SET arrive_tick=99 WHERE id=$1`, p.UnitID); err != nil {
		t.Fatal(err)
	}
	h := messenger.NewOrderDeliveryHandler(f.pool, events.NewScheduler(f.pool, f.clk), events.NewStore(f.pool), nil, f.clk)
	if err := h.Handle(ctx, deliveries[0]); err != nil {
		t.Fatal(err)
	} // named terminal refusal
	if err := h.NotifyDeadLetter(ctx, deliveries[1]); err != nil {
		t.Fatal(err)
	} // terminal system failure, no hub required
	if err := h.Handle(ctx, deliveries[2]); err != nil {
		t.Fatal(err)
	}
	var status string
	var receipts int
	if err := f.pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id=$1`, groups[0].MessengerID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM processed_deliveries WHERE event_id=ANY($1::bigint[])`, []int64{deliveries[0].ID, deliveries[1].ID, deliveries[2].ID}).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if status != "arrived" || receipts != 3 {
		t.Fatalf("terminal group stuck: %s receipts%d", status, receipts)
	}
	var target int
	if err := f.pool.QueryRow(ctx, `SELECT target_q FROM units WHERE id=$1`, p.UnitID).Scan(&target); err != nil {
		t.Fatal(err)
	}
	if target != 8 {
		t.Fatal("changed course incorrectly recalled")
	}
}

func TestRecallBatch_ConcurrentDispatchAndReplay(t *testing.T) {
	f := setupBatchRecall(t, 2)
	ctx := context.Background()
	body, _ := json.Marshal(map[string]any{"unit_ids": f.ids})
	responses := make(chan *httptest.ResponseRecorder, 2)
	for i := 0; i < 2; i++ {
		go func() {
			req := httptest.NewRequest("POST", "/worlds/"+f.worldID.String()+"/units/recall", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+f.accessToken)
			w := httptest.NewRecorder()
			f.router.ServeHTTP(w, req)
			responses <- w
		}()
	}
	accepted := 0
	for i := 0; i < 2; i++ {
		w := <-responses
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		var d struct {
			Results []recallBatchResult `json:"results"`
		}
		json.Unmarshal(w.Body.Bytes(), &d)
		for _, r := range d.Results {
			if r.Status == "order_dispatched" {
				accepted++
			}
		}
	}
	if accepted != 2 {
		t.Fatalf("concurrent dispatch accepted%d want2", accepted)
	}
	es := f.deliveries(t)
	if len(es) != 2 {
		t.Fatalf("deliveries%d", len(es))
	}
	var payload messenger.OrderDeliveryPayload
	json.Unmarshal(es[0].Payload, &payload)
	f.clk.Set(f.now.Add(time.Duration(es[0].DueTick*tick.TickSeconds) * time.Second))
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=$2,last_tick_at=$3 WHERE id=$1`, f.worldID, es[0].DueTick, f.clk.Now()); err != nil {
		t.Fatal(err)
	}
	h := messenger.NewOrderDeliveryHandler(f.pool, events.NewScheduler(f.pool, f.clk), events.NewStore(f.pool), nil, f.clk)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { errs <- h.Handle(ctx, es[0]) }()
	}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	var audits, arrivals int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type=$2`, payload.UnitID, "MarchRecalled").Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='UnitArrival' AND payload->>'unit_id'=$2`, f.worldID, payload.UnitID.String()).Scan(&arrivals); err != nil {
		t.Fatal(err)
	}
	if audits != 1 || arrivals != 1 {
		t.Fatalf("duplicate projections audit%d arrivals%d", audits, arrivals)
	}
}

func TestRecallBatch_AwaitingPassageStillBlocksDuplicates(t *testing.T) {
	f := setupBatchRecall(t, 2)
	ctx := context.Background()
	_, groups := f.dispatch(t, map[string]any{"unit_ids": f.ids})
	if _, err := f.pool.Exec(ctx, `DELETE FROM scheduled_events WHERE world_id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status='awaiting_passage' WHERE id=$1`, groups[0].MessengerID); err != nil {
		t.Fatal(err)
	}
	results, again := f.dispatch(t, map[string]any{"unit_ids": f.ids})
	if len(again) != 0 || len(results) != 2 {
		t.Fatalf("duplicate sea dispatch%+v %+v", results, again)
	}
	for _, r := range results {
		if r.Status != "rejected" || r.Reason == "" {
			t.Fatalf("missing named rejection%+v", r)
		}
	}
}

func TestRecallBatch_ReceiptRollsBackWithProjection(t *testing.T) {
	f := setupBatchRecall(t, 1)
	ctx := context.Background()
	_, groups := f.dispatch(t, map[string]any{"unit_ids": f.ids})
	e := f.deliveries(t)[0]
	var p messenger.OrderDeliveryPayload
	json.Unmarshal(e.Payload, &p)
	f.clk.Set(f.now.Add(time.Duration(groups[0].DueTick*tick.TickSeconds) * time.Second))
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=$2,last_tick_at=$3 WHERE id=$1`, f.worldID, groups[0].DueTick, f.clk.Now()); err != nil {
		t.Fatal(err)
	}
	scheduler := events.NewScheduler(f.pool, f.clk)
	store := events.NewStore(f.pool)
	member := p.RecallGroup.Members[0]
	d := combat.RecallDelivery{EventID: e.ID, DueTick: e.DueTick, OwnerID: p.PlayerID, Meeting: province.MapPosition{Q: p.RecallGroup.Q, R: p.RecallGroup.R}, DepartTick: member.DepartTick, ArriveTick: member.ArriveTick, Route: member.Route}
	// Force the arrival write to fail after changing the unit, then retry.
	fn := "recall_fault_" + fmt.Sprintf("%x", p.UnitID[:])
	if _, err := f.pool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='UnitArrival' AND NEW.payload->>'unit_id'='%s' THEN RAISE EXCEPTION 'injected arrival fault'; END IF; RETURN NEW; END $$`, fn, p.UnitID)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON scheduled_events FOR EACH ROW EXECUTE FUNCTION %s()`, fn, fn)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		f.pool.Exec(ctx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON scheduled_events`, fn))
		f.pool.Exec(ctx, fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, fn))
	})
	if _, err := combat.ExecuteRecallDelivery(ctx, f.pool, scheduler, store, f.clk, *p.Recall, d); err == nil {
		t.Fatal("missing injected arrival failure")
	}
	if _, err := f.pool.Exec(ctx, fmt.Sprintf(`DROP TRIGGER %s ON scheduled_events`, fn)); err != nil {
		t.Fatal(err)
	}

	var receipts, target int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM processed_deliveries WHERE event_id=$1`, e.ID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT target_q FROM units WHERE id=$1`, p.UnitID).Scan(&target); err != nil {
		t.Fatal(err)
	}
	if receipts != 0 || target != 8 {
		t.Fatalf("partial commit receipt%d target%d", receipts, target)
	}
	if _, err := combat.ExecuteRecallDelivery(ctx, f.pool, scheduler, store, f.clk, *p.Recall, d); err != nil {
		t.Fatalf("retry failed%v", err)
	}
}

func TestRecallBatch_LateWorkerUsesDeliveryTickForPositionRouteAndETA(t *testing.T) {
	for _, jitter := range []int{0, 3} {
		t.Run(fmt.Sprint(jitter), func(t *testing.T) {
			f := setupBatchRecall(t, 2)
			ctx := context.Background()
			_, groups := f.dispatch(t, map[string]any{"unit_ids": f.ids})
			es := f.deliveries(t)
			due := groups[0].DueTick
			f.clk.Set(f.now.Add(time.Duration((due+jitter)*tick.TickSeconds) * time.Second))
			if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=$2,last_tick_at=$3 WHERE id=$1`, f.worldID, due+jitter, f.clk.Now()); err != nil {
				t.Fatal(err)
			}
			h := messenger.NewOrderDeliveryHandler(f.pool, events.NewScheduler(f.pool, f.clk), events.NewStore(f.pool), nil, f.clk)
			for _, e := range es {
				if err := h.Handle(ctx, e); err != nil {
					t.Fatal(err)
				}
			}
			for _, id := range f.ids {
				var q, r, target, depart, arrive int
				var at time.Time
				var raw []byte
				if err := f.pool.QueryRow(ctx, `SELECT q,r,target_q,depart_tick,arrive_tick,arrives_at,march_route FROM units WHERE id=$1`, id).Scan(&q, &r, &target, &depart, &arrive, &at, &raw); err != nil {
					t.Fatal(err)
				}
				if q != groups[0].Q || r != groups[0].R || target != 0 || depart != due || arrive != due+2 || !at.Equal(f.now.Add(time.Duration((due+2)*tick.TickSeconds)*time.Second)) {
					t.Fatalf("jitter%d changed meeting/return: q%d r%d target%d depart%d arrive%d at%s", jitter, q, r, target, depart, arrive, at)
				}
				route, ok := combat.LoadActiveRoute(raw, "marching", &depart, &arrive)
				if !ok {
					t.Fatal("missing return route")
				}
				if route.StartTick != due || route.EndTick != due+2 || route.Hexes[0] != ([2]int{groups[0].Q, groups[0].R}) || route.Hexes[len(route.Hexes)-1] != ([2]int{0, 0}) {
					t.Fatalf("return route not anchored to delivery: %+v", route)
				}
			}
		})
	}
}

type batchRecallNotices struct{ failures []map[string]any }

func (*batchRecallNotices) BroadcastEvent(uuid.UUID, string, any) {}
func (n *batchRecallNotices) NotifyPlayer(_ context.Context, _, _ uuid.UUID, kind string, _ int, payload any) error {
	if kind == "OrderFailed" {
		n.failures = append(n.failures, payload.(map[string]any))
	}
	return nil
}
func TestRecallBatch_ChangedStatusRejectsByNameOnce(t *testing.T) {
	f := setupBatchRecall(t, 1)
	ctx := context.Background()
	f.dispatch(t, map[string]any{"unit_ids": f.ids})
	e := f.deliveries(t)[0]
	if _, err := f.pool.Exec(ctx, `UPDATE units SET status='garrison',target_q=NULL,target_r=NULL WHERE id=$1`, f.ids[0]); err != nil {
		t.Fatal(err)
	}
	notices := &batchRecallNotices{}
	h := messenger.NewOrderDeliveryHandler(f.pool, events.NewScheduler(f.pool, f.clk), events.NewStore(f.pool), notices, f.clk)
	for i := 0; i < 2; i++ {
		if err := h.Handle(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if len(notices.failures) != 1 || notices.failures[0]["name"] == "" || notices.failures[0]["reason"] == "" || notices.failures[0]["unit_id"] != f.ids[0] {
		t.Fatalf("missing named idempotent refusal%+v", notices.failures)
	}
}
