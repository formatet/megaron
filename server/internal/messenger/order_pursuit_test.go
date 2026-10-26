package messenger

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
)

// Wire opt-in is deliberately encoded as JSON so this regression runs against
// the unchanged baseline too: old envelopes must continue to mean the old thing.
func pursuitPayload(t *testing.T, f singleRecallFixture, verb string) []byte {
	t.Helper()
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE units SET status='positioned',q=4,r=0,target_q=NULL,target_r=NULL,departs_at=NULL,arrives_at=NULL,depart_tick=NULL,arrive_tick=NULL,march_route=NULL WHERE id=$1`, f.unitID); err != nil {
		t.Fatal(err)
	}
	p := f.payload
	p.Verb = verb
	p.Recall = nil
	pos := province.MapPosition{Q: 2, R: 0}
	if verb == "march" {
		p.March = &combat.MarchOrder{WorldID: f.world, PlayerID: f.owner, UnitID: f.unitID, TargetQ: 0, TargetR: 0, Delivered: true, ExpectedPosition: &pos}
	}
	if verb == "recall" || verb == "redirect" {
		p.Recall = &combat.RecallOrder{WorldID: f.world, UnitID: f.unitID, Mode: verb}
		p.AfterArrival = &combat.MarchOrder{WorldID: f.world, PlayerID: f.owner, UnitID: f.unitID, TargetQ: 0, TargetR: 0, ExpectedPosition: &pos}
		p.MeetAtDestination = true
		if verb == "redirect" {
			q, r := 0, 0
			p.Recall.NewTargetQ = &q
			p.Recall.NewTargetR = &r
		}
	}
	if verb == "stance" {
		p.Stance = &combat.StanceOrder{WorldID: f.world, PlayerID: f.owner, UnitID: f.unitID, Stance: "storm"}
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := SaveLandRouteTx(ctx, tx, f.world, f.messengerID, province.MapPosition{Q: 0, R: 0}, pos, f.clk.Now().Add(-2*time.Duration(tick.TickSeconds)*time.Second), f.clk.Now()); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT passage_generation FROM messengers WHERE id=$1`, f.messengerID).Scan(&p.PassageGeneration); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(p)
	var body map[string]any
	json.Unmarshal(raw, &body)
	body["following_unit"] = true
	raw, _ = json.Marshal(body)
	if _, err := tx.Exec(ctx, `UPDATE messengers SET order_payload=$2 WHERE id=$1`, f.messengerID, raw); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestOrderPursuit_MovedUnitNeedsPhysicalLeg(t *testing.T) {
	for _, verb := range []string{"march", "recall", "redirect", "stance"} {
		t.Run(verb, func(t *testing.T) {
			f := setupSingleRecall(t, verb)
			ctx := context.Background()
			raw := pursuitPayload(t, f, verb)
			if err := f.handler.Handle(ctx, events.ScheduledEvent{Payload: raw}); err != nil {
				t.Fatal(err)
			}
			var status string
			var arrives time.Time
			var land []byte
			var generation int
			if err := f.pool.QueryRow(ctx, `SELECT status,arrives_at,land_route,passage_generation FROM messengers WHERE id=$1`, f.messengerID).Scan(&status, &arrives, &land, &generation); err != nil {
				t.Fatal(err)
			}
			if status != "outbound" || !arrives.After(f.clk.Now()) {
				t.Fatalf("Runner finished or delivered remotely instead of following: status%s arrives%s", status, arrives)
			}
			var route combat.StoredRoute
			if err := json.Unmarshal(land, &route); err != nil {
				t.Fatal(err)
			}
			if route.Hexes[0] != [2]int{2, 0} || route.Hexes[len(route.Hexes)-1] != [2]int{4, 0} || !route.DepartureAt.Equal(f.clk.Now()) {
				t.Fatalf("new leg teleported or restarted from home: %+v", route)
			}
			var ustatus string
			var stance *string
			if err := f.pool.QueryRow(ctx, `SELECT status,stance FROM units WHERE id=$1`, f.unitID).Scan(&ustatus, &stance); err != nil {
				t.Fatal(err)
			}
			if ustatus != "positioned" || stance != nil {
				t.Fatalf("order applied before physical meeting: %s %v", ustatus, stance)
			}
			if len(f.hub.failures) != 0 {
				t.Fatalf("moved unit was reported failed: %v", f.hub.failures)
			}
			if err := f.handler.Handle(ctx, events.ScheduledEvent{Payload: raw}); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE event_type='OrderDeliveryV2' AND payload->>'messenger_id'=$1`, f.messengerID.String()).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("old-generation replay queued%d legs", count)
			}
			// It moves once more while the Runner walks. The same envelope must continue.
			if _, err := f.pool.Exec(ctx, `UPDATE units SET q=3 WHERE id=$1`, f.unitID); err != nil {
				t.Fatal(err)
			}
			var next []byte
			if err := f.pool.QueryRow(ctx, `SELECT payload FROM scheduled_events WHERE event_type='OrderDeliveryV2' AND payload->>'messenger_id'=$1 ORDER BY (payload->>'passage_generation')::int DESC LIMIT 1`, f.messengerID.String()).Scan(&next); err != nil {
				t.Fatal(err)
			}
			f.clk.Set(arrives)
			if err := f.handler.Handle(ctx, events.ScheduledEvent{Payload: next}); err != nil {
				t.Fatal(err)
			}
			if err := f.pool.QueryRow(ctx, `SELECT status,arrives_at,land_route FROM messengers WHERE id=$1`, f.messengerID).Scan(&status, &arrives, &land); err != nil {
				t.Fatal(err)
			}
			if status != "outbound" {
				t.Fatal("second move ended pursuit")
			}
			json.Unmarshal(land, &route)
			if route.Hexes[0] != [2]int{4, 0} || route.Hexes[len(route.Hexes)-1] != [2]int{3, 0} {
				t.Fatalf("second leg lost physical endpoint: %+v", route)
			}
			if err := f.pool.QueryRow(ctx, `SELECT payload FROM scheduled_events WHERE event_type='OrderDeliveryV2' AND payload->>'messenger_id'=$1 ORDER BY (payload->>'passage_generation')::int DESC LIMIT 1`, f.messengerID.String()).Scan(&next); err != nil {
				t.Fatal(err)
			}
			f.clk.Set(arrives)
			if err := f.handler.Handle(ctx, events.ScheduledEvent{Payload: next}); err != nil {
				t.Fatal(err)
			}
			if err := f.pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id=$1`, f.messengerID).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != "arrived" {
				t.Fatal("physical meeting did not deliver")
			}
			if err := f.pool.QueryRow(ctx, `SELECT status,stance FROM units WHERE id=$1`, f.unitID).Scan(&ustatus, &stance); err != nil {
				t.Fatal(err)
			}
			if verb == "stance" {
				if stance == nil || *stance != "storm" {
					t.Fatal("stance not applied at actual meeting")
				}
			} else if ustatus != "marching" {
				t.Fatalf("course not applied at actual meeting: %s, failures%v", ustatus, f.hub.failures)
			}
		})
	}
}

