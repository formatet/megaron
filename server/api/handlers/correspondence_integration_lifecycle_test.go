package handlers

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
)

// Explicit stocks make real HTTP offers and physical dispatch possible. Arrival,
// acceptance and both trade legs below use production handlers, not fabricated
// accepted/returned JSON transitions.
func integrationTradeFixture(t *testing.T) (*replyFixture, *events.Scheduler, *events.Store) {
	t.Helper()
	f := caseFixture(t)
	clk := clock.NewTestClock(time.Now())
	sched := events.NewScheduler(f.pool, clk)
	store := events.NewStore(f.pool)
	h := NewMessengerHandler(f.pool, sched, clk, nil)
	f.router.Post("/worlds/{worldID}/messengers/{messengerID}/trade-accept", h.TradeAccept)
	f.router.Post("/worlds/{worldID}/messengers/{messengerID}/trade-decline", h.TradeDecline)
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick)
 SELECT s.id,g.key,10000,0,1000000,current_world_tick() FROM settlements s CROSS JOIN goods g
 WHERE s.id=$1 OR s.id=$2 ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick()`, f.originID, f.destID); err != nil {
		t.Fatal(err)
	}
	return f, sched, store
}
func integrationEvent(t *testing.T, f *replyFixture, letter string, kind events.ScheduledEventType) events.ScheduledEvent {
	t.Helper()
	var e events.ScheduledEvent
	if err := f.pool.QueryRow(context.Background(), `SELECT id,world_id,event_type,payload,due_tick FROM scheduled_events
 WHERE world_id=$1 AND event_type IN ($2,$2||'V2',CASE WHEN $2='ShipmentDeliveryV2' THEN 'ShipmentDeliveryV3' ELSE $2 END) AND (payload->>'messenger_id'=$3 OR payload#>>'{then_return,messenger_id}'=$3) ORDER BY id DESC LIMIT 1`, f.worldID, kind, letter).Scan(&e.ID, &e.WorldID, &e.EventType, &e.Payload, &e.DueTick); err != nil {
		t.Fatalf("load %s: %v", kind, err)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE worlds SET current_tick=GREATEST(current_tick,$2) WHERE id=$1`, f.worldID, e.DueTick); err != nil {
		t.Fatal(err)
	}
	return e
}
func integrationSendOffer(t *testing.T, f *replyFixture, store *events.Store, sched *events.Scheduler, kind, subject string) (string, string, map[string]any) {
	t.Helper()
	terms := map[string]any{"kind": "sell", "offer_good": "copper", "offer_qty": float64(2), "want_silver": float64(5), "status": "pending"}
	if kind == "buy" {
		terms = map[string]any{"kind": "buy", "want_good": "copper", "want_qty": float64(2), "offer_silver": float64(5), "status": "pending"}
	}
	code, v := f.post(t, f.senderToken, "/worlds/"+f.worldID.String()+"/settlements/"+f.originID.String()+"/messengers", map[string]any{"destination_id": f.destID.String(), "message": "Actual negotiated terms", "subject": subject, "trade_offer": terms})
	if code != 201 {
		t.Fatalf("send offer: %d %v", code, v)
	}
	letter, id := v["id"].(string), v["case_id"].(string)
	e := integrationEvent(t, f, letter, events.ScheduledMessengerArrival)
	if err := messenger.NewArrivalHandler(f.pool, sched, store, nil).Handle(context.Background(), e); err != nil {
		t.Fatalf("actual messenger delivery: %v", err)
	}
	return letter, id, terms
}
func integrationAccept(t *testing.T, f *replyFixture, letter string) {
	t.Helper()
	code, v := f.post(t, f.recvToken, "/worlds/"+f.worldID.String()+"/messengers/"+letter+"/trade-accept", map[string]any{})
	if code != 200 {
		t.Fatalf("actual trade accept: %d %v", code, v)
	}
}
func integrationArchiveOffer(t *testing.T, f *replyFixture, token, id string) map[string]any {
	t.Helper()
	other := f.senderID
	if token == f.senderToken {
		other = f.receiverID
	}
	code, v := caseGet(t, f, token, "/worlds/"+f.worldID.String()+"/letters/"+other.String())
	if code != 200 {
		t.Fatalf("archive: %d %v", code, v)
	}
	rows := caseItems(t, v)
	if len(rows) != 1 {
		t.Fatalf("expected one case letter: %v", v)
	}
	return rows[0].(map[string]any)["trade_offer"].(map[string]any)
}

func TestCorrespondencePreReturnOfferContainsOnlyOriginalTerms(t *testing.T) {
	for _, kind := range []string{"sell", "buy"} {
		t.Run(kind, func(t *testing.T) {
			f, sched, store := integrationTradeFixture(t)
			letter, id, terms := integrationSendOffer(t, f, store, sched, kind, "Original terms")
			integrationAccept(t, f, letter)
			// A future engine field must not silently widen the sender's pre-return view.
			if _, err := f.pool.Exec(context.Background(), `UPDATE messengers SET trade_offer=trade_offer||'{"future_resolution_detail":{"private":true}}'::jsonb WHERE id=$1`, letter); err != nil {
				t.Fatal(err)
			}
			before := integrationArchiveOffer(t, f, f.senderToken, id)
			if !reflect.DeepEqual(before, terms) {
				t.Fatalf("pre-return archive leaked resolution fields: got=%v original=%v", before, terms)
			}
			recipient := integrationArchiveOffer(t, f, f.recvToken, id)
			for _, key := range []string{"goods_arrives_at", "silver_arrives_at", "goods_arrival_tick", "silver_arrival_tick", "ship_name", "future_resolution_detail"} {
				if _, exists := recipient[key]; !exists {
					t.Fatalf("recipient lost available field %s: %v", key, recipient)
				}
			}
			e := integrationEvent(t, f, letter, events.ScheduledMessengerReturn)
			if err := messenger.NewReturnHandler(f.pool, store, nil).Handle(context.Background(), e); err != nil {
				t.Fatal(err)
			}
			after := integrationArchiveOffer(t, f, f.senderToken, id)
			if !reflect.DeepEqual(after, recipient) {
				t.Fatalf("returned sender did not receive full outcome: %v != %v", after, recipient)
			}
		})
	}
}

func TestCorrespondenceAcceptedMemorySurvivesActualCompletedTrade(t *testing.T) {
	f, sched, store := integrationTradeFixture(t)
	letter, id, _ := integrationSendOffer(t, f, store, sched, "sell", "Completed agreement")
	integrationAccept(t, f, letter)
	e := integrationEvent(t, f, letter, events.ScheduledMessengerReturn)
	if err := messenger.NewReturnHandler(f.pool, store, nil).Handle(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	delivery := integrationEvent(t, f, letter, events.ScheduledTradeDelivery)
	if err := economy.NewDeliveryHandler(f.pool, store, nil, sched).Handle(context.Background(), delivery); err != nil {
		t.Fatalf("actual trade delivery: %v", err)
	}
	returned := integrationEvent(t, f, letter, events.ScheduledTradeReturn)
	if err := economy.NewTradeReturnHandler(f.pool, store, nil).Handle(context.Background(), returned); err != nil {
		t.Fatalf("actual trade return: %v", err)
	}
	var status string
	if err := f.pool.QueryRow(context.Background(), `SELECT trade_offer->>'status' FROM messengers WHERE id=$1`, letter).Scan(&status); err != nil || status != "returned" {
		t.Fatalf("trade did not complete: %s %v", status, err)
	}
	for _, token := range []string{f.senderToken, f.recvToken} {
		offer := integrationArchiveOffer(t, f, token, id)
		if offer["status"] != "returned" {
			t.Fatalf("completed agreement disappeared: %v", offer)
		}
	}
	// The stored trade return event is an actual saved engine outcome payload.
	var p map[string]any
	if err := json.Unmarshal(returned.Payload, &p); err != nil || p["messenger_id"] != letter {
		t.Fatalf("wrong saved return payload: %v %v", p, err)
	}
}
