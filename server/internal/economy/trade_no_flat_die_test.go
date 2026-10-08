package economy

// Slice T (megaron_transportrisk.md, Timothy 2026-10-08): the flat 5 % loss
// die at delivery is gone. Delivery between two DIFFERENT Wanaxes — the case
// that used to roll — must now always credit. Sea risk lives per sea hex on
// the ship (combat.StormScanHandler), never at the destination's door.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
)

// deliveriesPerArm is large enough that the old 5 % die would lose at least
// one caravan with overwhelming probability (0.95^200 ≈ 3.5e-5).
const deliveriesPerArm = 200

func TestDeliveryHandler_ExternalTradeNeverLostAtDoor(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID := mkTradeWorld(t, pool, ctx)
	ownerA := mkTradeOwner(t, pool, ctx)
	ownerB := mkTradeOwner(t, pool, ctx)
	origin := mkTradeSettlement(t, pool, ctx, worldID, ownerA, "NoDie-Origin", 0)
	dest := mkTradeSettlement(t, pool, ctx, worldID, ownerB, "NoDie-Dest", 1)
	h := NewDeliveryHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clock.NewTestClock(time.Now())))

	for i := 0; i < deliveriesPerArm; i++ {
		routeID := mkTradeRoute(t, pool, ctx, worldID, origin, dest)
		transportID := mkTradeTransport(t, pool, ctx, worldID, ownerA, origin, dest)
		ev := events.ScheduledEvent{ID: time.Now().UnixNano() + int64(i), WorldID: worldID, Payload: deliveryPayload(routeID, dest, transportID, 1)}
		if err := h.Handle(ctx, ev); err != nil {
			t.Fatalf("handle %d: %v", i, err)
		}
		var status string
		_ = pool.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1`, transportID).Scan(&status)
		if status != "delivered" {
			t.Fatalf("delivery %d: transport status = %q, want delivered — a delivery die still rolls", i, status)
		}
	}
	var lost int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'TradeLost'`, dest).Scan(&lost)
	if lost != 0 {
		t.Fatalf("TradeLost events = %d, want 0", lost)
	}
}

func TestTradeReturnHandler_ExternalTradeNeverLostAtDoor(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID := mkTradeWorld(t, pool, ctx)
	ownerSeller := mkTradeOwner(t, pool, ctx)
	ownerBuyer := mkTradeOwner(t, pool, ctx)
	buyer := mkTradeSettlement(t, pool, ctx, worldID, ownerBuyer, "NoDieRet-Buyer", 0)
	seller := mkTradeSettlement(t, pool, ctx, worldID, ownerSeller, "NoDieRet-Seller", 1)
	h := NewTradeReturnHandler(pool, events.NewStore(pool), nil)

	for i := 0; i < deliveriesPerArm; i++ {
		transportID := mkTradeTransport(t, pool, ctx, worldID, ownerSeller, seller, buyer)
		messengerID := mkReturnMessenger(t, pool, ctx, worldID, ownerBuyer, buyer, seller)
		ev := events.ScheduledEvent{ID: time.Now().UnixNano() + int64(i), WorldID: worldID, Payload: returnPayload(buyer, messengerID, transportID, "silver", 1)}
		if err := h.Handle(ctx, ev); err != nil {
			t.Fatalf("handle %d: %v", i, err)
		}
		var status string
		_ = pool.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1`, transportID).Scan(&status)
		if status != "delivered" {
			t.Fatalf("return %d: transport status = %q, want delivered — a return die still rolls", i, status)
		}
	}
}
