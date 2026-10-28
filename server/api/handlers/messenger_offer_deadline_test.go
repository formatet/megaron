package handlers

import (
	"context"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"net/http"
	"testing"
	"time"
)

// Literal 24 is the approved new-offer contract, measured through actual Send
// and ArrivalHandler scheduling rather than comparing a constant to itself.
func TestNewTradeOfferDeadlineIsActualArrivalPlus24(t *testing.T) {
	for _, kind := range []string{"buy", "sell"} {
		t.Run(kind, func(t *testing.T) {
			f := setupNavalPlayerFixture(t)
			ctx := context.Background()
			origin := f.settlement(t, "Origin", 0, f.initiatorID, false)
			dest := f.settlement(t, "Destination", 3, f.counterpartyID, false)
			f.mapTile(t, 1, 0, "plains")
			f.mapTile(t, 2, 0, "plains")
			f.good(t, origin, "silver", 500)
			f.good(t, origin, "copper", 100)
			offer := map[string]any{"kind": "buy", "want_good": "tin", "want_qty": 10, "offer_silver": 50}
			if kind == "sell" {
				offer = map[string]any{"kind": "sell", "offer_good": "copper", "offer_qty": 10, "want_silver": 50}
			}
			code, resp := f.post(t, f.initiatorToken, "/worlds/"+f.worldID.String()+"/settlements/"+origin.String()+"/messengers", map[string]any{"destination_id": dest.String(), "message": "An offer", "trade_offer": offer})
			if code != http.StatusCreated {
				t.Fatalf("Send=%d %v", code, resp)
			}
			id := uuid.MustParse(resp["id"].(string))
			var arrival events.ScheduledEvent
			arrival.WorldID = f.worldID
			if err := f.pool.QueryRow(ctx, `SELECT id,event_type,payload,due_tick,process_after FROM scheduled_events WHERE payload->>'messenger_id'=$1 AND event_type IN ('MessengerArrival','MessengerArrivalV2')`, id.String()).Scan(&arrival.ID, &arrival.EventType, &arrival.Payload, &arrival.DueTick, &arrival.ProcessAfter); err != nil {
				t.Fatal(err)
			}
			var expiry int
			var arrivesAt, expiresAt time.Time
			if err := f.pool.QueryRow(ctx, `SELECT arrives_at,expires_at,(SELECT due_tick FROM scheduled_events WHERE payload->>'messenger_id'=$2 AND event_type='OfferExpiry') FROM messengers WHERE id=$1`, id, id.String()).Scan(&arrivesAt, &expiresAt, &expiry); err != nil {
				t.Fatal(err)
			}
			if expiry-arrival.DueTick != 24 || expiresAt.Sub(arrivesAt) != tick.RealUntil(24, 0) {
				t.Fatalf("new offer expiry must equal actual arrival+24: arrival=%d expiry=%d wall=%v", arrival.DueTick, expiry, expiresAt.Sub(arrivesAt))
			}
			h := messenger.NewArrivalHandler(f.pool, events.NewScheduler(f.pool, f.clk), events.NewStore(f.pool), nil)
			if err := h.Handle(ctx, arrival); err != nil {
				t.Fatal(err)
			}
			if err := h.Handle(ctx, arrival); err != nil {
				t.Fatal(err)
			}
			var stay, n int
			if err := f.pool.QueryRow(ctx, `SELECT min(due_tick),count(*) FROM scheduled_events WHERE event_type='MessengerStayEnd' AND payload->>'messenger_id'=$1`, id.String()).Scan(&stay, &n); err != nil {
				t.Fatal(err)
			}
			if stay != expiry || n != 1 {
				t.Fatalf("offer bearer stay=%d timers=%d, want saved expiry=%d once", stay, n, expiry)
			}
		})
	}
}
