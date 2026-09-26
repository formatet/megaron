package economy

// R6 (megaron_plan_sjohandel_kraver_skepp.md): a naval transport already
// in_transit at deploy time, from before this slice, has ship_unit_id = NULL
// — it must go on delivering exactly as it always did, with no home leg and
// no crash, rather than the code assuming every naval transport now carries a
// bound ship.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestTradeDelivery_PreexistingNavalTransportWithoutShipDeliversNoHomeLeg(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-r6-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	var owner uuid.UUID
	_ = pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"r6-"+uuid.New().String(),
	).Scan(&owner)

	var prov, origin, dest uuid.UUID
	_ = pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID).Scan(&prov)
	_ = pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Origin', 'achaean', $3, 'capital', true, 'active', 5000) RETURNING id`,
		worldID, prov, owner,
	).Scan(&origin)
	var prov2 uuid.UUID
	_ = pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 5, 0, 'plains') RETURNING id`,
		worldID).Scan(&prov2)
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Dest', 'achaean', $3, 'colony', false, 'active', 5000) RETURNING id`,
		worldID, prov2, owner,
	).Scan(&dest); err != nil {
		t.Fatalf("create dest settlement: %v", err)
	}

	// A naval transport exactly as it looked before migration 146: category
	// 'naval', no ship_unit_id column value at all (defaults NULL).
	var legID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO transports
		   (world_id, owner_id, kind, origin_id, dest_id, category,
		    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, status, interceptable)
		 VALUES ($1,$2,'transfer',$3,$4,'naval',0,0,5,0, now(), now(), 1, 'in_transit', true)
		 RETURNING id`,
		worldID, owner, origin, dest,
	).Scan(&legID); err != nil {
		t.Fatalf("create pre-existing naval transport: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_goods (transport_id, good_key, quantity) VALUES ($1, 'grain', 30)`, legID,
	); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}

	h := NewDeliveryHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clock.NewTestClock(time.Now())))
	payload, _ := json.Marshal(map[string]any{
		"destination_id":     dest,
		"good_key":           "grain",
		"quantity":           30.0,
		"delivered_quantity": 30.0,
		"transport_id":       legID.String(),
	})
	if err := h.Handle(ctx, events.ScheduledEvent{ID: time.Now().UnixNano(), WorldID: worldID, Payload: payload}); err != nil {
		t.Fatalf("delivery handle: %v", err)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1`, legID).Scan(&status); err != nil {
		t.Fatalf("read transport status: %v", err)
	}
	if status != "delivered" {
		t.Errorf("transport status = %q, want delivered (unaffected by R3's ship binding)", status)
	}

	var grain float64
	_ = pool.QueryRow(ctx,
		`SELECT COALESCE(settled(amount, rate, calc_tick), 0) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'`,
		dest,
	).Scan(&grain)
	if grain != 30 {
		t.Errorf("dest grain = %v, want 30 (delivered exactly as before this slice)", grain)
	}

	// No home leg was dispatched — dispatchShipReturnLeg must no-op silently
	// when ship_unit_id is NULL (R6).
	var homeLegCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM transports WHERE world_id = $1 AND kind = 'ship_return'`, worldID,
	).Scan(&homeLegCount); err != nil {
		t.Fatalf("count ship_return legs: %v", err)
	}
	if homeLegCount != 0 {
		t.Errorf("ship_return legs = %d, want 0 (no ship was ever bound to this pre-existing transport)", homeLegCount)
	}
}
