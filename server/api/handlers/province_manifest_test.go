package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"

	"github.com/google/uuid"
	"net/http/httptest"
	"testing"
)

func TestTransferManifestAtomicDelivery(t *testing.T) {
	for _, tc := range []struct {
		name          string
		grain, silver float64
		code          int
	}{{"two goods one hull", 70, 40, 201}, {"total exceeds hull", 70, 60, 422}, {"exact hull ignores old weight", 80, 40, 201}} {
		t.Run(tc.name, func(t *testing.T) {
			f := setupTradeNavalFixture(t)
			ctx := context.Background()
			origin, prov := f.settlement(t, "Byblos", 0, true, true)
			dest, _ := f.settlement(t, "Ugarit", 5, true, false)
			_, _ = f.pool.Exec(ctx, `UPDATE settlement_goods SET amount=0 WHERE settlement_id=$1`, dest)
			for q := 1; q < 5; q++ {
				f.mapTile(t, q, 0, "coastal_sea")
			}
			ship := f.ship(t, origin, "merchantman")
			_, err := f.pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'silver',1000,0,1000000,0)`, origin)
			if err != nil {
				t.Fatal(err)
			}
			code, resp := f.post(t, fmt.Sprintf("/worlds/%s/provinces/%s/trade", f.worldID, prov), map[string]any{"destination_id": dest, "items": []map[string]any{{"good_key": "grain", "quantity": tc.grain}, {"good_key": "silver", "quantity": tc.silver}}})
			if code != tc.code {
				t.Fatalf("status %d want %d: %v", code, tc.code, resp)
			}
			var status string
			f.pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, ship).Scan(&status)
			var grain, silver float64
			f.pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'`, origin).Scan(&grain)
			f.pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, origin).Scan(&silver)
			if code == 422 {
				if grain != 100000 || silver != 1000 || status != "garrison" {
					t.Fatalf("rejection mutated stock/hull %v %v %s", grain, silver, status)
				}
				return
			}
			if status != "freighting" {
				t.Fatal(status)
			}
			var count int
			f.pool.QueryRow(ctx, `SELECT count(*) FROM transports WHERE world_id=$1`, f.worldID).Scan(&count)
			if count != 1 {
				t.Fatalf("%d transports", count)
			}
			var e events.ScheduledEvent
			e.WorldID = f.worldID
			if err := f.pool.QueryRow(ctx, `SELECT id,payload FROM scheduled_events WHERE world_id=$1 AND event_type IN ('ShipmentDeliveryV2','ShipmentDeliveryV3')`, f.worldID).Scan(&e.ID, &e.Payload); err != nil {
				t.Fatal(err)
			}
			var runner uuid.UUID
			if err := f.pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,kind,status,hex_q,hex_r,arrives_at,passage_status,carrier_unit_id) VALUES($1,$2,$3,$4,'SEALED PRIVATE TEXT','message','outbound',5,0,now(),'aboard',$5) RETURNING id`, f.worldID, f.playerID, origin, dest, ship).Scan(&runner); err != nil {
				t.Fatal(err)
			}
			h := economy.NewDeliveryHandler(f.pool, events.NewStore(f.pool), nil, f.scheduler)
			// Dispatch to the V2 handler through a temporary interface: the old baseline
			// compiles, then fails on the API contract before this point.
			handler, ok := any(h).(interface {
				HandleShipmentV2(context.Context, events.ScheduledEvent) error
			})
			if !ok {
				t.Fatal("missing V2 handler")
			}
			for i := 0; i < 2; i++ {
				if err := handler.HandleShipmentV2(ctx, e); err != nil {
					t.Fatal(err)
				}
			}
			var witnessCount int
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='CarrierPassengerLandedV1'`, runner).Scan(&witnessCount); err != nil || witnessCount != 1 {
				t.Fatalf("landing witness %d %v", witnessCount, err)
			}
			for good, want := range map[string]float64{"grain": tc.grain, "silver": tc.silver} {
				var got float64
				if err := f.pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key=$2`, dest, good).Scan(&got); err != nil {
					t.Fatal(err)
				}
				if got != want {
					t.Fatalf("%s got %v want %v", good, got, want)
				}
			}
			f.pool.QueryRow(ctx, `SELECT count(*) FROM transports WHERE world_id=$1 AND kind='ship_return'`, f.worldID).Scan(&count)
			if count != 1 {
				t.Fatalf("%d return legs", count)
			}
			var payload map[string]any
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				t.Fatal(err)
			}

		})
	}
}

func TestGiftManifestDeliveryAndRetry(t *testing.T) {
	f := giftFixture(t, false)
	ctx := context.Background()
	_, err := f.pool.Exec(ctx, `UPDATE settlement_goods SET amount=0 WHERE settlement_id=$1`, f.destID)
	if err != nil {
		t.Fatal(err)
	}
	code, resp := f.post(t, fmt.Sprintf("/worlds/%s/provinces/%s/trade", f.worldID, f.originProvince), map[string]any{"destination_id": f.destID, "items": []map[string]any{{"good_key": "silver", "quantity": 30}, {"good_key": "fish", "quantity": 20}}})
	if code != 201 {
		t.Fatalf("%d %v", code, resp)
	}
	var e events.ScheduledEvent
	e.WorldID = f.worldID
	if err := f.pool.QueryRow(ctx, `SELECT id,payload FROM scheduled_events WHERE world_id=$1 AND event_type IN ('ShipmentDeliveryV2','ShipmentDeliveryV3')`, f.worldID).Scan(&e.ID, &e.Payload); err != nil {
		t.Fatal(err)
	}
	h := economy.NewDeliveryHandler(f.pool, events.NewStore(f.pool), nil, f.scheduler)
	for i := 0; i < 2; i++ {
		if err := h.HandleShipmentV2(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	for good, want := range map[string]float64{"silver": 30, "fish": 20} {
		var got float64
		if err := f.pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key=$2`, f.destID, good).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s %v want %v", good, got, want)
		}
	}
	var count int
	f.pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE world_id=$1 AND event_type='GiftDeliveredV2'`, f.worldID).Scan(&count)
	if count != 1 {
		t.Fatalf("outcomes %d", count)
	}
}

func TestManifestValidationAndRollback(t *testing.T) {
	for _, items := range []any{[]any{}, []map[string]any{{"good_key": "silver", "quantity": 10}, {"good_key": "cult", "quantity": 10}}, []map[string]any{{"good_key": "silver", "quantity": 10}, {"good_key": "fish", "quantity": 1000000}}, []map[string]any{{"good_key": "silver", "quantity": 10}, {"good_key": "missing", "quantity": 1}}} {
		f := giftFixture(t, false)
		ctx := context.Background()
		code, resp := f.post(t, fmt.Sprintf("/worlds/%s/provinces/%s/trade", f.worldID, f.originProvince), map[string]any{"destination_id": f.destID, "items": items})
		if code != 400 && code != 422 {
			t.Fatalf("invalid accepted %d %v", code, resp)
		}
		var amount float64
		f.pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, f.originID).Scan(&amount)
		if amount != 100000 {
			t.Fatalf("partial debit %v", amount)
		}
		var count int
		f.pool.QueryRow(ctx, `SELECT count(*) FROM transports WHERE world_id=$1`, f.worldID).Scan(&count)
		if count != 0 {
			t.Fatalf("partial dispatch %d", count)
		}
	}
}

func TestManifestPreviewAndRouteReadModel(t *testing.T) {
	f := setupTradeNavalFixture(t)
	ctx := context.Background()
	origin, prov := f.settlement(t, "Byblos", 0, true, true)
	dest, _ := f.settlement(t, "Ugarit", 5, true, false)
	for q := 1; q < 5; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	ship := f.ship(t, origin, "merchantman")
	f.router.Get("/worlds/{worldID}/provinces/{provinceID}/trade/preview", NewProvinceHandler(f.pool, f.scheduler, f.clk, events.NewStore(f.pool), nil).TradePreview)
	f.router.Get("/worlds/{worldID}/provinces/{provinceID}/trade", NewProvinceHandler(f.pool, f.scheduler, f.clk, events.NewStore(f.pool), nil).TradeRoutes)
	get := func(path string) []byte {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+f.token)
		rec := httptest.NewRecorder()
		f.router.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("GET %d %s", rec.Code, rec.Body.String())
		}
		return rec.Body.Bytes()
	}
	path := fmt.Sprintf("/worlds/%s/provinces/%s/trade", f.worldID, prov)
	var preview map[string]any
	if err := json.Unmarshal(get(path+"/preview?destination_id="+dest.String()), &preview); err != nil {
		t.Fatal(err)
	}
	if preview["cargo_capacity"] != float64(120) || preview["ship_id"] != ship.String() {
		t.Fatalf("preview %v", preview)
	}
	var status string
	f.pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, ship).Scan(&status)
	if status != "garrison" {
		t.Fatal("preview bound ship")
	}
	_, err := f.pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'silver',1000,0,1000000,0)`, origin)
	if err != nil {
		t.Fatal(err)
	}
	code, resp := f.post(t, path, map[string]any{"destination_id": dest, "items": []map[string]any{{"good_key": "grain", "quantity": 10}, {"good_key": "silver", "quantity": 20}, {"good_key": "grain", "quantity": 5}}})
	if code != 201 {
		t.Fatalf("%d %v", code, resp)
	}
	var routes []struct {
		Items []economy.CargoItem `json:"items"`
	}
	if err := json.Unmarshal(get(path), &routes); err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 || len(routes[0].Items) != 2 || routes[0].Items[0].Quantity != 15 {
		t.Fatalf("routes %v", routes)
	}
}

