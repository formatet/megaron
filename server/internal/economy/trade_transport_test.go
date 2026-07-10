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
