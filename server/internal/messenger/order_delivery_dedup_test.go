package messenger

import (
	"context"
	"encoding/json"
	"fmt"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"testing"
	"time"
)

type singleRecallFixture struct {
	pool                              *pgxpool.Pool
	clk                               *clock.TestClock
	sched                             *events.Scheduler
	store                             *events.Store
	world, owner, unitID, messengerID uuid.UUID
	payload                           OrderDeliveryPayload
	hub                               *singleRecallNotices
	handler                           *OrderDeliveryHandler
}
type singleRecallNotices struct{ failures []map[string]any }

func (*singleRecallNotices) BroadcastEvent(uuid.UUID, string, any) {}
func (n *singleRecallNotices) NotifyPlayer(_ context.Context, _, _ uuid.UUID, kind string, _ int, p any) error {
	if kind == "OrderFailed" {
		n.failures = append(n.failures, p.(map[string]any))
	}
	return nil
}
func setupSingleRecall(t *testing.T, verb string) singleRecallFixture {
	t.Helper()
	ctx := context.Background()
	pool := testPool(t)
	now := time.Date(2026, 10, 7, 16, 0, 0, 0, time.UTC)
	f := singleRecallFixture{pool: pool, clk: clock.NewTestClock(now), hub: &singleRecallNotices{}}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick,last_tick_at) VALUES($1,'active',1,$2) RETURNING id`, "single-recall-"+uuid.NewString(), now).Scan(&f.world); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM worlds WHERE id=$1`, f.world) })
	if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "single-recall-"+uuid.NewString()).Scan(&f.owner); err != nil {
		t.Fatal(err)
	}
	var prov, home uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,0,0,'plains') RETURNING id`, f.world).Scan(&prov); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital) VALUES($1,$2,'Home','achaean',$3,'capital',true) RETURNING id`, f.world, prov, f.owner).Scan(&home); err != nil {
		t.Fatal(err)
	}
	path := []province.MapPosition{}
	costs := []float64{}
	for q := 0; q <= 4; q++ {
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,0,'plains')`, f.world, q); err != nil {
			t.Fatal(err)
		}
		path = append(path, province.MapPosition{Q: q, R: 0})
		if q > 0 {
			costs = append(costs, .75)
		}
	}
	route, ok := combat.BuildRoute(path, costs, 0, 3)
	if !ok {
		t.Fatal("route fixture")
	}
	raw, _ := json.Marshal(route)
	if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,target_q,target_r,depart_tick,arrive_tick,departs_at,arrives_at,march_route,home_settlement_id,support_settlement_id,name)
 VALUES($1,$2,'infantry','land',10,'marching',0,0,4,0,0,3,$3,$4,$5,$6,$6,'Delta') RETURNING id`, f.world, f.owner, now.Add(-time.Duration(tick.TickSeconds)*time.Second), now.Add(time.Duration(2*tick.TickSeconds)*time.Second), raw, home).Scan(&f.unitID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,message_text,status,kind,hex_q,hex_r,dest_q,dest_r,arrives_at) VALUES($1,$2,$3,'Runner','outbound','order',0,0,2,0,$4) RETURNING id`, f.world, f.owner, home, now).Scan(&f.messengerID); err != nil {
		t.Fatal(err)
	}
	order := &combat.RecallOrder{WorldID: f.world, UnitID: f.unitID, Mode: verb}
	if verb == "redirect" {
		q, r := 0, 0
		order.NewTargetQ = &q
		order.NewTargetR = &r
	}
	f.payload = OrderDeliveryPayload{WorldID: f.world, PlayerID: f.owner, UnitID: f.unitID, MessengerID: f.messengerID, Verb: verb, Recall: order}
	f.sched = events.NewScheduler(pool, f.clk)
	f.store = events.NewStore(pool)
	f.handler = NewOrderDeliveryHandler(pool, f.sched, f.store, f.hub, f.clk)
	at := 3
	if err := f.sched.EnqueueTick(ctx, f.world, events.ScheduledUnitArrival, unit.ScheduledUnitArrivalPayload{UnitID: f.unitID, WorldID: f.world, ArriveTick: &at}, 3); err != nil {
		t.Fatal(err)
	}
	return f
}
func (f singleRecallFixture) event() events.ScheduledEvent {
	raw, _ := json.Marshal(f.payload)
	return events.ScheduledEvent{WorldID: f.world, Payload: raw, DueTick: 1}
}
func (f singleRecallFixture) assertApplied(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	var target, arrive, count, audits int
	if err := f.pool.QueryRow(ctx, `SELECT target_q,arrive_tick FROM units WHERE id=$1`, f.unitID).Scan(&target, &arrive); err != nil {
		t.Fatal(err)
	}
	if target != 0 || arrive != 3 {
		t.Fatalf("order dropped without jitter: target%d arrive%d want0/3", target, arrive)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='UnitArrival' AND due_tick=3`, f.world).Scan(&count); err != nil {
		t.Fatal(err)
	}
	kind := unit.EventUnitMarchRecalled
	if f.payload.Verb == "redirect" {
		kind = unit.EventUnitMarchRedirected
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type=$2`, f.unitID, kind).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if count != 1 || audits != 1 {
		t.Fatalf("arrivals%d audits%d want1/1", count, audits)
	}
}
func TestSingleRecall_IdenticalActiveArrival(t *testing.T) {
	for _, verb := range []string{"recall", "redirect"} {
		t.Run(verb, func(t *testing.T) {
			f := setupSingleRecall(t, verb)
			res, err := combat.ExecuteRecall(context.Background(), f.pool, f.sched, f.store, f.clk, *f.payload.Recall)
			if err != nil {
				t.Fatalf("identical arrival failed with NO jitter: %v", err)
			}
			if res == nil {
				t.Fatal("missing applied result")
			}
			f.assertApplied(t)
		})
	}
}
func TestSingleRecall_DeliveryReusesArrival(t *testing.T) {
	for _, verb := range []string{"recall", "redirect"} {
		t.Run(verb, func(t *testing.T) {
			f := setupSingleRecall(t, verb)
			if err := f.handler.Handle(context.Background(), f.event()); err != nil {
				t.Fatal(err)
			}
			f.assertApplied(t)
		})
	}
}
func TestSingleRecall_FailureAfterClaimIsNamedAndAuditedOnce(t *testing.T) {
	for _, verb := range []string{"recall", "redirect"} {
		t.Run(verb, func(t *testing.T) {
			f := setupSingleRecall(t, verb)
			ctx := context.Background()
			// Force a different DB failure in the course mutation; arrival dedup is irrelevant.
			fn := "single_fault_" + strings.ReplaceAll(f.unitID.String(), "-", "")
			sql := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.id='%s' THEN RAISE EXCEPTION 'injected unit fault'; END IF; RETURN NEW; END $$`, fn, f.unitID)
			if _, err := f.pool.Exec(ctx, sql); err != nil {
				t.Fatal(err)
			}
			if _, err := f.pool.Exec(ctx, fmt.Sprintf(`CREATE TRIGGER %s BEFORE UPDATE ON units FOR EACH ROW EXECUTE FUNCTION %s()`, fn, fn)); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				f.pool.Exec(ctx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON units`, fn))
				f.pool.Exec(ctx, fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, fn))
			})
			for i := 0; i < 2; i++ {
				if err := f.handler.Handle(ctx, f.event()); err != nil {
					t.Fatal(err)
				}
			}
			if len(f.hub.failures) != 1 || f.hub.failures[0]["name"] == "" || f.hub.failures[0]["reason"] == "" {
				t.Fatalf("silent failed order after claim: %+v", f.hub.failures)
			}
			var audits int
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE event_type='OrderDeliveryFailed' AND payload->>'messenger_id'=$1`, f.messengerID.String()).Scan(&audits); err != nil {
				t.Fatal(err)
			}
			if audits != 1 {
				t.Fatalf("failure audit%d want1", audits)
			}
			var rawAudit []byte
			if err := f.pool.QueryRow(ctx, `SELECT payload FROM events WHERE event_type='OrderDeliveryFailed' AND payload->>'messenger_id'=$1`, f.messengerID.String()).Scan(&rawAudit); err != nil {
				t.Fatal(err)
			}
			var audit map[string]any
			json.Unmarshal(rawAudit, &audit)
			if audit["name"] != f.hub.failures[0]["name"] || audit["reason"] != f.hub.failures[0]["reason"] || audit["verb"] != verb || !strings.Contains(audit["reason"].(string), "reissue") {
				t.Fatalf("audit/notice mismatch %+v", audit)
			}
			var target int
			if err := f.pool.QueryRow(ctx, `SELECT target_q FROM units WHERE id=$1`, f.unitID).Scan(&target); err != nil {
				t.Fatal(err)
			}
			if target != 4 {
				t.Fatal("failed course partially committed")
			}

		})
	}
}

