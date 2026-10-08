package economy

// R4/R6 (megaron_plan_sjohandel_mellan_spelare.md): when a negotiated trade's
// leg 1 bound a real ship, leg 2 (the chained ThenReturn) sails home carrying
// the counterpart good/silver on the SAME ship — owned by the ship's real
// owner (the trade's initiator), never by whoever's city leg 2 departs from
// (the land-caravan ownership convention ThenReturn's own "owner_id" field
// otherwise encodes). Exactly one hemresa is dispatched (no empty
// ship_return alongside it), and the ship is released at the initiator's
// settlement when leg 2 lands. A pre-existing land trade (no ship bound)
// keeps building leg 2 exactly as before (R6).
//
// These drive DeliveryHandler/TradeReturnHandler directly with a synthetic
// ScheduledTradeDelivery/ScheduledTradeReturn payload — the same pattern
// trade_naval_preexisting_test.go and trade_delivery_stale_cap_test.go use —
// rather than going through the HTTP TradeAccept handler, since the ship
// binding itself (R1-R3) is proven at the handlers layer
// (province_trade_naval_test.go's sibling in api/handlers).

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type navalPlayerTradeFixture struct {
	pool                       *pgxpool.Pool
	worldID                    uuid.UUID
	initiator, counterparty    uuid.UUID
	initiatorCity, counterCity uuid.UUID
	shipID                     uuid.UUID
}

// newNavalPlayerTradeFixture builds two players, each with one settlement,
// and a ship already bound (status='freighting', settlement_id still the
// initiator's city — BindShip never touches settlement_id) at the
// initiator's city, standing in for a completed R3 accept.
func newNavalPlayerTradeFixture(t *testing.T, pool *pgxpool.Pool) navalPlayerTradeFixture {
	t.Helper()
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-1b-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	var initiator, counterparty uuid.UUID
	_ = pool.QueryRow(ctx, `INSERT INTO players (username, password_hash) VALUES ($1,'x') RETURNING id`,
		"initiator-"+uuid.New().String()).Scan(&initiator)
	_ = pool.QueryRow(ctx, `INSERT INTO players (username, password_hash) VALUES ($1,'x') RETURNING id`,
		"counterparty-"+uuid.New().String()).Scan(&counterparty)

	var prov1, initiatorCity uuid.UUID
	_ = pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1,0,0,'plains') RETURNING id`,
		worldID).Scan(&prov1)
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1,$2,'InitiatorCity','achaean',$3,'capital',true,'active',5000) RETURNING id`,
		worldID, prov1, initiator,
	).Scan(&initiatorCity); err != nil {
		t.Fatalf("create initiator city: %v", err)
	}

	var prov2, counterCity uuid.UUID
	_ = pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1,5,0,'plains') RETURNING id`,
		worldID).Scan(&prov2)
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1,$2,'CounterCity','achaean',$3,'capital',true,'active',5000) RETURNING id`,
		worldID, prov2, counterparty,
	).Scan(&counterCity); err != nil {
		t.Fatalf("create counterparty city: %v", err)
	}

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1,$2,'merchantman','naval',1,10,'freighting',$3) RETURNING id`,
		worldID, initiator, initiatorCity,
	).Scan(&shipID); err != nil {
		t.Fatalf("create bound ship: %v", err)
	}

	for _, s := range []uuid.UUID{initiatorCity, counterCity} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
			 VALUES ($1,'silver',0,0,1000000,0)`, s,
		); err != nil {
			t.Fatalf("seed silver: %v", err)
		}
	}

	return navalPlayerTradeFixture{
		pool: pool, worldID: worldID,
		initiator: initiator, counterparty: counterparty,
		initiatorCity: initiatorCity, counterCity: counterCity,
		shipID: shipID,
	}
}

