package economy

// G2 idempotency regression for DeliveryHandler (ScheduledTradeDelivery).
//
// trade.go's own doc comment on the claim insert: "Exactly-once claim: the
// worker marks the event done in a separate statement after this tx commits,
// so a crash in between would re-run this handler ... without this marker a
// retry would double-credit silver and double-schedule the goods return."
// This is a DB integration test (real Postgres, gated by DATABASE_URL)
// proving that claim: it drives the SAME ScheduledTradeDelivery event through
// Handle twice (no trade_route_id / no transport_id — the "legacy event"
// shape trade_delivery_stale_cap_test.go also uses, so the interception veto
// and route-resolved check are out of scope) and asserts the destination
// settlement's good is credited exactly once.

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestDeliveryHandler_ReplayIsIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 500) RETURNING id`,
		"trade-delivery-idem-"+uuid.NewString(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var owner uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"trade-delivery-owner-"+uuid.NewString(), uuid.NewString()+"@test.invalid",
	).Scan(&owner); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var prov, buyer uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&prov); err != nil {
		t.Fatalf("create province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Buyer', 'achaean', $3, 'capital', true, 'active', 5000) RETURNING id`,
		worldID, prov, owner,
	).Scan(&buyer); err != nil {
		t.Fatalf("create buyer settlement: %v", err)
	}

	h := NewDeliveryHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clock.NewTestClock(time.Now())))
	h.Dice = neverLosesDice() // subject is the delivery claim, not the loss die

	payload, err := json.Marshal(map[string]any{
		"destination_id":     buyer,
		"good_key":           "cedar",
		"quantity":           40.0,
		"delivered_quantity": 40.0,
		// trade_route_id and transport_id both omitted (zero UUID) — "legacy
		// event": skips the route-resolved check and the interception veto,
		// same shape trade_delivery_stale_cap_test.go uses.
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	// processed_deliveries keys ONLY on event_id (no world/settlement scoping,
	// and has no per-test cleanup) — derive the id from this test's own random
	// world so a replay within THIS run is a genuine same-event replay without
	// colliding with any other test/run that also seeds processed_deliveries
	// (mirrors logistics_idempotent_test.go's fixedEventID derivation).
	fixedEventID := int64(binary.BigEndian.Uint64(worldID[:8]) &^ (1 << 63))
	evt := events.ScheduledEvent{ID: fixedEventID, WorldID: worldID, Payload: payload}

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (first run): %v", err)
	}

	var cedarAfterFirst float64
	if err := pool.QueryRow(ctx,
		`SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'cedar'`,
		buyer,
	).Scan(&cedarAfterFirst); err != nil {
		t.Fatalf("read cedar after first run: %v", err)
	}
	if cedarAfterFirst != 40 {
		t.Fatalf("cedar after first run = %v, want 40 — fixture does not exercise the handler", cedarAfterFirst)
	}

	// Replay the SAME event.
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (replay): %v", err)
	}

	var cedarAfterReplay float64
	if err := pool.QueryRow(ctx,
		`SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'cedar'`,
		buyer,
	).Scan(&cedarAfterReplay); err != nil {
		t.Fatalf("read cedar after replay: %v", err)
	}
	if cedarAfterReplay != 40 {
		t.Errorf("cedar after replay = %v, want still 40 (a non-idempotent handler would double-credit to 80)", cedarAfterReplay)
	}

	var claimCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM processed_deliveries WHERE event_id = $1`, fixedEventID).Scan(&claimCount); err != nil {
		t.Fatalf("count processed_deliveries claim rows: %v", err)
	}
	if claimCount != 1 {
		t.Errorf("processed_deliveries rows for event %d = %d, want exactly 1", fixedEventID, claimCount)
	}
}
