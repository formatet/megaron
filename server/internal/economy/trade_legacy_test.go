package economy

// Legacy (pre-migration-160) deliveries already sitting in the queue: no
// journey/travel_ticks in ThenReturn, transports with journey/departed_tick NULL.
// DeliveryHandler.Handle now fails hard instead of best-effort, so these lock
// what still goes through and what rolls back.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func legacyDeliveryHandler(f navalPlayerTradeFixture) *DeliveryHandler {
	h := NewDeliveryHandler(f.pool, events.NewStore(f.pool), nil, events.NewScheduler(f.pool, clock.NewTestClock(time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC))))
	h.Dice = neverLosesDice()
	return h
}

func legacyRolledBack(t *testing.T, f navalPlayerTradeFixture, leg uuid.UUID, evtID int64) {
	t.Helper()
	ctx := context.Background()
	var status string
	if err := f.pool.QueryRow(ctx, `SELECT status FROM transports WHERE id=$1`, leg).Scan(&status); err != nil || status != "in_transit" {
		t.Fatalf("leg status after failure %q %v, want in_transit", status, err)
	}
	var n int
	for _, q := range []string{
		`SELECT count(*) FROM processed_deliveries WHERE event_id=$1`,
	} {
		if err := f.pool.QueryRow(ctx, q, evtID).Scan(&n); err != nil || n != 0 {
			t.Fatalf("claim row left behind (%s): %d %v", q, n, err)
		}
	}
	for _, q := range []string{
		`SELECT count(*) FROM transports WHERE world_id=$1 AND kind IN ('trade_return','ship_return')`,
		`SELECT count(*) FROM scheduled_events WHERE world_id=$1`,
		`SELECT count(*) FROM events WHERE world_id=$1`,
	} {
		if err := f.pool.QueryRow(ctx, q, f.worldID).Scan(&n); err != nil || n != 0 {
			t.Fatalf("rows left behind (%s): %d %v", q, n, err)
		}
	}
	var amount float64
	if err := f.pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, f.counterCity).Scan(&amount); err != nil || amount != 0 {
		t.Fatalf("silver credited despite failure: %v %v", amount, err)
	}
}

// 1. Legacy silver leg: ThenReturn without journey. Must still deliver and
// build the return caravan from travel_mins alone.
func TestDeliveryLegacySilverLegWithoutJourneyStillDelivers(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newNavalPlayerTradeFixture(t, pool)
	leg := f.leg1Transport(t)
	if _, err := pool.Exec(ctx, `UPDATE transports SET category='land',ship_unit_id=NULL WHERE id=$1`, leg); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=17 WHERE id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	h := legacyDeliveryHandler(f)
	// travel_mins 150 -> round(2.5) = 3 ticks.
	raw, _ := json.Marshal(map[string]any{
		"transport_id": leg, "destination_id": f.counterCity, "good_key": "silver", "quantity": 100,
		"then_return": map[string]any{
			"destination_id": f.initiatorCity, "good_key": "copper", "quantity": 20,
			"messenger_id": uuid.New().String(), "travel_mins": 150, "owner_id": f.counterparty,
			"origin_q": 5, "origin_r": 0, "dest_q": 0, "dest_r": 0,
		},
	})
	evt := events.ScheduledEvent{ID: time.Now().UnixNano(), WorldID: f.worldID, Payload: raw}
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("legacy silver leg must deliver: %v", err)
	}
	var amount float64
	if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, f.counterCity).Scan(&amount); err != nil || amount != 100 {
		t.Fatalf("silver credit %v %v", amount, err)
	}
	var retID uuid.UUID
	var journeyNull, departedNull bool
	var due int
	var category string
	if err := pool.QueryRow(ctx, `SELECT id,journey IS NULL,departed_tick IS NULL,due_tick,category FROM transports WHERE world_id=$1 AND kind='trade_return'`, f.worldID).Scan(&retID, &journeyNull, &departedNull, &due, &category); err != nil {
		t.Fatal(err)
	}
	if !journeyNull || !departedNull || due != 17+3 || category != "land" {
		t.Fatalf("legacy return: journeyNull=%v departedNull=%v due=%d category=%s", journeyNull, departedNull, due, category)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='TradeReturn' AND payload->>'transport_id'=$2`, f.worldID, retID.String()).Scan(&n); err != nil || n != 1 {
		t.Fatalf("scheduled TradeReturn %d %v", n, err)
	}
	var st string
	if err := pool.QueryRow(ctx, `SELECT status FROM transports WHERE id=$1`, leg).Scan(&st); err != nil || st != "delivered" {
		t.Fatalf("leg1 status %s %v", st, err)
	}
}

// 2. Legacy naval leg: ship bound, journey NULL, no ThenReturn. The ship's
// empty voyage home is planned from the map via province.PlanTradeJourney. The
// test DB has no map tiles, so there is no sea route home.
// ACTUAL BEHAVIOUR (locked): Handle returns an error and the WHOLE delivery
// rolls back (silver not credited, leg stays in_transit, claim row removed).
// The worker retries; three failures dead-letter it, so the goods stay stuck.
func TestDeliveryLegacyNavalWithoutJourneyNoSeaRouteHome(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newNavalPlayerTradeFixture(t, pool)
	leg := f.leg1Transport(t) // naval, ship bound, journey/departed_tick NULL
	h := legacyDeliveryHandler(f)
	raw, _ := json.Marshal(map[string]any{"transport_id": leg, "destination_id": f.counterCity, "good_key": "silver", "quantity": 100})
	evt := events.ScheduledEvent{ID: time.Now().UnixNano(), WorldID: f.worldID, Payload: raw}
	err := h.Handle(ctx, evt)
	if err == nil {
		t.Fatal("expected error: no sea route home in a tile-less world")
	}
	t.Logf("actual error: %v", err)
	legacyRolledBack(t, f, leg, evt.ID)
	// Retrying does not help: same error each time.
	if err2 := h.Handle(ctx, evt); err2 == nil {
		t.Fatal("retry unexpectedly succeeded")
	}
	legacyRolledBack(t, f, leg, evt.ID)
}

// 3. ThenReturn present but without destination_id (corrupt/old).
// ACTUAL BEHAVIOUR (locked): Handle returns "trade return destination missing"
// and the whole delivery rolls back, so the silver is not credited.
func TestDeliveryThenReturnWithoutDestinationRollsBack(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newNavalPlayerTradeFixture(t, pool)
	leg := f.leg1Transport(t)
	if _, err := pool.Exec(ctx, `UPDATE transports SET category='land',ship_unit_id=NULL WHERE id=$1`, leg); err != nil {
		t.Fatal(err)
	}
	h := legacyDeliveryHandler(f)
	raw, _ := json.Marshal(map[string]any{
		"transport_id": leg, "destination_id": f.counterCity, "good_key": "silver", "quantity": 100,
		"then_return": map[string]any{"good_key": "copper", "quantity": 20, "travel_mins": 60, "owner_id": f.counterparty},
	})
	evt := events.ScheduledEvent{ID: time.Now().UnixNano(), WorldID: f.worldID, Payload: raw}
	err := h.Handle(ctx, evt)
	if err == nil {
		t.Fatal("expected error for ThenReturn without destination_id")
	}
	t.Logf("actual error: %v", err)
	if err.Error() != "trade return destination missing" {
		t.Fatalf("unexpected error %q", err)
	}
	legacyRolledBack(t, f, leg, evt.ID)
}