// leg1Transport inserts leg 1's transport row exactly as TradeAccept (R3)
// would have: naval, ship_unit_id set, origin=initiator's city, dest=
// counterparty's city.
func (f navalPlayerTradeFixture) leg1Transport(t *testing.T) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO transports
		   (world_id, owner_id, kind, origin_id, dest_id, category,
		    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, status, interceptable, ship_unit_id)
		 VALUES ($1,$2,'trade',$3,$4,'naval',0,0,5,0, now(), now(), 1, 'in_transit', true, $5)
		 RETURNING id`,
		f.worldID, f.initiator, f.initiatorCity, f.counterCity, f.shipID,
	).Scan(&id); err != nil {
		t.Fatalf("create leg1 transport: %v", err)
	}
	return id
}

// TestDeliveryHandler_NavalThenReturn_Leg2SailsSameShipHomeToInitiator is R4's
// core contract: leg 1 bound a ship, ThenReturn exists -> leg 2 is naval, same
// ship, owner = the ship's real owner (initiator) even though ThenReturn's own
// "owner_id" field says the counterparty (the land-caravan convention) — and
// no separate empty ship_return leg is dispatched alongside it.
func TestDeliveryHandler_NavalThenReturn_Leg2SailsSameShipHomeToInitiator(t *testing.T) {
	pool := testPool(t)
	f := newNavalPlayerTradeFixture(t, pool)
	ctx := context.Background()
	leg1ID := f.leg1Transport(t)

	h := NewDeliveryHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clock.NewTestClock(time.Now())))
	payload, _ := json.Marshal(map[string]any{
		"destination_id":     f.counterCity,
		"good_key":           "silver",
		"quantity":           100.0,
		"delivered_quantity": 100.0,
		"transport_id":       leg1ID.String(),
		"then_return": map[string]any{
			"destination_id": f.initiatorCity.String(),
			"good_key":       "copper",
			"quantity":       20.0,
			"messenger_id":   uuid.New().String(),
			"travel_mins":    60.0,
			// Deliberately the ACCEPTOR (counterparty) — the land-caravan
			// convention this field otherwise encodes. R4 must override it
			// with the ship's real owner when a ship is bound.
			"owner_id": f.counterparty.String(),
			"origin_q": 5, "origin_r": 0,
			"dest_q": 0, "dest_r": 0,
		},
	})
	if err := h.Handle(ctx, events.ScheduledEvent{ID: time.Now().UnixNano(), WorldID: f.worldID, Payload: payload}); err != nil {
		t.Fatalf("delivery handle: %v", err)
	}

	var leg2Kind, leg2Category string
	var leg2Owner uuid.UUID
	var leg2ShipUnitID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT kind, category, owner_id, ship_unit_id FROM transports
		 WHERE world_id = $1 AND kind = 'trade_return' ORDER BY created_at DESC LIMIT 1`,
		f.worldID,
	).Scan(&leg2Kind, &leg2Category, &leg2Owner, &leg2ShipUnitID); err != nil {
		t.Fatalf("no trade_return leg found: %v", err)
	}
	if leg2Category != "naval" {
		t.Errorf("leg2 category = %q, want naval", leg2Category)
	}
	if leg2ShipUnitID == nil || *leg2ShipUnitID != f.shipID {
		t.Errorf("leg2 ship_unit_id = %v, want %s (same ship as leg 1)", leg2ShipUnitID, f.shipID)
	}
	if leg2Owner != f.initiator {
		t.Errorf("leg2 owner = %s, want %s (the ship's real owner, the initiator — not ThenReturn's own owner_id=%s)",
			leg2Owner, f.initiator, f.counterparty)
	}

	// Exactly one hemresa: no separate empty ship_return leg alongside leg 2
	// (R4 — dispatchShipReturnLeg must be skipped when ThenReturn exists).
	var shipReturnCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM transports WHERE world_id = $1 AND kind = 'ship_return'`, f.worldID,
	).Scan(&shipReturnCount); err != nil {
		t.Fatalf("count ship_return legs: %v", err)
	}
	if shipReturnCount != 0 {
		t.Errorf("ship_return legs = %d, want 0 (leg 2 IS the ship's way home)", shipReturnCount)
	}

	// The ship is still freighting — the round trip isn't over until leg 2 lands.
	var shipStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, f.shipID).Scan(&shipStatus); err != nil {
		t.Fatalf("read ship status: %v", err)
	}
	if shipStatus != "freighting" {
		t.Errorf("ship status right after leg1 delivery = %q, want still freighting", shipStatus)
	}
}

// TestTradeReturnHandler_NavalLeg2Arrival_ReleasesShipAtInitiatorsCity is R4's
// other half: when leg 2 lands, the counterpart good is credited as always
// AND the ship is released (garrison) at the initiator's own settlement.
func TestTradeReturnHandler_NavalLeg2Arrival_ReleasesShipAtInitiatorsCity(t *testing.T) {
	pool := testPool(t)
	f := newNavalPlayerTradeFixture(t, pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1,'copper',0,0,1000000,0)`, f.initiatorCity,
	); err != nil {
		t.Fatalf("seed initiator copper: %v", err)
	}

	// Leg 2 exactly as DeliveryHandler now builds it: naval, same ship, sails
	// counterparty -> initiator, owner already the initiator (R4).
	var leg2ID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO transports
		   (world_id, owner_id, kind, origin_id, dest_id, category,
		    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, status, interceptable, ship_unit_id)
		 VALUES ($1,$2,'trade_return',$3,$4,'naval',5,0,0,0, now(), now(), 1, 'in_transit', true, $5)
		 RETURNING id`,
		f.worldID, f.initiator, f.counterCity, f.initiatorCity, f.shipID,
	).Scan(&leg2ID); err != nil {
		t.Fatalf("create leg2 transport: %v", err)
	}

	messengerID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, trade_offer,
		                         status, hex_q, hex_r, sent_at, arrives_at)
		 VALUES ($1,$2,$3,$4,'x','{"kind":"buy","status":"accepted"}'::jsonb,'delivered',0,0,now(),now())`,
		f.worldID, f.initiator, f.initiatorCity, f.counterCity,
	); err != nil {
		t.Fatalf("seed messenger row: %v", err)
	}
	// messengerID must match the row's own id for the handler's idempotency
	// read — simplest is to read it back.
	_ = pool.QueryRow(ctx, `SELECT id FROM messengers WHERE world_id=$1 ORDER BY sent_at DESC LIMIT 1`, f.worldID).Scan(&messengerID)

	h := NewTradeReturnHandler(pool, events.NewStore(pool), nil)
	payload, _ := json.Marshal(map[string]any{
		"destination_id": f.initiatorCity,
		"good_key":       "copper",
		"quantity":       20.0,
		"messenger_id":   messengerID,
		"transport_id":   leg2ID,
	})
	if err := h.Handle(ctx, events.ScheduledEvent{ID: time.Now().UnixNano(), WorldID: f.worldID, Payload: payload}); err != nil {
		t.Fatalf("trade return handle: %v", err)
	}

	var copper float64
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(settled(amount, rate, calc_tick), 0) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'copper'`,
		f.initiatorCity,
	).Scan(&copper); err != nil {
		t.Fatalf("read initiator copper: %v", err)
	}
	if copper != 20 {
		t.Errorf("initiator copper = %v, want 20", copper)
	}

	var shipStatus string
	var shipSettlement *uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT status, settlement_id FROM units WHERE id = $1`, f.shipID).
		Scan(&shipStatus, &shipSettlement); err != nil {
		t.Fatalf("read final ship state: %v", err)
	}
	if shipStatus != "garrison" {
		t.Errorf("final ship status = %q, want garrison", shipStatus)
	}
	if shipSettlement == nil || *shipSettlement != f.initiatorCity {
		t.Errorf("final ship settlement = %v, want initiator's city %s", shipSettlement, f.initiatorCity)
	}
}