// The unit is still marching at the physical meeting: delivered course orders
// must redirect under the same transaction checks as a stationary meeting.
func TestOrderPursuit_MovingMeeting(t *testing.T) {
	for _, verb := range []string{"march", "recall", "redirect", "stance"} {
		t.Run(verb, func(t *testing.T) {
			f := setupSingleRecall(t, verb)
			ctx := context.Background()
			raw := pursuitPayload(t, f, verb)
			route := combat.StoredRoute{Version: 2, StartTick: 1, EndTick: 5, DepartureAt: f.clk.Now().Add(-time.Duration(tick.TickSeconds) * time.Second), TickNanoseconds: int64(time.Duration(tick.TickSeconds) * time.Second), Hexes: [][2]int{{1, 0}, {2, 0}, {3, 0}, {4, 0}}, Costs: []int64{1000, 1000, 2000}}
			saved, _ := json.Marshal(route)
			if _, err := f.pool.Exec(ctx, `UPDATE units SET status='marching',q=1,r=0,target_q=4,target_r=0,depart_tick=1,arrive_tick=5,departs_at=$2,arrives_at=$3,march_route=$4 WHERE id=$1`, f.unitID, route.DepartureAt, route.DepartureAt.Add(4*time.Duration(tick.TickSeconds)*time.Second), saved); err != nil {
				t.Fatal(err)
			}
			if err := f.handler.Handle(ctx, events.ScheduledEvent{Payload: raw}); err != nil {
				t.Fatal(err)
			}
			var status string
			var stance *string
			var target int
			if err := f.pool.QueryRow(ctx, `SELECT status,stance,target_q FROM units WHERE id=$1`, f.unitID).Scan(&status, &stance, &target); err != nil {
				t.Fatal(err)
			}
			if len(f.hub.failures) > 0 {
				t.Fatalf("physical moving meeting failed: %v", f.hub.failures)
			}
			if verb == "stance" {
				if stance == nil || *stance != "storm" {
					t.Fatal("moving stance not applied")
				}
			} else if status != "marching" || target != 0 {
				t.Fatalf("moving course not applied: %s target%d", status, target)
			}
		})
	}
}

