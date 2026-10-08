package handlers

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/capabilities"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/transport"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func giftFixture(t *testing.T, hidden bool) *tradeInternalFixture {
	t.Helper()
	f := setupTradeInternalFixture(t)
	ctx := context.Background()
	var recipient uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "recipient-"+uuid.NewString()).Scan(&recipient); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1`, f.destID, recipient); err != nil {
		t.Fatal(err)
	}
	if hidden {
		if _, err := f.pool.Exec(ctx, `UPDATE provinces SET map_q=20 WHERE id=(SELECT province_id FROM settlements WHERE id=$1)`, f.destID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) SELECT $1,q,0,'plains' FROM generate_series(4,20) q`, f.worldID); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func TestGiftContactedForeignCityDispatch(t *testing.T) {
	f := giftFixture(t, false)
	code, body := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.originProvince.String()+"/trade", map[string]any{"destination_id": f.destID, "good_key": "silver", "quantity": 50})
	if code != 201 {
		t.Fatalf("gift to contacted foreign city returned %d: %v, want 201", code, body)
	}
	var amount float64
	if err := f.pool.QueryRow(context.Background(), `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, f.originID).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if amount != 99950 {
		t.Fatalf("origin amount=%v, want 99950", amount)
	}
	var interceptable bool
	if err := f.pool.QueryRow(context.Background(), `SELECT interceptable FROM transports WHERE id=$1`, body["transport_id"]).Scan(&interceptable); err != nil {
		t.Fatal(err)
	}
	if !interceptable {
		t.Fatal("gift caravan cannot be raided")
	}
}

func TestGiftHiddenForeignCityDeniedWithoutDebit(t *testing.T) {
	f := giftFixture(t, true)
	code, body := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.originProvince.String()+"/trade", map[string]any{"destination_id": f.destID, "good_key": "silver", "quantity": 50})
	if code != 403 {
		t.Fatalf("hidden destination returned %d: %v, want 403", code, body)
	}
	var amount float64
	var movers int
	if err := f.pool.QueryRow(context.Background(), `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, f.originID).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM transports WHERE world_id=$1`, f.worldID).Scan(&movers); err != nil {
		t.Fatal(err)
	}
	if amount != 100000 || movers != 0 {
		t.Fatalf("refusal changed amount=%v movers=%d", amount, movers)
	}
}

func TestGiftDestinationListMatchesLetterFOW(t *testing.T) {
	for _, hidden := range []bool{false, true} {
		t.Run(map[bool]string{false: "contacted", true: "hidden"}[hidden], func(t *testing.T) {
			f := giftFixture(t, hidden)
			ph := NewProvinceHandler(f.pool, f.scheduler, f.clk, economy.SitosConfig{}, nil, nil)
			f.router.Get("/worlds/{worldID}/provinces/{provinceID}/trade/destinations", ph.TransferDestinations)
			request := httptest.NewRequest("GET", "/worlds/"+f.worldID.String()+"/provinces/"+f.originProvince.String()+"/trade/destinations", nil)
			request.Header.Set("Authorization", "Bearer "+f.token)
			response := httptest.NewRecorder()
			f.router.ServeHTTP(response, request)
			if response.Code != 200 {
				t.Fatalf("destinations %d: %s", response.Code, response.Body.String())
			}
			var rows []struct {
				ID        uuid.UUID `json:"settlement_id"`
				Owner     uuid.UUID `json:"owner_id"`
				Name      string    `json:"name"`
				OwnerName string    `json:"owner_name"`
				Own       bool      `json:"own"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil {
				t.Fatal(err)
			}
			if hidden {
				if len(rows) != 0 {
					t.Fatalf("hidden destination leaked: %+v", rows)
				}
				return
			}
			if len(rows) != 1 || rows[0].ID != f.destID || rows[0].Own || rows[0].Owner == f.playerID || rows[0].Name != "Dest" || rows[0].OwnerName == "" {
				t.Fatalf("foreign destination lacks city/owner identity: %+v", rows)
			}
		})
	}
}

