package handlers

// A declined offer's reservation goes home with its runner (Timothy
// 2026-10-24, temenos_settlement "Svar och kartkunskap" p.3): the goods or
// silver held for the offer come back to the sender's city when the runner
// is home, not when the other side clicks Decline.

import (
	"context"
	"net/http"
	"testing"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
	"formatet/megaron/server/internal/notify"
	"github.com/google/uuid"
)

func TestTradeDecline_ReservationReturnsWithTheRunner(t *testing.T) {
	for _, tc := range []struct {
		kind, good string
		offer      map[string]any
		start      float64
		held       float64
	}{
		{"sell", "copper", map[string]any{"kind": "sell", "offer_good": "copper", "offer_qty": 20.0, "want_silver": 80.0}, 100, 20},
		{"buy", "silver", map[string]any{"kind": "buy", "want_good": "copper", "want_qty": 20.0, "offer_silver": 80.0}, 500, 80},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			f := setupNavalPlayerFixture(t)
			ctx := context.Background()
			hub := notify.New()
			hub.SetPool(f.pool)
			mh := NewMessengerHandler(f.pool, events.NewScheduler(f.pool, f.clk), f.clk, hub)
			f.router.Post("/worlds/{worldID}/messengers/{messengerID}/trade-decline", mh.TradeDecline)

			originID := f.settlement(t, "Tyre", 0, f.initiatorID, false)
			destID := f.settlement(t, "Sidon", 3, f.counterpartyID, false)
			f.mapTile(t, 1, 0, "plains")
			f.mapTile(t, 2, 0, "plains")
			f.good(t, originID, tc.good, tc.start)
			f.good(t, destID, "copper", 100)
			if tc.good != "silver" {
				f.good(t, destID, "silver", 500)
			}

			code, resp := f.post(t, f.initiatorToken,
				"/worlds/"+f.worldID.String()+"/settlements/"+originID.String()+"/messengers",
				map[string]any{"destination_id": destID.String(), "message": "a trade", "trade_offer": tc.offer})
			if code != http.StatusCreated {
				t.Fatalf("Send = %d: %v", code, resp)
			}
			messengerID := uuid.MustParse(resp["id"].(string))
			f.deliverMessenger(t, messengerID)

			stock := func() float64 {
				t.Helper()
				var v float64
				if err := f.pool.QueryRow(ctx,
					`SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key=$2`,
					originID, tc.good).Scan(&v); err != nil {
					t.Fatal(err)
				}
				return v
			}
			if got := stock(); got != tc.start-tc.held {
				t.Fatalf("after send: %s = %v, want %v held", tc.good, got, tc.start-tc.held)
			}

			path := "/worlds/" + f.worldID.String() + "/messengers/" + messengerID.String() + "/trade-decline"
			if code, resp := f.post(t, f.counterpartyToken, path, nil); code != http.StatusOK {
				t.Fatalf("trade-decline = %d: %v", code, resp)
			}
			if got := stock(); got != tc.start-tc.held {
				t.Fatalf("declined, runner not home: %s = %v, want still %v — the reservation travels with the runner",
					tc.good, got, tc.start-tc.held)
			}

			var e events.ScheduledEvent
			if err := f.pool.QueryRow(ctx,
				`SELECT id, world_id, event_type, payload, due_tick FROM scheduled_events
				  WHERE world_id=$1 AND event_type=$2 AND payload->>'messenger_id'=$3
				  ORDER BY id DESC LIMIT 1`,
				f.worldID, events.ScheduledMessengerReturn, messengerID.String(),
			).Scan(&e.ID, &e.WorldID, &e.EventType, &e.Payload, &e.DueTick); err != nil {
				t.Fatalf("load MessengerReturn: %v", err)
			}
			rh := messenger.NewReturnHandler(f.pool, events.NewStore(f.pool), hub)
			for i := 0; i < 2; i++ { // the same homecoming run twice returns it once
				if err := rh.Handle(ctx, e); err != nil {
					t.Fatalf("MessengerReturn: %v", err)
				}
			}
			if got := stock(); got != tc.start {
				t.Fatalf("runner home: %s = %v, want %v (returned exactly once)", tc.good, got, tc.start)
			}
		})
	}
}
