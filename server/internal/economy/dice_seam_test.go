package economy

// fix/forlusttarning-injicerbar (2026-07-31): proves the Dice seam actually
// controls DeliveryHandler/TradeReturnHandler's loss roll, in both
// directions. Without this, "fixing" the flaky stale-cap test could have
// meant quietly setting tradeRiskPct = 0 and nobody would notice the
// difference — the seam has to be shown driving real outcomes, not just
// compiling.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
)

// TestDeliveryHandler_DiceSeamControlsOutcome is bevisplan step 7 for the
// outbound leg (trade.go): an alwaysLosesDice must lose the cargo and write
// TradeLost; a neverLosesDice on an otherwise-identical external trade must
// deliver it.
func TestDeliveryHandler_DiceSeamControlsOutcome(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID := mkTradeWorld(t, pool, ctx)
	ownerA := mkTradeOwner(t, pool, ctx)
	ownerB := mkTradeOwner(t, pool, ctx)
	origin := mkTradeSettlement(t, pool, ctx, worldID, ownerA, "Seam-Origin", 0)
	dest := mkTradeSettlement(t, pool, ctx, worldID, ownerB, "Seam-Dest", 1)

	t.Run("alwaysLosesDice loses the cargo and records TradeLost", func(t *testing.T) {
		h := NewDeliveryHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clock.NewTestClock(time.Now())))
		h.Dice = alwaysLosesDice()

		routeID := mkTradeRoute(t, pool, ctx, worldID, origin, dest)
		transportID := mkTradeTransport(t, pool, ctx, worldID, ownerA, origin, dest)
		payload := deliveryPayload(routeID, dest, transportID, 42)
		ev := events.ScheduledEvent{ID: time.Now().UnixNano(), WorldID: worldID, Payload: payload}
		if err := h.Handle(ctx, ev); err != nil {
			t.Fatalf("handle: %v", err)
		}

		var status string
		_ = pool.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1`, transportID).Scan(&status)
		if status != "lost" {
			t.Errorf("transport status = %q, want lost — alwaysLosesDice did not drive the outcome", status)
		}
		got := settlementGoodAmount(t, pool, ctx, dest, "silver")
		if got != 0 {
			t.Errorf("dest silver = %v, want 0 (cargo lost, nothing credited)", got)
		}
		var lostEvents int
		_ = pool.QueryRow(ctx,
			`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'TradeLost'`, dest,
		).Scan(&lostEvents)
		if lostEvents == 0 {
			t.Errorf("no TradeLost event recorded for dest %s", dest)
		}
	})

	t.Run("neverLosesDice delivers the cargo", func(t *testing.T) {
		h := NewDeliveryHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clock.NewTestClock(time.Now())))
		h.Dice = neverLosesDice()

		routeID := mkTradeRoute(t, pool, ctx, worldID, origin, dest)
		transportID := mkTradeTransport(t, pool, ctx, worldID, ownerA, origin, dest)
		pre := settlementGoodAmount(t, pool, ctx, dest, "silver")
		payload := deliveryPayload(routeID, dest, transportID, 42)
		ev := events.ScheduledEvent{ID: time.Now().UnixNano() + 1, WorldID: worldID, Payload: payload}
		if err := h.Handle(ctx, ev); err != nil {
			t.Fatalf("handle: %v", err)
		}

		var status string
		_ = pool.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1`, transportID).Scan(&status)
		if status != "delivered" {
			t.Errorf("transport status = %q, want delivered — neverLosesDice did not drive the outcome", status)
		}
		post := settlementGoodAmount(t, pool, ctx, dest, "silver")
		if post != pre+42 {
			t.Errorf("dest silver = %v, want %v (cargo delivered)", post, pre+42)
		}
	})
}

// TestTradeReturnHandler_DiceSeamControlsOutcome mirrors the above for the
// return leg (trade_return.go).
func TestTradeReturnHandler_DiceSeamControlsOutcome(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID := mkTradeWorld(t, pool, ctx)
	ownerSeller := mkTradeOwner(t, pool, ctx)
	ownerBuyer := mkTradeOwner(t, pool, ctx)
	buyer := mkTradeSettlement(t, pool, ctx, worldID, ownerBuyer, "SeamRet-Buyer", 0)
	seller := mkTradeSettlement(t, pool, ctx, worldID, ownerSeller, "SeamRet-Seller", 1)

	t.Run("alwaysLosesDice loses the cargo and records TradeLost", func(t *testing.T) {
		h := NewTradeReturnHandler(pool, events.NewStore(pool), nil)
		h.Dice = alwaysLosesDice()

		transportID := mkTradeTransport(t, pool, ctx, worldID, ownerSeller, seller, buyer)
		messengerID := mkReturnMessenger(t, pool, ctx, worldID, ownerBuyer, buyer, seller)
		payload := returnPayload(buyer, messengerID, transportID, "silver", 17)
		ev := events.ScheduledEvent{ID: time.Now().UnixNano(), WorldID: worldID, Payload: payload}
		if err := h.Handle(ctx, ev); err != nil {
			t.Fatalf("handle: %v", err)
		}

		var status string
		_ = pool.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1`, transportID).Scan(&status)
		if status != "lost" {
			t.Errorf("transport status = %q, want lost — alwaysLosesDice did not drive the outcome", status)
		}
		got := settlementGoodAmount(t, pool, ctx, buyer, "silver")
		if got != 0 {
			t.Errorf("buyer silver = %v, want 0 (cargo lost, nothing credited)", got)
		}
		var lostEvents int
		_ = pool.QueryRow(ctx,
			`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'TradeLost'`, buyer,
		).Scan(&lostEvents)
		if lostEvents == 0 {
			t.Errorf("no TradeLost event recorded for buyer %s", buyer)
		}
	})

	t.Run("neverLosesDice delivers the cargo", func(t *testing.T) {
		h := NewTradeReturnHandler(pool, events.NewStore(pool), nil)
		h.Dice = neverLosesDice()

		transportID := mkTradeTransport(t, pool, ctx, worldID, ownerSeller, seller, buyer)
		messengerID := mkReturnMessenger(t, pool, ctx, worldID, ownerBuyer, buyer, seller)
		pre := settlementGoodAmount(t, pool, ctx, buyer, "silver")
		payload := returnPayload(buyer, messengerID, transportID, "silver", 17)
		ev := events.ScheduledEvent{ID: time.Now().UnixNano() + 1, WorldID: worldID, Payload: payload}
		if err := h.Handle(ctx, ev); err != nil {
			t.Fatalf("handle: %v", err)
		}

		var status string
		_ = pool.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1`, transportID).Scan(&status)
		if status != "delivered" {
			t.Errorf("transport status = %q, want delivered — neverLosesDice did not drive the outcome", status)
		}
		post := settlementGoodAmount(t, pool, ctx, buyer, "silver")
		if post != pre+17 {
			t.Errorf("buyer silver = %v, want %v (cargo delivered)", post, pre+17)
		}
	})
}
