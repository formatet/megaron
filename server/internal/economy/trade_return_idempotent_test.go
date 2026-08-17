package economy

// G2 idempotency regression for TradeReturnHandler (ScheduledTradeReturn).
//
// trade_return.go's own comment: "Idempotency: check messenger offer not
// already returned." — Handle reads messengers.trade_offer->>'status' and
// no-ops if it already reads 'returned', then flips it to 'returned' itself
// as the terminal step. This is a DB integration test (real Postgres, gated
// by DATABASE_URL) proving that guard: it drives the SAME ScheduledTradeReturn
// event through Handle twice (using the same fixture helpers as
// trade_return_risk_test.go, dice pinned to neverLosesDice so the risk roll
// can't turn this into an intermittent failure) and asserts the buyer's good
// is credited exactly once.

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestTradeReturnHandler_ReplayIsIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID := mkTradeWorld(t, pool, ctx)
	ownerSeller := mkTradeOwner(t, pool, ctx)
	ownerBuyer := mkTradeOwner(t, pool, ctx)
	buyer := mkTradeSettlement(t, pool, ctx, worldID, ownerBuyer, "RetIdem-Buyer", 0)
	seller := mkTradeSettlement(t, pool, ctx, worldID, ownerSeller, "RetIdem-Seller", 1)

	messengerID := mkReturnMessenger(t, pool, ctx, worldID, ownerSeller, seller, buyer)

	h := NewTradeReturnHandler(pool, events.NewStore(pool), nil)
	h.Dice = neverLosesDice() // subject is the messenger-status guard, not the loss die

	const qty = 25.0
	payload := returnPayload(buyer, messengerID, uuid.UUID{}, "tin", qty)
	evt := events.ScheduledEvent{ID: 1, WorldID: worldID, Payload: payload}

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (first run): %v", err)
	}

	tinAfterFirst := settlementGoodAmount(t, pool, ctx, buyer, "tin")
	if tinAfterFirst != qty {
		t.Fatalf("tin after first run = %v, want %v — fixture does not exercise the handler", tinAfterFirst, qty)
	}

	var statusAfterFirst string
	if err := pool.QueryRow(ctx,
		`SELECT trade_offer->>'status' FROM messengers WHERE id = $1`, messengerID,
	).Scan(&statusAfterFirst); err != nil {
		t.Fatalf("read offer status after first run: %v", err)
	}
	if statusAfterFirst != "returned" {
		t.Fatalf("offer status after first run = %q, want returned", statusAfterFirst)
	}

	// Replay the SAME event.
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (replay): %v", err)
	}

	tinAfterReplay := settlementGoodAmount(t, pool, ctx, buyer, "tin")
	if tinAfterReplay != qty {
		t.Errorf("tin after replay = %v, want still %v (a non-idempotent handler would double-credit to %v)", tinAfterReplay, qty, 2*qty)
	}
}