type giftDice struct {
	lost  bool
	calls int
}

func (d *giftDice) Float64() float64 {
	d.calls++
	if d.lost {
		return 0
	}
	return 1
}
func (d *giftDice) Intn(n int) int { return 0 }

func TestGiftDeliveryLifecycle(t *testing.T) {
	for _, scenario := range []string{"silver", "timber", "capacity", "new_owner", "collapsed", "deleted", "raided"} {
		t.Run(scenario, func(t *testing.T) {
			f := giftFixture(t, false)
			ctx := context.Background()
			good := "silver"
			if scenario == "timber" {
				good = "timber"
				if _, err := f.pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'timber',100,0,1000000,0)`, f.originID); err != nil {
					t.Fatal(err)
				}
			}
			code, body := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.originProvince.String()+"/trade", map[string]any{"destination_id": f.destID, "good_key": good, "quantity": 50})
			if code != 201 {
				t.Fatalf("gift %d: %v", code, body)
			}
			var event events.ScheduledEvent
			event.WorldID = f.worldID
			if err := f.pool.QueryRow(ctx, `SELECT id,payload FROM scheduled_events WHERE world_id=$1 AND event_type='GiftDelivery' ORDER BY id DESC LIMIT 1`, f.worldID).Scan(&event.ID, &event.Payload); err != nil {
				t.Fatal(err)
			}
			var shipment economy.GiftShipment
			if err := json.Unmarshal(event.Payload, &shipment); err != nil {
				t.Fatal(err)
			}
			wantCredit, wantLost := 50.0, 0.0
			wantKind := "GiftDelivered"
			wantNotices := 2
			if scenario == "capacity" {
				if _, err := f.pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'silver',95,0,100,0)`, f.destID); err != nil {
					t.Fatal(err)
				}
				wantCredit, wantLost = 5, 45
			}
			var actual *uuid.UUID
			if scenario == "new_owner" {
				var owner uuid.UUID
				if err := f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "conqueror-"+uuid.NewString()).Scan(&owner); err != nil {
					t.Fatal(err)
				}
				if _, err := f.pool.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1`, f.destID, owner); err != nil {
					t.Fatal(err)
				}
				actual = &owner
				wantNotices = 3
			}
			if scenario == "collapsed" {
				if _, err := f.pool.Exec(ctx, `UPDATE settlements SET state='collapsed',owner_id=NULL WHERE id=$1`, f.destID); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "deleted" {
				if _, err := f.pool.Exec(ctx, `DELETE FROM settlements WHERE id=$1`, f.destID); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "raided" {
				if _, err := f.pool.Exec(ctx, `UPDATE transports SET status='intercepted' WHERE id=$1`, body["transport_id"]); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "collapsed" || scenario == "deleted" || scenario == "raided" {
				wantCredit, wantLost, wantKind = 0, 50, "GiftLost"
			}
			dice := &giftDice{}
			handler := economy.NewDeliveryHandler(f.pool, events.NewStore(f.pool), nil, f.scheduler)
			handler.Dice = dice
			for i := 0; i < 2; i++ {
				if err := handler.HandleGift(ctx, event); err != nil {
					t.Fatal(err)
				}
			}
			// A second timer id for the same caravan must also not repeat outcomes.
			event.ID += 10000000
			if err := handler.HandleGift(ctx, event); err != nil {
				t.Fatal(err)
			}
			var out economy.GiftOutcome
			var raw []byte
			if err := f.pool.QueryRow(ctx, `SELECT payload FROM events WHERE world_id=$1 AND event_type=$2`, f.worldID, wantKind).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatal(err)
			}
			if out.CreditedQuantity != wantCredit || out.LostQuantity != wantLost || out.SenderID != f.playerID || out.RecipientID != shipment.RecipientID || out.OriginID != f.originID || out.DestinationID != f.destID || out.CreditedQuantity+out.LostQuantity != 50 {
				t.Fatalf("outcome=%+v, want credit=%v lost=%v", out, wantCredit, wantLost)
			}
			if actual != nil && (out.ActualRecipientID == nil || *out.ActualRecipientID != *actual || !out.OwnerChanged) {
				t.Fatalf("changed owner omitted: %+v", out)
			}
			var amount float64
			if err := f.pool.QueryRow(ctx, `SELECT COALESCE((SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key=$2),0)`, f.destID, good).Scan(&amount); err != nil {
				t.Fatal(err)
			}
			wantAmount := wantCredit
			if scenario == "capacity" {
				wantAmount = 100
			}
			if amount != wantAmount {
				t.Fatalf("destination amount=%v, want %v", amount, wantAmount)
			}
			var notices, outcomes int
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind=$2`, f.worldID, wantKind).Scan(&notices); err != nil {
				t.Fatal(err)
			}
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE world_id=$1 AND event_type IN('GiftDelivered','GiftLost')`, f.worldID).Scan(&outcomes); err != nil {
				t.Fatal(err)
			}
			if notices != wantNotices || outcomes != 1 {
				t.Fatalf("notices=%d outcomes=%d, want %d/1", notices, outcomes, wantNotices)
			}
		})
	}
}

func TestGiftNavalCarrierCompletesEmptyReturn(t *testing.T) {
	for _, fallen := range []bool{false, true} {
		t.Run(map[bool]string{false: "delivered", true: "deleted_destination"}[fallen], func(t *testing.T) {
			f := giftFixture(t, false)
			ctx := context.Background()
			if _, err := f.pool.Exec(ctx, `UPDATE provinces SET coastal=true WHERE world_id=$1`, f.worldID); err != nil {
				t.Fatal(err)
			}
			if _, err := f.pool.Exec(ctx, `UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id=$1 AND q IN(1,2)`, f.worldID); err != nil {
				t.Fatal(err)
			}
			var ship uuid.UUID
			if err := f.pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id) VALUES($1,$2,'merchantman','naval',1,10,'garrison',$3) RETURNING id`, f.worldID, f.playerID, f.originID).Scan(&ship); err != nil {
				t.Fatal(err)
			}
			code, body := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.originProvince.String()+"/trade", map[string]any{"destination_id": f.destID, "good_key": "silver", "quantity": 50})
			if code != 201 || body["category"] != "naval" {
				t.Fatalf("naval gift %d: %v", code, body)
			}
			var e events.ScheduledEvent
			e.WorldID = f.worldID
			if err := f.pool.QueryRow(ctx, `SELECT id,payload FROM scheduled_events WHERE world_id=$1 AND event_type='GiftDelivery'`, f.worldID).Scan(&e.ID, &e.Payload); err != nil {
				t.Fatal(err)
			}
			if fallen {
				if _, err := f.pool.Exec(ctx, `DELETE FROM settlements WHERE id=$1`, f.destID); err != nil {
					t.Fatal(err)
				}
			}
			h := economy.NewDeliveryHandler(f.pool, events.NewStore(f.pool), nil, f.scheduler)
			h.Dice = &giftDice{}
			if err := h.HandleGift(ctx, e); err != nil {
				t.Fatal(err)
			}
			var status string
			if err := f.pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, ship).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != "freighting" {
				t.Fatalf("ship released before return: %s", status)
			}
			var ret events.ScheduledEvent
			ret.WorldID = f.worldID
			if err := f.pool.QueryRow(ctx, `SELECT se.id,se.payload FROM scheduled_events se JOIN transports tr ON tr.id=(se.payload->>'transport_id')::uuid WHERE se.world_id=$1 AND se.event_type='TransportArrival' AND tr.kind='ship_return' AND tr.ship_unit_id=$2`, f.worldID, ship).Scan(&ret.ID, &ret.Payload); err != nil {
				t.Fatal(err)
			}
			a := transport.NewArrivalHandler(f.pool, nil)
			for i := 0; i < 2; i++ {
				if err := a.Handle(ctx, ret); err != nil {
					t.Fatal(err)
				}
			}
			var home *uuid.UUID
			if err := f.pool.QueryRow(ctx, `SELECT status,settlement_id FROM units WHERE id=$1`, ship).Scan(&status, &home); err != nil {
				t.Fatal(err)
			}
			if status != "garrison" || home == nil || *home != f.originID {
				t.Fatalf("carrier did not land at home: %s %v", status, home)
			}
			var cargo int
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM transport_goods tg JOIN transports tr ON tr.id=tg.transport_id WHERE tr.world_id=$1 AND tr.kind='ship_return'`, f.worldID).Scan(&cargo); err != nil {
				t.Fatal(err)
			}
			if cargo != 0 {
				t.Fatalf("gift goods incorrectly return: %d", cargo)
			}
		})
	}
}

func TestGiftHistoryPrivateUntilArrival(t *testing.T) {
	f := giftFixture(t, false)
	ctx := context.Background()
	service := auth.NewService(f.pool, "test-secret")
	recipientToken, _, err := service.Register(ctx, "history-"+uuid.NewString(), "x")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := service.ValidateAccessToken(recipientToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1`, f.destID, claims.PlayerID); err != nil {
		t.Fatal(err)
	}
	stranger, _, err := service.Register(ctx, "outsider-"+uuid.NewString(), "x")
	if err != nil {
		t.Fatal(err)
	}
	ph := NewProvinceHandler(f.pool, f.scheduler, f.clk, economy.SitosConfig{}, nil, nil)
	f.router.Get("/worlds/{worldID}/gifts", ph.GiftHistory)
	history := func(token string) []map[string]any {
		t.Helper()
		req := httptest.NewRequest("GET", "/worlds/"+f.worldID.String()+"/gifts", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		f.router.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("history %d: %s", rec.Code, rec.Body.String())
		}
		var rows []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}
	code, body := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.originProvince.String()+"/trade", map[string]any{"destination_id": f.destID, "good_key": "silver", "quantity": 50})
	if code != 201 {
		t.Fatalf("gift %d %v", code, body)
	}
	if rows := history(f.token); len(rows) != 1 || rows[0]["kind"] != "GiftDispatched" {
		t.Fatalf("sender departure: %v", rows)
	}
	if len(history(recipientToken)) != 0 || len(history(stranger)) != 0 {
		t.Fatal("dispatch leaked before cargo arrival")
	}
	var e events.ScheduledEvent
	e.WorldID = f.worldID
	if err := f.pool.QueryRow(ctx, `SELECT id,payload FROM scheduled_events WHERE world_id=$1 AND event_type='GiftDelivery'`, f.worldID).Scan(&e.ID, &e.Payload); err != nil {
		t.Fatal(err)
	}
	h := economy.NewDeliveryHandler(f.pool, events.NewStore(f.pool), nil, f.scheduler)
	h.Dice = &giftDice{}
	if err := h.HandleGift(ctx, e); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{f.token, recipientToken} {
		if rows := history(token); len(rows) != 1 || rows[0]["kind"] != "GiftDelivered" {
			t.Fatalf("outcome not visible once: %v", rows)
		}
	}
	if len(history(stranger)) != 0 {
		t.Fatal("outsider can read gift history")
	}
}

func TestGiftCapabilityWithOneOwnCity(t *testing.T) {
	for _, hidden := range []bool{false, true} {
		t.Run(map[bool]string{false: "contacted", true: "hidden"}[hidden], func(t *testing.T) {
			f := giftFixture(t, hidden)
			verbs := capabilities.List(context.Background(), f.pool, f.clk, f.worldID, f.originProvince, f.playerID, f.originID)
			for _, v := range verbs {
				if v.Name == "transfer" {
					if v.Available == hidden {
						t.Fatalf("transfer available=%t with hidden=%t: %+v", v.Available, hidden, v)
					}
					return
				}
			}
			t.Fatal("transfer action absent")
		})
	}
}