func TestInterceptCourierRoute_FrozenPhysicalClock(t *testing.T) {
	f := setupSingleRecall(t, "recall")
	ctx := context.Background()
	d := time.Duration(tick.TickSeconds) * time.Second
	// One nominal tick of world-anchor drift. Frozen v2 route is unaffected.
	route := combat.StoredRoute{Version: 2, StartTick: 1, EndTick: 5, DepartureAt: f.clk.Now(), TickNanoseconds: int64(d), OffsetMilli: 500, Hexes: [][2]int{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}}, Costs: []int64{1000, 1000, 1000, 1000}}
	a := tick.Anchor{Tick: 1, At: f.clk.Now().Add(-d)}
	got, ok, err := InterceptCourierTargetRoute(ctx, f.pool, f.world, province.MapPosition{Q: 0, R: 0}, route, a, f.clk.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got != (province.MapPosition{Q: 1, R: 0}) {
		t.Fatalf("intercept used world phase instead of frozen physical route: %v %v", got, ok)
	}
}

func TestOrderPursuit_GuardsAndWaiting(t *testing.T) {
	for _, mode := range []string{"early", "generation", "passage", "disbanded", "owner", "world", "embarked", "unlocated", "noport", "witness"} {
		t.Run(mode, func(t *testing.T) {
			f := setupSingleRecall(t, "stance")
			ctx := context.Background()
			raw := pursuitPayload(t, f, "stance")
			switch mode {
			case "early":
				_, err := f.pool.Exec(ctx, `UPDATE messengers SET arrives_at=$2 WHERE id=$1`, f.messengerID, f.clk.Now().Add(time.Second))
				if err != nil {
					t.Fatal(err)
				}
			case "generation":
				_, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_generation=passage_generation+1 WHERE id=$1`, f.messengerID)
				if err != nil {
					t.Fatal(err)
				}
			case "passage":
				_, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status='awaiting_passage' WHERE id=$1`, f.messengerID)
				if err != nil {
					t.Fatal(err)
				}
			case "disbanded":
				_, err := f.pool.Exec(ctx, `UPDATE units SET status='disbanded' WHERE id=$1`, f.unitID)
				if err != nil {
					t.Fatal(err)
				}
			case "owner":
				_, err := f.pool.Exec(ctx, `WITH p AS (INSERT INTO players(username,password_hash) VALUES(gen_random_uuid()::text,'x') RETURNING id) UPDATE units SET owner_id=p.id FROM p WHERE units.id=$1`, f.unitID)
				if err != nil {
					t.Fatal(err)
				}
			case "world":
				var p OrderDeliveryPayload
				json.Unmarshal(raw, &p)
				p.WorldID = f.owner // mismatched world must never claim this messenger
				raw, _ = json.Marshal(p)
			case "embarked":
				_, err := f.pool.Exec(ctx, `UPDATE units SET status='embarked' WHERE id=$1`, f.unitID)
				if err != nil {
					t.Fatal(err)
				}
			case "witness":
				if _, err := f.pool.Exec(ctx, `INSERT INTO events(world_id,stream_id,stream_type,event_type,payload) VALUES($1,$2,'messenger','CarrierPassengerLandedV1','{}')`, f.world, f.messengerID); err != nil {
					t.Fatal(err)
				}
			case "noport":
				if _, err := f.pool.Exec(ctx, `UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id=$1 AND q=3`, f.world); err != nil {
					t.Fatal(err)
				}
			case "unlocated":
				_, err := f.pool.Exec(ctx, `UPDATE units SET q=NULL,r=NULL WHERE id=$1`, f.unitID)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := f.handler.Handle(ctx, events.ScheduledEvent{Payload: raw}); err != nil {
				t.Fatal(err)
			}
			var status string
			var payload []byte
			var due time.Time
			if err := f.pool.QueryRow(ctx, `SELECT status,order_payload,arrives_at FROM messengers WHERE id=$1`, f.messengerID).Scan(&status, &payload, &due); err != nil {
				t.Fatal(err)
			}
			var p OrderDeliveryPayload
			json.Unmarshal(payload, &p)
			if mode == "disbanded" || mode == "owner" {
				if status != "arrived" || len(f.hub.failures) != 1 {
					t.Fatalf("terminal target not reported: %s %v", status, f.hub.failures)
				}
				return
			}
			if status != "outbound" || len(f.hub.failures) != 0 {
				t.Fatalf("guard claimed/failed: %s %v", status, f.hub.failures)
			}
			if mode != "embarked" && mode != "unlocated" && mode != "noport" {
				var jobs int
				if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE event_type='OrderDeliveryV2' AND payload->>'messenger_id'=$1`, f.messengerID.String()).Scan(&jobs); err != nil {
					t.Fatal(err)
				}
				if jobs != 0 || p.PursuitLegs != 0 || p.FollowWaiting {
					t.Fatalf("guard scheduled or advanced the Runner: jobs%d payload%+v", jobs, p)
				}
				return
			}
			if !p.FollowWaiting || !due.After(f.clk.Now()) {
				t.Fatal("temporary absence did not persist a physical wait")
			}
			// Retry the original generation, then return the unit to the Runner's hex.
			if err := f.handler.Handle(ctx, events.ScheduledEvent{Payload: raw}); err != nil {
				t.Fatal(err)
			}
			if _, err := f.pool.Exec(ctx, `UPDATE units SET status='positioned',q=2,r=0 WHERE id=$1`, f.unitID); err != nil {
				t.Fatal(err)
			}
			var next []byte
			if err := f.pool.QueryRow(ctx, `SELECT payload FROM scheduled_events WHERE event_type='OrderDeliveryV2' AND payload->>'messenger_id'=$1 ORDER BY (payload->>'passage_generation')::int DESC LIMIT 1`, f.messengerID.String()).Scan(&next); err != nil {
				t.Fatal(err)
			}
			f.clk.Set(due)
			if err := f.handler.Handle(ctx, events.ScheduledEvent{Payload: next}); err != nil {
				t.Fatal(err)
			}
			var stance *string
			if err := f.pool.QueryRow(ctx, `SELECT stance FROM units WHERE id=$1`, f.unitID).Scan(&stance); err != nil {
				t.Fatal(err)
			}
			if stance == nil || *stance != "storm" {
				t.Fatalf("waiting did not resume at meeting: %v", f.hub.failures)
			}
		})
	}
}

func TestOrderPursuit_MovingRouteLeg(t *testing.T) {
	for _, version := range []int{0, 2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			f := setupSingleRecall(t, "stance")
			ctx := context.Background()
			raw := pursuitPayload(t, f, "stance")
			d := time.Duration(tick.TickSeconds) * time.Second
			route := combat.StoredRoute{Version: version, StartTick: 1, EndTick: 5, DepartureAt: f.clk.Now(), TickNanoseconds: int64(d), Hexes: [][2]int{{4, 0}, {3, 0}, {2, 0}, {1, 0}, {0, 0}}, Costs: []int64{1000, 1000, 1000, 1000}}
			saved, _ := json.Marshal(route)
			if _, err := f.pool.Exec(ctx, `UPDATE units SET status='marching',q=4,r=0,target_q=0,target_r=0,depart_tick=1,arrive_tick=5,departs_at=$2,arrives_at=$3,march_route=$4 WHERE id=$1`, f.unitID, f.clk.Now(), f.clk.Now().Add(4*d), saved); err != nil {
				t.Fatal(err)
			}
			if err := f.handler.Handle(ctx, events.ScheduledEvent{Payload: raw}); err != nil {
				t.Fatal(err)
			}
			var status string
			var dest int
			var land, next []byte
			var due time.Time
			if err := f.pool.QueryRow(ctx, `SELECT status,dest_q,land_route,arrives_at FROM messengers WHERE id=$1`, f.messengerID).Scan(&status, &dest, &land, &due); err != nil {
				t.Fatal(err)
			}
			wantDest := 3
			if version == 0 {
				wantDest = 2
			}
			if status != "outbound" || dest != wantDest || !due.After(f.clk.Now()) {
				t.Fatalf("bad moving intercept: %s q%d %s", status, dest, due)
			}
			var r combat.StoredRoute
			json.Unmarshal(land, &r)
			if r.Hexes[0] != [2]int{2, 0} {
				t.Fatal("moving leg teleported")
			}
			if err := f.pool.QueryRow(ctx, `SELECT payload FROM scheduled_events WHERE event_type='OrderDeliveryV2' AND payload->>'messenger_id'=$1 ORDER BY (payload->>'passage_generation')::int DESC LIMIT 1`, f.messengerID.String()).Scan(&next); err != nil {
				t.Fatal(err)
			}
			// At the saved deadline both occupy the selected hex (v1 changes on departure).
			f.clk.Set(due)
			n := int(due.Sub(route.DepartureAt) / d)
			if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=$2,last_tick_at=$3 WHERE id=$1`, f.world, 1+n, route.DepartureAt.Add(time.Duration(n)*d)); err != nil {
				t.Fatal(err)
			}
			if err := f.handler.Handle(ctx, events.ScheduledEvent{Payload: next}); err != nil {
				t.Fatal(err)
			}
			var stance *string
			if err := f.pool.QueryRow(ctx, `SELECT stance FROM units WHERE id=$1`, f.unitID).Scan(&stance); err != nil {
				t.Fatal(err)
			}
			for tries := 0; (stance == nil || *stance != "storm") && tries < 8; tries++ {
				if err := f.pool.QueryRow(ctx, `SELECT payload,process_after FROM scheduled_events WHERE event_type='OrderDeliveryV2' AND payload->>'messenger_id'=$1 ORDER BY (payload->>'passage_generation')::int DESC LIMIT 1`, f.messengerID.String()).Scan(&next, &due); err != nil {
					t.Fatal(err)
				}
				f.clk.Set(due)
				n := int(due.Sub(route.DepartureAt) / d)
				if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=$2,last_tick_at=$3 WHERE id=$1`, f.world, 1+n, route.DepartureAt.Add(time.Duration(n)*d)); err != nil {
					t.Fatal(err)
				}
				if err := f.handler.Handle(ctx, events.ScheduledEvent{Payload: next}); err != nil {
					t.Fatal(err)
				}
				if err := f.pool.QueryRow(ctx, `SELECT stance FROM units WHERE id=$1`, f.unitID).Scan(&stance); err != nil {
					t.Fatal(err)
				}
			}
			if stance == nil || *stance != "storm" {
				t.Fatalf("route meeting did not deliver: %v", f.hub.failures)
			}
		})
	}
}
