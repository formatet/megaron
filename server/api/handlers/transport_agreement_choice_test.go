package handlers

import (
	"context"
	"fmt"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"testing"
)

func TestAgreementChoicePersistsAndControlsAcceptedDispatch(t *testing.T) {
	for _, kind := range []string{"sell", "buy"} {
		for _, by := range []string{"land", "sea"} {
			t.Run(kind+"_"+by, func(t *testing.T) {
				f := setupNavalPlayerFixture(t)
				ctx := context.Background()
				origin := f.settlement(t, "Proposer", 0, f.initiatorID, true)
				dest := f.settlement(t, "Counterparty", 5, f.counterpartyID, true)
				for q := 1; q < 5; q++ {
					f.mapTile(t, q, 0, "coastal_sea")
				}
				for q := 0; q <= 5; q++ {
					f.mapTile(t, q, 1, "plains")
				}
				ship := f.ship(t, origin, f.initiatorID, "merchantman")
				f.good(t, origin, "silver", 100)
				f.good(t, dest, "silver", 100)
				f.good(t, origin, "copper", 100)
				f.good(t, dest, "copper", 100)
				offer := map[string]any{"kind": kind, "by": by}
				if kind == "sell" {
					offer["offer_good"] = "copper"
					offer["offer_qty"] = 10
					offer["want_silver"] = 20
				} else {
					offer["want_good"] = "copper"
					offer["want_qty"] = 10
					offer["offer_silver"] = 20
				}
				code, body := f.post(t, f.initiatorToken, fmt.Sprintf("/worlds/%s/settlements/%s/messengers", f.worldID, origin), map[string]any{"destination_id": dest, "message": "Route choice", "trade_offer": offer})
				if code != 201 {
					t.Fatalf("send %d %+v", code, body)
				}
				id, err := uuid.Parse(body["id"].(string))
				if err != nil {
					t.Fatal(err)
				}
				var saved string
				if err := f.pool.QueryRow(ctx, `SELECT trade_offer->>'by' FROM messengers WHERE id=$1`, id).Scan(&saved); err != nil || saved != by {
					t.Fatalf("saved choice%q err%v", saved, err)
				}
				f.deliverMessenger(t, id)
				code, body = f.post(t, f.counterpartyToken, fmt.Sprintf("/worlds/%s/messengers/%s/trade-accept", f.worldID, id), nil)
				if code != 200 {
					t.Fatalf("accept %d %+v", code, body)
				}
				var category, mode, status string
				if err := f.pool.QueryRow(ctx, `SELECT category,route_mode FROM transports WHERE world_id=$1 AND kind='trade'`, f.worldID).Scan(&category, &mode); err != nil {
					t.Fatal(err)
				}
				want := "land"
				if by == "sea" {
					want = "naval"
				}
				if category != want || mode != by || body["by"] != by {
					t.Fatalf("agreement ignored choice category%q mode%q response%v", category, mode, body)
				}
				_ = f.pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, ship).Scan(&status)
				if by == "land" && status != "garrison" {
					t.Fatalf("land choice bound hull %s", status)
				}
				var delivery events.ScheduledEvent
				delivery.WorldID = f.worldID
				if err := f.pool.QueryRow(ctx, `SELECT id,payload,COALESCE(due_tick,0) FROM scheduled_events WHERE world_id=$1 AND event_type IN ('TradeDelivery','TradeDeliveryV2') ORDER BY id DESC LIMIT 1`, f.worldID).Scan(&delivery.ID, &delivery.Payload, &delivery.DueTick); err != nil {
					t.Fatal(err)
				}
				handler := economy.NewDeliveryHandler(f.pool, events.NewStore(f.pool), nil, events.NewScheduler(f.pool, f.clk))
				if err := handler.Handle(ctx, delivery); err != nil {
					t.Fatal(err)
				}
				var retBy, retCategory string
				if err := f.pool.QueryRow(ctx, `SELECT route_mode,category FROM transports WHERE world_id=$1 AND kind='trade_return'`, f.worldID).Scan(&retBy, &retCategory); err != nil {
					t.Fatal(err)
				}
				if retBy != by || retCategory != want {
					t.Fatalf("chained return lost selection mode%q category%q", retBy, retCategory)
				}

			})
		}
	}
}

func TestAgreementExplicitSeaNeverFallsBackAtAccept(t *testing.T) {
	f := setupNavalPlayerFixture(t)
	ctx := context.Background()
	origin := f.settlement(t, "Proposer", 0, f.initiatorID, true)
	dest := f.settlement(t, "Counterparty", 5, f.counterpartyID, true)
	for q := 1; q < 5; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	for q := 0; q <= 5; q++ {
		f.mapTile(t, q, 1, "plains")
	}
	ship := f.ship(t, origin, f.initiatorID, "merchantman")
	f.good(t, origin, "copper", 100)
	f.good(t, dest, "silver", 100)
	code, body := f.post(t, f.initiatorToken, fmt.Sprintf("/worlds/%s/settlements/%s/messengers", f.worldID, origin), map[string]any{"destination_id": dest, "message": "Sea only", "trade_offer": map[string]any{"kind": "sell", "by": "sea", "offer_good": "copper", "offer_qty": 10, "want_silver": 20}})
	if code != 201 {
		t.Fatalf("send %d %+v", code, body)
	}
	id, _ := uuid.Parse(body["id"].(string))
	f.deliverMessenger(t, id)
	if _, err := f.pool.Exec(ctx, `UPDATE units SET status='freighting' WHERE id=$1`, ship); err != nil {
		t.Fatal(err)
	}
	code, body = f.post(t, f.counterpartyToken, fmt.Sprintf("/worlds/%s/messengers/%s/trade-accept", f.worldID, id), nil)
	if code != 422 {
		t.Fatalf("explicitsea accepted fallback %d %+v", code, body)
	}
	var status string
	var n int
	_ = f.pool.QueryRow(ctx, `SELECT trade_offer->>'status' FROM messengers WHERE id=$1`, id).Scan(&status)
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM transports WHERE world_id=$1`, f.worldID).Scan(&n)
	if status != "pending" || n != 0 {
		t.Fatalf("failed choice changed offer=%s movers=%d", status, n)
	}
}