func TestSingleRecall_OldSharedMessengerOnlyTurnsFirst(t *testing.T) {
	f := setupSingleRecall(t, "recall")
	ctx := context.Background()
	var second uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,target_q,target_r,depart_tick,arrive_tick,departs_at,arrives_at,march_route,home_settlement_id,support_settlement_id,name)
 SELECT world_id,owner_id,type,category,size,status,q,r,target_q,target_r,depart_tick,arrive_tick,departs_at,arrives_at,march_route,home_settlement_id,support_settlement_id,'Gamma' FROM units WHERE id=$1 RETURNING id`, f.unitID).Scan(&second); err != nil {
		t.Fatal(err)
	}
	if err := f.handler.Handle(ctx, f.event()); err != nil {
		t.Fatal(err)
	}
	f.assertApplied(t)
	p := f.payload
	p.UnitID = second
	order := *p.Recall
	order.UnitID = second
	p.Recall = &order
	raw, _ := json.Marshal(p)
	if err := f.handler.Handle(ctx, events.ScheduledEvent{WorldID: f.world, Payload: raw, DueTick: 1}); err != nil {
		t.Fatal(err)
	}
	var target int
	if err := f.pool.QueryRow(ctx, `SELECT target_q FROM units WHERE id=$1`, second).Scan(&target); err != nil {
		t.Fatal(err)
	}
	if target != 4 {
		t.Fatal("changed frozen shared-messenger semantics")
	}
}
func TestSingleRecall_PassageRebuiltEnvelopeUsesSameFix(t *testing.T) {
	for _, verb := range []string{"recall", "redirect"} {
		t.Run(verb, func(t *testing.T) {
			f := setupSingleRecall(t, verb)
			ctx := context.Background()
			raw, _ := json.Marshal(f.payload)
			if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status='aboard',order_payload=$2 WHERE id=$1`, f.messengerID, raw); err != nil {
				t.Fatal(err)
			}
			tx, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if err := scheduleCompletion(ctx, tx, f.sched, f.messengerID, 1, f.clk.Now()); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			var e events.ScheduledEvent
			if err := f.pool.QueryRow(ctx, `SELECT id,payload,due_tick FROM scheduled_events WHERE world_id=$1 AND event_type='OrderDelivery'`, f.world).Scan(&e.ID, &e.Payload, &e.DueTick); err != nil {
				t.Fatal(err)
			}
			e.WorldID = f.world
			var p OrderDeliveryPayload
			json.Unmarshal(e.Payload, &p)
			if p.PassageGeneration != 1 {
				t.Fatalf("generation%d", p.PassageGeneration)
			}
			if err := f.handler.Handle(ctx, e); err != nil {
				t.Fatal(err)
			}
			f.assertApplied(t)
		})
	}
}
func TestSingleRecall_DifferentEnvelopeIsPreserved(t *testing.T) {
	f := setupSingleRecall(t, "recall")
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `DELETE FROM scheduled_events WHERE world_id=$1`, f.world); err != nil {
		t.Fatal(err)
	}
	other := 7
	if err := f.sched.EnqueueTick(ctx, f.world, events.ScheduledUnitArrival, unit.ScheduledUnitArrivalPayload{UnitID: f.unitID, WorldID: f.world, ArriveTick: &other}, 7); err != nil {
		t.Fatal(err)
	}
	if err := f.handler.Handle(ctx, f.event()); err != nil {
		t.Fatal(err)
	}
	f.assertApplied(t)
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND due_tick=7 AND processed_at IS NULL`, f.world).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("different arrival was cancelled")
	}
}

func TestSingleRecall_IncompleteOrderIsNamedAndAudited(t *testing.T) {
	f := setupSingleRecall(t, "recall")
	f.payload.Recall = nil
	if err := f.handler.Handle(context.Background(), f.event()); err != nil {
		t.Fatal(err)
	}
	if len(f.hub.failures) != 1 || f.hub.failures[0]["name"] == "" {
		t.Fatalf("missing incomplete-order outcome %+v", f.hub.failures)
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM events WHERE event_type='OrderDeliveryFailed' AND payload->>'messenger_id'=$1`, f.messengerID.String()).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("missing audit")
	}
}

// Preserve the old tick-vs-wall-hours regression on the active order envelope.
// At six seconds per tick the saved-route fixture turns at q2 and travels two
// ticks home. A wall-hour ETA would make the map crawl and then snap on arrival.
func TestSingleRecall_ArrivesAtMatchesTickSchedule(t *testing.T) {
	orig := tick.TickSeconds
	tick.TickSeconds = 6
	t.Cleanup(func() { tick.TickSeconds = orig })
	for _, verb := range []string{"recall", "redirect"} {
		t.Run(verb, func(t *testing.T) {
			f := setupSingleRecall(t, verb)
			if err := f.handler.Handle(context.Background(), f.event()); err != nil {
				t.Fatal(err)
			}
			f.assertApplied(t)
			var arrivesAt time.Time
			if err := f.pool.QueryRow(context.Background(), `SELECT arrives_at FROM units WHERE id=$1`, f.unitID).Scan(&arrivesAt); err != nil {
				t.Fatal(err)
			}
			want := f.clk.Now().Add(12 * time.Second)
			if !arrivesAt.Equal(want) {
				t.Fatalf("wall-hour ETA: got %v want %v (two real game ticks)", arrivesAt, want)
			}
		})
	}
}
