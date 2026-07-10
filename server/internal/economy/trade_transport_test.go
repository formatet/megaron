package economy

// Trade legs are physical caravans (Del 3-fas-2): a shadow `transports` row carries
// map position + interceptability while the trade delivery event drives crediting.
// This verifies the interception veto — if the leg's caravan was intercepted en
// route, the delivery handler must NOT credit the destination. (The happy path hits
// the 5% storm-loss rand roll, so it is verified live on CT 126, not here.)

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
)

func TestTradeDelivery_InterceptedCaravanNotCredited(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-tradetx-%'`,
	); err != nil {
		t.Fatalf("archive leftover worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-tradetx-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	var owner uuid.UUID
	_ = pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"ttx-"+uuid.New().String(), "ttx-"+uuid.New().String()+"@test.invalid",
	).Scan(&owner)

	var prov, dest uuid.UUID
	_ = pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID).Scan(&prov)
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Buyer', 'achaean', $3, 'capital', true, 'active', 5000) RETURNING id`,
		worldID, prov, owner,
	).Scan(&dest); err != nil {
		t.Fatalf("create dest settlement: %v", err)
	}

	// Leg's physical caravan — already intercepted en route.
	var legID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO transports
		   (world_id, owner_id, kind, origin_id, dest_id, category,
		    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, status, interceptable)
		 VALUES ($1,$2,'trade',NULL,$3,'land',5,0,0,0, now(), now(), 1, 'intercepted', true)
		 RETURNING id`,
		worldID, owner, dest,
	).Scan(&legID); err != nil {
		t.Fatalf("create intercepted transport: %v", err)
	}

	h := NewDeliveryHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clock.NewTestClock(time.Now())))
	payload, _ := json.Marshal(map[string]any{
		"destination_id":     dest,
		"good_key":           "silver",
		"quantity":           100.0,
		"delivered_quantity": 100.0,
		"transport_id":       legID.String(),
	})
	if err := h.Handle(ctx, events.ScheduledEvent{ID: time.Now().UnixNano(), WorldID: worldID, Payload: payload}); err != nil {
		t.Fatalf("delivery handle: %v", err)
	}

	// Destination must NOT have been credited — the caravan was seized.
	var credited float64
	_ = pool.QueryRow(ctx,
		`SELECT COALESCE(settled(amount, rate, calc_tick), 0) FROM settlement_goods
		 WHERE settlement_id = $1 AND good_key = 'silver'`, dest).Scan(&credited)
	if credited != 0 {
		t.Errorf("dest silver = %v, want 0 (intercepted caravan must not deliver)", credited)
	}

	// The transport stays intercepted (not flipped to delivered).
	var status string
	_ = pool.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1`, legID).Scan(&status)
	if status != "intercepted" {
		t.Errorf("transport status = %q, want intercepted", status)
	}
}

// TestTradeDelivery_PhysicalLegDeliversAndChainsReturn verifies the happy path:
// leg 1's caravan is marked delivered and a physical leg-2 (trade_return) caravan is
// created with the return manifest. The handler rolls a 5% storm loss, so we retry
// with a fresh caravan until a delivery lands (P(all lost) is astronomically small).
func TestTradeDelivery_PhysicalLegDeliversAndChainsReturn(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-tradetx-%'`,
	); err != nil {
		t.Fatalf("archive leftover worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-tradetx-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	var owner uuid.UUID
	_ = pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"ttx-"+uuid.New().String(), "ttx-"+uuid.New().String()+"@test.invalid",
	).Scan(&owner)

	mkSettlement := func(name string, q int) uuid.UUID {
		var prov, id uuid.UUID
		_ = pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
			worldID, q).Scan(&prov)
		_ = pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', $5, 'active', 5000) RETURNING id`,
			worldID, prov, name, owner, q == 0).Scan(&id)
		return id
	}
	seller := mkSettlement("Seller", 0) // leg-1 dest (silver), leg-2 origin
	buyer := mkSettlement("Buyer", 3)   // leg-1 origin, leg-2 dest (goods)

	h := NewDeliveryHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clock.NewTestClock(time.Now())))

	// then_return describes leg 2: goods seller(0,0) → buyer(3,0).
	thenReturn := map[string]any{
		"destination_id": buyer.String(),
		"good_key":       "copper",
		"quantity":       40.0,
		"messenger_id":   uuid.New().String(),
		"travel_mins":    90.0,
		"owner_id":       owner.String(),
		"origin_q":       0, "origin_r": 0,
		"dest_q":         3, "dest_r": 0,
	}

	var leg1 uuid.UUID
	delivered := false
	for i := 0; i < 25 && !delivered; i++ {
		if err := pool.QueryRow(ctx,
			`INSERT INTO transports
			   (world_id, owner_id, kind, origin_id, dest_id, category,
			    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, status, interceptable)
			 VALUES ($1,$2,'trade',$3,$4,'land',3,0,0,0, now(), now(), 1, 'in_transit', true)
			 RETURNING id`,
			worldID, owner, buyer, seller,
		).Scan(&leg1); err != nil {
			t.Fatalf("create leg1 transport: %v", err)
		}
		payload, _ := json.Marshal(map[string]any{
			"destination_id":     seller,
			"good_key":           "silver",
			"quantity":           200.0,
			"delivered_quantity": 200.0,
			"transport_id":       leg1.String(),
			"then_return":        thenReturn,
		})
		if err := h.Handle(ctx, events.ScheduledEvent{ID: time.Now().UnixNano() + int64(i), WorldID: worldID, Payload: payload}); err != nil {
			t.Fatalf("delivery handle: %v", err)
		}
		var st string
		_ = pool.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1`, leg1).Scan(&st)
		delivered = st == "delivered"
	}
	if !delivered {
		t.Fatalf("leg 1 never delivered across retries (all lost to storm?)")
	}

	// Leg 1 marked delivered; seller credited the silver.
	var sellerSilver float64
	_ = pool.QueryRow(ctx,
		`SELECT COALESCE(settled(amount, rate, calc_tick), 0) FROM settlement_goods
		 WHERE settlement_id = $1 AND good_key = 'silver'`, seller).Scan(&sellerSilver)
	if sellerSilver != 200 {
		t.Errorf("seller silver = %v, want 200", sellerSilver)
	}

	// A physical leg-2 return caravan was created (goods → buyer) with the manifest.
	var leg2Count int
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM transports t
		 JOIN transport_goods g ON g.transport_id = t.id
		 WHERE t.world_id = $1 AND t.kind = 'trade_return' AND t.dest_id = $2
		   AND g.good_key = 'copper' AND g.quantity = 40`,
		worldID, buyer).Scan(&leg2Count)
	if leg2Count != 1 {
		t.Errorf("leg-2 return caravan rows = %d, want 1 (physical return leg with copper manifest)", leg2Count)
	}
}
