package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/transport"
	"github.com/google/uuid"
)

// The only connection is river -> ford -> river -> river. There is no sea
// or alternative land path. This exercises the HTTP contract and real
// scheduled delivery/return handlers, not just the pathfinder.
func TestTrade_RiverFordRoundTrip(t *testing.T) {
	f := setupTradeNavalFixture(t)
	ctx := context.Background()
	origin, originProvince := f.settlement(t, "River Origin", 0, true, true)
	dest, _ := f.settlement(t, "River Destination", 5, true, false)
	for q, terrain := range []string{"river", "river_ford", "river", "river"} {
		f.mapTile(t, q+1, 0, terrain)
	}
	ship := f.ship(t, origin, "merchantman")
	if _, err := f.pool.Exec(ctx, `UPDATE settlement_goods SET amount=1000 WHERE settlement_id=$1 AND good_key='grain'`, dest); err != nil {
		t.Fatal(err)
	}
	code, resp := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+originProvince.String()+"/trade", map[string]any{"destination_id": dest.String(), "good_key": "grain", "quantity": 10.0})
	if code != 201 {
		t.Fatalf("river/ford trade HTTP=%d, want201: %v", code, resp)
	}
	if got := f.transportCategory(t, f.worldID, origin, dest); got != "naval" {
		t.Fatalf("river trade category=%s, want naval", got)
	}
	var journeyRaw []byte
	var bound *uuid.UUID
	var transportID uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT id,journey,ship_unit_id FROM transports WHERE world_id=$1 AND origin_id=$2 AND dest_id=$3`, f.worldID, origin, dest).Scan(&transportID, &journeyRaw, &bound); err != nil {
		t.Fatal(err)
	}
	if bound == nil || *bound != ship {
		t.Fatalf("physical river cargo not bound to its ship: %v", bound)
	}
	var journey province.TradeJourney
	if err := json.Unmarshal(journeyRaw, &journey); err != nil {
		t.Fatal(err)
	}
	crossedFord := false
	for _, p := range journey.Path {
		if p.Q == 2 && p.R == 0 {
			crossedFord = true
		}
	}
	if !crossedFord || journey.TravelTicks <= 0 {
		t.Fatalf("frozen voyage skips ford or has no duration: %+v", journey)
	}
	checkShip := func(want string, home bool) {
		t.Helper()
		var status string
		var sid *uuid.UUID
		if err := f.pool.QueryRow(ctx, `SELECT status,settlement_id FROM units WHERE id=$1`, ship).Scan(&status, &sid); err != nil {
			t.Fatal(err)
		}
		if status != want {
			t.Fatalf("ship status=%s, want%s", status, want)
		}
		if home && (sid == nil || *sid != origin) {
			t.Fatalf("returned ship home=%v, want%s", sid, origin)
		}
	}
	checkShip("freighting", false)
	// Invoke the saved due events at their tick boundary. This is deterministic
	// worker-handler integration; it does not wait for the wall-clock poller.
	advanceDue := func(ev events.ScheduledEvent) {
		t.Helper()
		var due int
		if err := f.pool.QueryRow(ctx, `SELECT due_tick FROM scheduled_events WHERE id=$1`, ev.ID).Scan(&due); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=$2 WHERE id=$1`, f.worldID, due); err != nil {
			t.Fatal(err)
		}
	}
	delivery := economy.NewDeliveryHandler(f.pool, events.NewStore(f.pool), nil, f.scheduler)
	ev := f.lastTradeDeliveryEvent(t)
	advanceDue(ev)
	for i := 0; i < 2; i++ {
		if err := delivery.Handle(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	var grain float64
	if err := f.pool.QueryRow(ctx, `SELECT settled(amount,rate,calc_tick) FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'`, dest).Scan(&grain); err != nil {
		t.Fatal(err)
	}
	if grain != 1010 {
		t.Fatalf("river delivery grain=%v, want1010 after replay", grain)
	}
	checkShip("freighting", false)
	var status string
	if err := f.pool.QueryRow(ctx, `SELECT status FROM transports WHERE id=$1`, transportID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "delivered" {
		t.Fatalf("outbound transport=%s", status)
	}
	arrival := transport.NewArrivalHandler(f.pool, nil)
	ret := f.lastTransportArrivalEvent(t)
	advanceDue(ret)
	for i := 0; i < 2; i++ {
		if err := arrival.Handle(ctx, ret); err != nil {
			t.Fatal(err)
		}
	}
	checkShip("garrison", true)
}

func TestTrade_RiverWithoutShipRefusesWithoutDebit(t *testing.T) {
	f := setupTradeNavalFixture(t)
	ctx := context.Background()
	origin, originProvince := f.settlement(t, "River Origin", 0, true, true)
	dest, _ := f.settlement(t, "River Destination", 5, true, false)
	for q, terrain := range []string{"river", "river_ford", "river", "river"} {
		f.mapTile(t, q+1, 0, terrain)
	}
	var before, after float64
	readGrain := func(dst *float64) {
		t.Helper()
		if err := f.pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'`, origin).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	readGrain(&before)
	code, resp := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+originProvince.String()+"/trade", map[string]any{"destination_id": dest.String(), "good_key": "grain", "quantity": 10.0})
	if code != 422 {
		t.Fatalf("river with no ship HTTP=%d, want422: %v", code, resp)
	}
	readGrain(&after)
	if before != after {
		t.Fatalf("refused river cargo debited: %v -> %v", before, after)
	}
	var movers int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM transports WHERE world_id=$1`, f.worldID).Scan(&movers); err != nil {
		t.Fatal(err)
	}
	if movers != 0 {
		t.Fatalf("refused river cargo created%d movers", movers)
	}
}
