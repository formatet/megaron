package economy

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
)

// A saved return survives removed terrain, and its credit, mover, manifest,
// timer, outcome and notice are one transaction even when the queue fails.
func TestDeliverySavedReturnAtomicAndFrozen(t *testing.T) {
	for _, category := range []string{"land", "naval"} {
		t.Run(category, func(t *testing.T) {
			pool := testPool(t)
			ctx := context.Background()
			f := newNavalPlayerTradeFixture(t, pool)
			leg := f.leg1Transport(t)
			if category == "land" {
				if _, err := pool.Exec(ctx, `UPDATE transports SET category='land',ship_unit_id=NULL WHERE id=$1`, leg); err != nil {
					t.Fatal(err)
				}
			}
			g := province.TileGraph{}
			for q := 0; q <= 5; q++ {
				g[[2]int{q, 0}] = "plains"
				if category == "naval" {
					g[[2]int{q, 1}] = "coastal_sea"
				}
			}
			journey, err := g.PlanTradeJourney(province.MapPosition{Q: 5}, province.MapPosition{}, category)
			if err != nil {
				t.Fatal(err)
			}
			// The database deliberately has no map tiles: saved payload cannot reroute.
			if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=17 WHERE id=$1`, f.worldID); err != nil {
				t.Fatal(err)
			}
			clk := clock.NewTestClock(time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC))
			h := NewDeliveryHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clk))
			raw, err := json.Marshal(map[string]any{"transport_id": leg, "destination_id": f.counterCity, "good_key": "silver", "quantity": 100, "then_return": map[string]any{"messenger_id": uuid.New().String(), "destination_id": f.initiatorCity, "owner_id": f.counterparty, "good_key": "copper", "quantity": 20, "travel_mins": 1, "travel_ticks": journey.TravelTicks, "journey": journey, "origin_q": 5, "origin_r": 0, "dest_q": 0, "dest_r": 0}})
			if err != nil {
				t.Fatal(err)
			}
			evt := events.ScheduledEvent{ID: time.Now().UnixNano(), WorldID: f.worldID, Payload: raw}
			// World-scoped physical failure after the return mover+manifest insert.
			triggerName := "fail_saved_return_" + fmt.Sprint(time.Now().UnixNano())
			sql := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.world_id='%s' AND NEW.event_type='TradeReturn' THEN RAISE EXCEPTION 'injected return queue failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER %s BEFORE INSERT ON scheduled_events FOR EACH ROW EXECUTE FUNCTION %s()`, triggerName, f.worldID, triggerName, triggerName)
			if _, err := pool.Exec(ctx, sql); err != nil {
				t.Fatal(err)
			}
			drop := func() {
				if _, err := pool.Exec(ctx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON scheduled_events; DROP FUNCTION IF EXISTS %s()`, triggerName, triggerName)); err != nil {
					t.Error(err)
				}
			}
			t.Cleanup(drop)
			if err := h.Handle(ctx, evt); err == nil {
				t.Fatal("queue failure must roll delivery back")
			}
			var amount float64
			var status string
			if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, f.counterCity).Scan(&amount); err != nil || amount != 0 {
				t.Fatalf("credit on failure %v %v", amount, err)
			}
			if err := pool.QueryRow(ctx, `SELECT status FROM transports WHERE id=$1`, leg).Scan(&status); err != nil || status != "in_transit" {
				t.Fatalf("status on failure %s %v", status, err)
			}
			var count int
			for _, query := range []string{`SELECT count(*) FROM processed_deliveries WHERE event_id=$1`, `SELECT count(*) FROM transports WHERE world_id=$1 AND kind='trade_return'`, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='TradeReturn'`, `SELECT count(*) FROM events WHERE world_id=$1`, `SELECT count(*) FROM notifications WHERE world_id=$1`} {
				arg := any(f.worldID)
				if query == `SELECT count(*) FROM processed_deliveries WHERE event_id=$1` {
					arg = evt.ID
				}
				if err := pool.QueryRow(ctx, query, arg).Scan(&count); err != nil || count != 0 {
					t.Fatalf("failure left rows in %s: %d %v", query, count, err)
				}
			}
			drop()
			if err := h.Handle(ctx, evt); err != nil {
				t.Fatal(err)
			}
			var saved []byte
			var departed, due int
			var ship *uuid.UUID
			var owner uuid.UUID
			if err := pool.QueryRow(ctx, `SELECT journey,departed_tick,due_tick,ship_unit_id,owner_id FROM transports WHERE world_id=$1 AND kind='trade_return'`, f.worldID).Scan(&saved, &departed, &due, &ship, &owner); err != nil {
				t.Fatal(err)
			}
			var stored province.TradeJourney
			if err := json.Unmarshal(saved, &stored); err != nil {
				t.Fatal(err)
			}
			if departed != 17 || due != 17+journey.TravelTicks || stored.TravelTicks != journey.TravelTicks || len(stored.Path) != len(journey.Path) {
				t.Fatalf("saved return %+v ticks%d/%d", stored, departed, due)
			}
			if category == "naval" && (ship == nil || *ship != f.shipID || owner != f.initiator) {
				t.Fatalf("naval return carrier/owner %v/%s", ship, owner)
			}
			if err := h.Handle(ctx, evt); err != nil {
				t.Fatal(err)
			}
			for _, query := range []string{`SELECT count(*) FROM events WHERE world_id=$1 AND event_type='TradeDelivery'`, `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind='TradeDelivery'`, `SELECT count(*) FROM transports WHERE world_id=$1 AND kind='trade_return'`, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='TradeReturn'`} {
				if err := pool.QueryRow(ctx, query, f.worldID).Scan(&count); err != nil || count != 1 {
					t.Fatalf("replay rows %s: %d %v", query, count, err)
				}
			}
		})
	}
}

func TestTradeReturnSavedNavalStrandsAtWaterAndRollsBackReleaseFailure(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newNavalPlayerTradeFixture(t, pool)
	// Home has collapsed and the owner holds no remaining settlement.
	if _, err := pool.Exec(ctx, `UPDATE settlements SET state='collapsed' WHERE id=$1`, f.initiatorCity); err != nil {
		t.Fatal(err)
	}
	journey := province.TradeJourney{Category: "naval", Path: []province.MapPosition{{Q: 1, R: 1}, {R: 1}}, StepCosts: []int64{400}, TravelTicks: 1, Distance: 1}
	raw, _ := json.Marshal(journey)
	var leg uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO transports (world_id,owner_id,kind,origin_id,dest_id,category,origin_q,origin_r,dest_q,dest_r,departs_at,arrives_at,due_tick,ship_unit_id,journey,departed_tick) VALUES ($1,$2,'trade_return',$3,$4,'naval',5,0,0,0,now(),now(),1,$5,$6,0) RETURNING id`, f.worldID, f.initiator, f.counterCity, f.initiatorCity, f.shipID, raw).Scan(&leg); err != nil {
		t.Fatal(err)
	}
	var messenger uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO messengers (world_id,sender_id,origin_id,destination_id,message_text,trade_offer,status,hex_q,hex_r,sent_at,arrives_at) VALUES ($1,$2,$3,$4,'x','{"kind":"buy","status":"accepted"}'::jsonb,'delivered',0,0,now(),now()) RETURNING id`, f.worldID, f.initiator, f.initiatorCity, f.counterCity).Scan(&messenger); err != nil {
		t.Fatal(err)
	}
	h := NewTradeReturnHandler(pool, events.NewStore(pool), nil)
	payload, _ := json.Marshal(map[string]any{"destination_id": f.initiatorCity, "good_key": "copper", "quantity": 20, "messenger_id": messenger, "transport_id": leg})
	evt := events.ScheduledEvent{ID: time.Now().UnixNano(), WorldID: f.worldID, Payload: payload}
	triggerName := "fail_ship_release_" + fmt.Sprint(time.Now().UnixNano())
	if _, err := pool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.id='%s' THEN RAISE EXCEPTION 'injected release failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER %s BEFORE UPDATE ON units FOR EACH ROW EXECUTE FUNCTION %s()`, triggerName, f.shipID, triggerName, triggerName)); err != nil {
		t.Fatal(err)
	}
	drop := func() {
		if _, err := pool.Exec(ctx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON units; DROP FUNCTION IF EXISTS %s()`, triggerName, triggerName)); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(drop)
	if err := h.Handle(ctx, evt); err == nil {
		t.Fatal("release failure must roll credit back")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM settlement_goods WHERE settlement_id=$1 AND good_key='copper'`, f.initiatorCity).Scan(&count); err != nil || count != 0 {
		t.Fatalf("release failure credited goods %d %v", count, err)
	}
	var offer string
	if err := pool.QueryRow(ctx, `SELECT trade_offer->>'status' FROM messengers WHERE id=$1`, messenger).Scan(&offer); err != nil || offer != "accepted" {
		t.Fatalf("release failure resolved offer %s %v", offer, err)
	}
	drop()
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatal(err)
	}
	var status string
	var q, r int
	if err := pool.QueryRow(ctx, `SELECT status,q,r FROM units WHERE id=$1`, f.shipID).Scan(&status, &q, &r); err != nil {
		t.Fatal(err)
	}
	if status != "positioned" || q != 0 || r != 1 {
		t.Fatalf("stranded ship %s (%d,%d), want saved water endpoint", status, q, r)
	}
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatal(err)
	}
	var amount float64
	if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='copper'`, f.initiatorCity).Scan(&amount); err != nil || amount != 20 {
		t.Fatalf("replay credit %v %v", amount, err)
	}
}