// TestDeliveryHandler_LandThenReturn_UnaffectedByShipLogic is R6: a
// pre-existing (or simply non-naval) negotiated trade with no ship bound on
// leg 1 must build leg 2 exactly as before — land, owner = ThenReturn's own
// "owner_id" (the acceptor) — R4's ship-owner override must never fire when
// there is no ship.
func TestDeliveryHandler_LandThenReturn_UnaffectedByShipLogic(t *testing.T) {
	pool := testPool(t)
	f := newNavalPlayerTradeFixture(t, pool)
	ctx := context.Background()

	// Leg 1 with NO ship bound — the ordinary land case.
	var leg1ID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO transports
		   (world_id, owner_id, kind, origin_id, dest_id, category,
		    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, status, interceptable)
		 VALUES ($1,$2,'trade',$3,$4,'land',0,0,5,0, now(), now(), 1, 'in_transit', true)
		 RETURNING id`,
		f.worldID, f.initiator, f.initiatorCity, f.counterCity,
	).Scan(&leg1ID); err != nil {
		t.Fatalf("create land leg1 transport: %v", err)
	}

	h := NewDeliveryHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clock.NewTestClock(time.Now())))
	payload, _ := json.Marshal(map[string]any{
		"destination_id":     f.counterCity,
		"good_key":           "silver",
		"quantity":           100.0,
		"delivered_quantity": 100.0,
		"transport_id":       leg1ID.String(),
		"then_return": map[string]any{
			"destination_id": f.initiatorCity.String(),
			"good_key":       "copper",
			"quantity":       20.0,
			"messenger_id":   uuid.New().String(),
			"travel_mins":    60.0,
			"owner_id":       f.counterparty.String(), // land rule: leg 2's own sender
			"origin_q":       5, "origin_r": 0,
			"dest_q": 0, "dest_r": 0,
		},
	})
	if err := h.Handle(ctx, events.ScheduledEvent{ID: time.Now().UnixNano(), WorldID: f.worldID, Payload: payload}); err != nil {
		t.Fatalf("delivery handle: %v", err)
	}

	var leg2Category string
	var leg2Owner uuid.UUID
	var leg2ShipUnitID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT category, owner_id, ship_unit_id FROM transports
		 WHERE world_id = $1 AND kind = 'trade_return' ORDER BY created_at DESC LIMIT 1`,
		f.worldID,
	).Scan(&leg2Category, &leg2Owner, &leg2ShipUnitID); err != nil {
		t.Fatalf("no trade_return leg found: %v", err)
	}
	if leg2Category != "land" {
		t.Errorf("leg2 category = %q, want land (R6: unaffected by ship logic)", leg2Category)
	}
	if leg2ShipUnitID != nil {
		t.Errorf("leg2 ship_unit_id = %v, want nil", leg2ShipUnitID)
	}
	if leg2Owner != f.counterparty {
		t.Errorf("leg2 owner = %s, want %s (ThenReturn's own owner_id, unchanged)", leg2Owner, f.counterparty)
	}
}
