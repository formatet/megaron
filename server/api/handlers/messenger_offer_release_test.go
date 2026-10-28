package handlers

// An ended trade offer releases its runner at once (todo: "Terminala offerter
// ska inte hålla bud till ursprunglig offertfrist"). Before this, a runner whose
// offer was accepted, declined or cancelled still stood in the other city until
// the offer's original expiry, OfferExpiryTicks later.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
	"github.com/google/uuid"
)

func TestTradeOfferEnd_RunnerTurnsHomeSameTick(t *testing.T) {
	for _, end := range []string{"accept", "decline", "cancel"} {
		t.Run(end, func(t *testing.T) {
			f := setupNavalPlayerFixture(t)
			ctx := context.Background()
			mh := NewMessengerHandler(f.pool, events.NewScheduler(f.pool, f.clk), f.clk, nil)
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

			var tick int
			if err := f.pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&tick); err != nil {
				t.Fatal(err)
			}
			path := "/worlds/" + f.worldID.String() + "/messengers/" + messengerID.String() + "/trade-" + end
			token := f.counterpartyToken
			if end == "cancel" {
				token = f.initiatorToken
			}
			if code, resp := f.post(t, token, path, nil); code != http.StatusOK {
				t.Fatalf("trade-%s = %d: %v", end, code, resp)
			}

			var status string
			var returnDue *int
			if err := f.pool.QueryRow(ctx,
				`SELECT m.status, (SELECT min(due_tick) FROM scheduled_events WHERE event_type IN ('MessengerReturn','MessengerReturnV2') AND payload->>'messenger_id'=$2)
				   FROM messengers m WHERE m.id=$1`, messengerID, messengerID.String()).Scan(&status, &returnDue); err != nil {
				t.Fatal(err)
			}
			if status != "returning" {
				t.Fatalf("after trade-%s the runner is %q, want returning the same tick", end, status)
			}
			if returnDue == nil || *returnDue >= tick+messenger.OfferExpiryTicks {
				t.Errorf("runner's homecoming due %v, want well before the old expiry tick %d", returnDue, tick+messenger.OfferExpiryTicks)
			}

			// The offer's own expiry, firing later, changes nothing.
			raw, _ := json.Marshal(map[string]string{"messenger_id": messengerID.String()})
			if err := economy.NewOfferExpiryHandler(f.pool, events.NewScheduler(f.pool, f.clk)).Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: tick + messenger.OfferExpiryTicks, Payload: raw}); err != nil {
				t.Fatalf("late OfferExpiry: %v", err)
			}
			var offerStatus string
			if err := f.pool.QueryRow(ctx, `SELECT m.status, m.trade_offer->>'status' FROM messengers m WHERE id=$1`, messengerID).Scan(&status, &offerStatus); err != nil {
				t.Fatal(err)
			}
			if status != "returning" || offerStatus == "expired" {
				t.Errorf("late expiry touched an ended offer: runner=%q offer=%q", status, offerStatus)
			}
		})
	}
}
