package messenger

import (
	"context"
	"encoding/json"
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
