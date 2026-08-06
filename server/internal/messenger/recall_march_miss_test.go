package messenger

// Regression test for the silent-tap branch inside
// RecallArrivalHandler.handleMarch (megaron_todo.md "Aggregatarmé-recall",
// 2026-07-30): the outbound marching_armies row can already be resolved=true
// by the time the recall messenger reaches it (the army arrived and fought,
// colonized, or was otherwise resolved first) — the exact race
// RecallMarch.go's own old comment named: "If the army arrives and fights
// first, the recall simply misses." Before this fix that branch only
// slog.Info'd and returned — no OrderFailed, even though the dispatch's 200
// response had promised the Wanax a messenger was en route. Mirrors
// order_delivery_recall_miss_test.go's individual-unit companion, reusing its
// fakeRecallBroadcaster test double.
//
// Also proves the fix's idempotency split: a replay of the SAME event (the
// messenger's outbound→arrived claim already consumed) must never re-notify.
//
// DB integration test (real Postgres, gated by DATABASE_URL).

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestRecallArrival_MarchAlreadyResolvedNotifiesOrderFailed(t *testing.T) {
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
		"aggregate-race-"+uuid.New().String(), "aggregate-race-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	var homeProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&homeProvinceID); err != nil {
		t.Fatalf("create home province: %v", err)
	}
	var homeSettlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Capital', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, homeProvinceID, ownerID,
	).Scan(&homeSettlementID); err != nil {
		t.Fatalf("create home settlement: %v", err)
	}

	var interceptProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 4, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&interceptProvinceID); err != nil {
		t.Fatalf("create intercept-point province: %v", err)
	}

	now := time.Now()
	// resolved = true already at insert time — simulates the army having
	// arrived and been resolved by combat/colonize/another order BEFORE the
	// recall messenger reaches it: the genuine race this handler must never
	// fold silently into a no-op.
	var marchID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO marching_armies
		     (world_id, origin_id, target_id, infantry, chariot, ship, elite_infantry,
		      war_galley, merchantman, intent, departs_at, arrives_at, resolved)
		 VALUES ($1,$2,$3, 100,0,0,0,0,0, 'attack', $4,$5, true) RETURNING id`,
		worldID, homeProvinceID, interceptProvinceID, now.Add(-6*time.Hour), now,
	).Scan(&marchID); err != nil {
		t.Fatalf("create already-resolved march: %v", err)
	}

	var messengerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO messengers
		     (world_id, sender_id, origin_id, destination_id, message_text, status, kind, hex_q, hex_r, dest_q, dest_r, arrives_at)
		 VALUES ($1,$2,$3,NULL,'Recall order — return home.','outbound','recall',0,0,4,0,$4) RETURNING id`,
		worldID, ownerID, homeSettlementID, now,
	).Scan(&messengerID); err != nil {
		t.Fatalf("create recall messenger: %v", err)
	}

	hub := &fakeRecallBroadcaster{}
	clk := clock.NewTestClock(now)
	h := NewRecallArrivalHandler(pool, events.NewScheduler(pool, clk), hub, clk)

	raw, _ := json.Marshal(RecallMarchPayload{
		Kind: "march", WorldID: worldID, MessengerID: messengerID, MarchID: marchID,
		Spearman: 100,
		OriginQ:  0, OriginR: 0, TargetQ: 4, TargetR: 0,
		OriginID: homeProvinceID, TargetID: interceptProvinceID,
	})
	if err := h.Handle(ctx, events.ScheduledEvent{Payload: raw}); err != nil {
		t.Fatalf("Handle(march recall race) error: %v", err)
	}

	if len(hub.notified) != 1 || hub.notified[0] != "OrderFailed" {
		t.Fatalf("NotifyPlayer calls = %v, want exactly one OrderFailed — an aggregate-army recall that "+
			"loses the race must never be a silent no-op", hub.notified)
	}
	if len(hub.reasons) != 1 || hub.reasons[0] == "" {
		t.Fatalf("OrderFailed reason = %q, want a non-empty, actionable explanation", hub.reasons)
	}

	var msgStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id = $1`, messengerID).Scan(&msgStatus); err != nil {
		t.Fatalf("read messenger: %v", err)
	}
	if msgStatus != "arrived" {
		t.Errorf("messenger status = %q, want arrived", msgStatus)
	}

	// Idempotent replay of the SAME event must never re-notify — the
	// messenger's own outbound→arrived flip is the claim (Fas 2.2).
	if err := h.Handle(ctx, events.ScheduledEvent{Payload: raw}); err != nil {
		t.Fatalf("Handle(replay) error: %v", err)
	}
	if len(hub.notified) != 1 {
		t.Errorf("NotifyPlayer calls after replay = %d, want still 1 — a replayed event must never re-notify the owner", len(hub.notified))
	}
}
