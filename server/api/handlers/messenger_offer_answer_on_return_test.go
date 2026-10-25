package handlers

// The answer to a trade offer travels with its runner (Timothy 2026-10-24,
// temenos_settlement "Svar och kartkunskap"): the sender learns "accepted" or
// "declined" when the runner is home, not when the other side clicks. Until
// then TradeAccept/TradeDecline sent OfferAccepted/OfferDeclined at once.

import (
	"context"
	"net/http"
	"testing"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
	"formatet/megaron/server/internal/notify"
	"github.com/google/uuid"
)

func TestTradeOfferAnswer_ArrivesWithTheRunner(t *testing.T) {
	for _, tc := range []struct{ end, want string }{
		{"accept", "OfferAccepted"},
		{"decline", "OfferDeclined"},
		{"cancel", ""},
	} {
		t.Run(tc.end, func(t *testing.T) {
			f := setupNavalPlayerFixture(t)
			ctx := context.Background()
			hub := notify.New()
			hub.SetPool(f.pool)
			mh := NewMessengerHandler(f.pool, events.NewScheduler(f.pool, f.clk), f.clk, hub)
			f.router.Post("/worlds/{worldID}/messengers/{messengerID}/trade-decline", mh.TradeDecline)
			f.router.Post("/worlds/{worldID}/messengers/{messengerID}/trade-cancel", mh.CancelOffer)

			sellerID := f.settlement(t, "Tyre", 0, f.initiatorID, false)
			buyerID := f.settlement(t, "Sidon", 3, f.counterpartyID, false)
			f.mapTile(t, 1, 0, "plains")
			f.mapTile(t, 2, 0, "plains")
			f.good(t, sellerID, "copper", 100)
			f.good(t, buyerID, "silver", 500)

			code, resp := f.post(t, f.initiatorToken,
				"/worlds/"+f.worldID.String()+"/settlements/"+sellerID.String()+"/messengers",
				map[string]any{
					"destination_id": buyerID.String(),
					"message":        "copper for sale",
					"trade_offer":    map[string]any{"kind": "sell", "offer_good": "copper", "offer_qty": 20.0, "want_silver": 80.0},
				})
			if code != http.StatusCreated {
				t.Fatalf("Send = %d: %v", code, resp)
			}
			idStr, _ := resp["id"].(string)
			messengerID := uuid.MustParse(idStr)
			f.deliverMessenger(t, messengerID)

			answers := func() map[string]int {
				t.Helper()
				rows, err := f.pool.Query(ctx,
					`SELECT kind, count(*) FROM notifications
					  WHERE world_id=$1 AND player_id=$2 AND kind IN ('OfferAccepted','OfferDeclined')
					  GROUP BY kind`, f.worldID, f.initiatorID)
				if err != nil {
					t.Fatal(err)
				}
				defer rows.Close()
				got := map[string]int{}
				for rows.Next() {
					var k string
					var n int
					if err := rows.Scan(&k, &n); err != nil {
						t.Fatal(err)
					}
					got[k] = n
				}
				return got
			}

			token := f.counterpartyToken
			if tc.end == "cancel" {
				token = f.initiatorToken
			}
			path := "/worlds/" + f.worldID.String() + "/messengers/" + messengerID.String() + "/trade-" + tc.end
			if code, resp := f.post(t, token, path, nil); code != http.StatusOK {
				t.Fatalf("trade-%s = %d: %v", tc.end, code, resp)
			}
			if got := answers(); len(got) != 0 {
				t.Fatalf("after trade-%s, before the runner is home, the sender already has %v — the answer must travel with the runner", tc.end, got)
			}

			var e events.ScheduledEvent
			if err := f.pool.QueryRow(ctx,
				`SELECT id, world_id, event_type, payload, due_tick FROM scheduled_events
				  WHERE world_id=$1 AND event_type IN ($2,$2||'V2',CASE WHEN $2='ShipmentDeliveryV2' THEN 'ShipmentDeliveryV3' ELSE $2 END) AND payload->>'messenger_id'=$3
				  ORDER BY id DESC LIMIT 1`,
				f.worldID, events.ScheduledMessengerReturn, messengerID.String(),
			).Scan(&e.ID, &e.WorldID, &e.EventType, &e.Payload, &e.DueTick); err != nil {
				t.Fatalf("load MessengerReturn: %v", err)
			}
			rh := messenger.NewReturnHandler(f.pool, events.NewStore(f.pool), hub)
			for i := 0; i < 2; i++ { // replay must not repeat the notice
				if err := rh.Handle(ctx, e); err != nil {
					t.Fatalf("MessengerReturn: %v", err)
				}
			}

			got := answers()
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("a cancelled offer's homecoming sent %v, want nothing", got)
				}
				return
			}
			if len(got) != 1 || got[tc.want] != 1 {
				t.Fatalf("runner home after trade-%s: sender has %v, want exactly one %s", tc.end, got, tc.want)
			}
		})
	}
}
