package messenger

// E2E for the order courier's delivery half (temenos_orderlopare_plan.md Fas 2):
// a march order carried by a kind='order' messenger executes ONLY when the
// courier arrives — the unit starts marching at delivery, the messenger row's
// outbound→arrived flip is the idempotency claim, and a delivery that is no
// longer executable (latest-delivered-wins collisions) finishes without error
// and without touching the unit.
//
// DB integration tests (real Postgres, gated by DATABASE_URL).

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
)

func TestOrderDelivery_MarchExecutesOnArrival(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"order-owner-"+uuid.New().String(), "order-owner-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	for q := 0; q <= 4; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			worldID, q,
		); err != nil {
			t.Fatalf("create map_tiles(%d,0): %v", q, err)
		}
	}

	// The dispatching city at (0,0) — the courier's origin.
	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Capital', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, provinceID, ownerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	// The field unit the runner is chasing, positioned at (2,0).
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'positioned', 2, 0) RETURNING id`,
		worldID, ownerID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create positioned unit: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	h := NewOrderDeliveryHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil, clk)

	deliver := func(target [2]int) (uuid.UUID, error) {
		var messengerID uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO messengers
			     (world_id, sender_id, origin_id, destination_id, message_text, status, kind, hex_q, hex_r, dest_q, dest_r, arrives_at)
			 VALUES ($1,$2,$3,NULL,'March order.','outbound','order',0,0,2,0,$4) RETURNING id`,
			worldID, ownerID, settlementID, clk.Now(),
		).Scan(&messengerID); err != nil {
			t.Fatalf("create order messenger: %v", err)
		}
		raw, _ := json.Marshal(OrderDeliveryPayload{
			WorldID: worldID, PlayerID: ownerID, UnitID: unitID, MessengerID: messengerID,
			Verb: "march",
			March: &combat.MarchOrder{
				WorldID: worldID, PlayerID: ownerID, UnitID: unitID,
				TargetQ: target[0], TargetR: target[1],
			},
		})
		return messengerID, h.Handle(ctx, events.ScheduledEvent{Payload: raw})
	}

	// 1. Delivery executes the march: unit marching toward (4,0).
	messengerID, err := deliver([2]int{4, 0})
	if err != nil {
		t.Fatalf("Handle(deliver march) error: %v", err)
	}
	var status string
	var targetQ, targetR *int
	if err := pool.QueryRow(ctx,
		`SELECT status, target_q, target_r FROM units WHERE id = $1`, unitID,
	).Scan(&status, &targetQ, &targetR); err != nil {
		t.Fatalf("read unit: %v", err)
	}
	if status != "marching" || targetQ == nil || *targetQ != 4 || targetR == nil || *targetR != 0 {
		t.Fatalf("unit after delivery = %s → (%v,%v), want marching → (4,0)", status, targetQ, targetR)
	}
	var msgStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id = $1`, messengerID).Scan(&msgStatus); err != nil {
		t.Fatalf("read messenger: %v", err)
	}
	if msgStatus != "arrived" {
		t.Errorf("messenger status = %q, want arrived (the delivery claim)", msgStatus)
	}

	// 2. Idempotent replay: same event again must no-op without error.
	raw, _ := json.Marshal(OrderDeliveryPayload{
		WorldID: worldID, PlayerID: ownerID, UnitID: unitID, MessengerID: messengerID,
		Verb:  "march",
		March: &combat.MarchOrder{WorldID: worldID, PlayerID: ownerID, UnitID: unitID, TargetQ: 4, TargetR: 0},
	})
	if err := h.Handle(ctx, events.ScheduledEvent{Payload: raw}); err != nil {
		t.Fatalf("Handle(replay) error: %v", err)
	}

	// 3. A later courier reaching a unit that is no longer orderable (it is
	// marching now) must finish WITHOUT error — the failure is a notice, not a
	// retry loop — and must not disturb the unit.
	if _, err := deliver([2]int{3, 0}); err != nil {
		t.Fatalf("Handle(deliver to marching unit) error: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT status, target_q FROM units WHERE id = $1`, unitID,
	).Scan(&status, &targetQ); err != nil {
		t.Fatalf("re-read unit: %v", err)
	}
	if status != "marching" || targetQ == nil || *targetQ != 4 {
		t.Errorf("unit disturbed by failed delivery: %s → target_q=%v, want marching → 4", status, targetQ)
	}
}