func TestManifestLossReportsEveryGoodAndProtectsRecipientOrigin(t *testing.T) {
	for _, state := range []string{"delivered", "intercepted", "foundered"} {
		t.Run(state, func(t *testing.T) {
			f := giftFixture(t, false)
			ctx := context.Background()
			code, resp := f.post(t, fmt.Sprintf("/worlds/%s/provinces/%s/trade", f.worldID, f.originProvince), map[string]any{"destination_id": f.destID, "items": []map[string]any{{"good_key": "silver", "quantity": 30}, {"good_key": "fish", "quantity": 20}}})
			if code != 201 {
				t.Fatalf("%d %v", code, resp)
			}
			var e events.ScheduledEvent
			e.WorldID = f.worldID
			if err := f.pool.QueryRow(ctx, `SELECT id,payload FROM scheduled_events WHERE world_id=$1 AND event_type IN ('ShipmentDeliveryV2','ShipmentDeliveryV3')`, f.worldID).Scan(&e.ID, &e.Payload); err != nil {
				t.Fatal(err)
			}
			if state != "delivered" {
				if _, err := f.pool.Exec(ctx, `UPDATE transports SET status=$2 WHERE id=$1`, resp["transport_id"], state); err != nil {
					t.Fatal(err)
				}
			}
			if state == "intercepted" {
				_, err := events.NewStore(f.pool).Append(ctx, f.originID, events.StreamProvince, "TransportDamagedReturnDispatched", map[string]any{"transport_id": resp["transport_id"], "return_transport_id": uuid.New(), "returned_goods": map[string]float64{"silver": 15, "fish": 10}}, f.worldID, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			h := economy.NewDeliveryHandler(f.pool, events.NewStore(f.pool), nil, f.scheduler)
			if err := h.HandleShipmentV2(ctx, e); err != nil {
				t.Fatal(err)
			}
			kind := "GiftDeliveredV2"
			if state != "delivered" {
				kind = "GiftLostV2"
			}
			var raw []byte
			if err := f.pool.QueryRow(ctx, `SELECT payload FROM events WHERE world_id=$1 AND event_type IN ($2,$2||'V2',CASE WHEN $2='ShipmentDeliveryV2' THEN 'ShipmentDeliveryV3' ELSE $2 END)`, f.worldID, kind).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var outcome economy.ShipmentOutcomeV2
			if err := json.Unmarshal(raw, &outcome); err != nil {
				t.Fatal(err)
			}
			if len(outcome.Items) != 2 {
				t.Fatal(string(raw))
			}
			for _, it := range outcome.Items {
				if state == "intercepted" && (it.ReturnedQuantity != it.Quantity/2 || it.LostQuantity != it.Quantity/2) {
					t.Fatal(string(raw))
				}
				if state == "foundered" && (it.LostQuantity != it.Quantity || it.ReturnedQuantity != 0) {
					t.Fatal(string(raw))
				}
			}
			var recipient uuid.UUID
			f.pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, f.destID).Scan(&recipient)
			if err := f.pool.QueryRow(ctx, `SELECT body_json FROM notifications WHERE world_id=$1 AND player_id=$2 AND kind=$3`, f.worldID, recipient, kind).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var notice map[string]any
			json.Unmarshal(raw, &notice)
			if _, leaked := notice["origin_id"]; leaked {
				t.Fatalf("origin id leaked %s", raw)
			}
			if _, leaked := notice["origin_name"]; leaked {
				t.Fatalf("origin name leaked %s", raw)
			}
			// A second timer for the same route must not create another outcome.
			e.ID += 100000000
			if err := h.HandleShipmentV2(ctx, e); err != nil {
				t.Fatal(err)
			}
			var count int
			f.pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE world_id=$1 AND event_type IN ($2,$2||'V2',CASE WHEN $2='ShipmentDeliveryV2' THEN 'ShipmentDeliveryV3' ELSE $2 END)`, f.worldID, kind).Scan(&count)
			if count != 1 {
				t.Fatalf("retry produced %d outcomes", count)
			}
		})
	}
}

// Internal shipments preserve the legacy destination semantics independently of
// who holds the city when they arrive. Gifts retain their separate contract.
func TestTransferV2MatchesLegacyDestinationChanges(t *testing.T) {
	for _, change := range []string{"new owner", "collapsed city", "ownerless city", "deleted city"} {
		t.Run(change, func(t *testing.T) {
			type outcome struct {
				amount float64
				status string
				routes int
			}
			var outcomes []outcome
			for _, list := range []bool{false, true} {
				f := setupTradeNavalFixture(t)
				ctx := context.Background()
				origin, prov := f.settlement(t, "Byblos", 0, true, true)
				dest, _ := f.settlement(t, "Ugarit", 5, true, false)
				for q := 1; q < 5; q++ {
					f.mapTile(t, q, 0, "coastal_sea")
				}
				f.ship(t, origin, "merchantman")
				_, err := f.pool.Exec(ctx, `UPDATE settlement_goods SET amount=0 WHERE settlement_id=$1`, dest)
				if err != nil {
					t.Fatal(err)
				}
				body := map[string]any{"destination_id": dest, "good_key": "grain", "quantity": 10}
				timer := "TradeDelivery"
				if list {
					body = map[string]any{"destination_id": dest, "items": []map[string]any{{"good_key": "grain", "quantity": 10}}}
					timer = "ShipmentDeliveryV2"
				}
				code, resp := f.post(t, fmt.Sprintf("/worlds/%s/provinces/%s/trade", f.worldID, prov), body)
				if code != 201 {
					t.Fatalf("%d %v", code, resp)
				}
				var e events.ScheduledEvent
				e.WorldID = f.worldID
				if err := f.pool.QueryRow(ctx, `SELECT id,payload FROM scheduled_events WHERE world_id=$1 AND event_type IN ($2,$2||'V2',CASE WHEN $2='ShipmentDeliveryV2' THEN 'ShipmentDeliveryV3' ELSE $2 END)`, f.worldID, timer).Scan(&e.ID, &e.Payload); err != nil {
					t.Fatal(err)
				}
				switch change {
				case "new owner":
					var other uuid.UUID
					if err := f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "new-holder-"+uuid.NewString()).Scan(&other); err != nil {
						t.Fatal(err)
					}
					_, err = f.pool.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1`, dest, other)
				case "collapsed city":
					_, err = f.pool.Exec(ctx, `UPDATE settlements SET state='collapsed' WHERE id=$1`, dest)
				case "ownerless city":
					_, err = f.pool.Exec(ctx, `UPDATE settlements SET owner_id=NULL WHERE id=$1`, dest)
				case "deleted city":
					_, err = f.pool.Exec(ctx, `DELETE FROM settlements WHERE id=$1`, dest)
				}
				if err != nil {
					t.Fatal(err)
				}
				h := economy.NewDeliveryHandler(f.pool, events.NewStore(f.pool), nil, f.scheduler)
				if list {
					err = h.HandleShipmentV2(ctx, e)
				} else {
					err = h.Handle(ctx, e)
				}
				if err != nil {
					t.Fatal(err)
				}
				var got outcome
				if err := f.pool.QueryRow(ctx, `SELECT COALESCE((SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'),0),(SELECT status FROM transports WHERE id=$2),(SELECT count(*) FROM transports WHERE world_id=$3 AND kind='ship_return')`, dest, resp["transport_id"], f.worldID).Scan(&got.amount, &got.status, &got.routes); err != nil {
					t.Fatal(err)
				}
				outcomes = append(outcomes, got)
			}
			if outcomes[0] != outcomes[1] {
				t.Fatalf("legacy %+v V2 %+v", outcomes[0], outcomes[1])
			}
			if change != "deleted city" && (outcomes[1].amount != 10 || outcomes[1].status != "delivered" || outcomes[1].routes != 1) {
				t.Fatalf("delivery changed %+v", outcomes)
			}
		})
	}
}
