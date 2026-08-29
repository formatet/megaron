package messenger

// Regression test for the silent-tap branch inside OrderDeliveryHandler.Handle's
// recall/redirect case (temenos_orderlopare_plan.md interception fix, 2026-07-30):
// combat.ExecuteRecall returns (nil, nil) when the unit is no longer marching by
// the time the Runner arrives (already completed its march, or an earlier order
// already turned it). Before this fix that branch only slog.Info'd and returned —
// no OrderFailed, even though the dispatch's 202 had promised the Wanax a Runner
// was en route. This never touches ExecuteRecall's own contract (still "no longer
// marching" ⇒ nil, nil) — only how the CALLER reports that outcome to the owner.
//
// DB integration test (real Postgres, gated by DATABASE_URL).

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// fakeRecallBroadcaster records NotifyPlayer calls — a minimal
// combat.Broadcaster test double (mirrors internal/combat's fakeBroadcaster).
type fakeRecallBroadcaster struct {
	notified []string
	reasons  []string
}

func (f *fakeRecallBroadcaster) BroadcastEvent(worldID uuid.UUID, kind string, payload any) {}

func (f *fakeRecallBroadcaster) NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error {
	f.notified = append(f.notified, kind)
	if m, ok := payload.(map[string]any); ok {
		if reason, ok := m["reason"].(string); ok {
			f.reasons = append(f.reasons, reason)
		}
	}
	return nil
}

func TestOrderDelivery_RecallMissedIsNeverSilent(t *testing.T) {
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
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"recall-miss-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

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

	// The unit is NOT marching (already arrived / never marched) — this is
	// exactly the state ExecuteRecall treats as "too late", returning (nil, nil).
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'garrison', 0, 0) RETURNING id`,
		worldID, ownerID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create garrisoned (non-marching) unit: %v", err)
	}

	var messengerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO messengers
		     (world_id, sender_id, origin_id, destination_id, message_text, status, kind, hex_q, hex_r, dest_q, dest_r, arrives_at)
		 VALUES ($1,$2,$3,NULL,'Runner — recall order, return home.','outbound','order',0,0,0,0,$4) RETURNING id`,
		worldID, ownerID, settlementID, time.Now(),
	).Scan(&messengerID); err != nil {
		t.Fatalf("create recall order messenger: %v", err)
	}

	hub := &fakeRecallBroadcaster{}
	clk := clock.NewTestClock(time.Now())
	h := NewOrderDeliveryHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), hub, clk)

	raw, _ := json.Marshal(OrderDeliveryPayload{
		WorldID: worldID, PlayerID: ownerID, UnitID: unitID, MessengerID: messengerID,
		Verb:   "recall",
		Recall: &combat.RecallOrder{WorldID: worldID, UnitID: unitID, Mode: "recall"},
	})
	if err := h.Handle(ctx, events.ScheduledEvent{Payload: raw}); err != nil {
		t.Fatalf("Handle(recall miss) error: %v", err)
	}

	if len(hub.notified) != 1 || hub.notified[0] != "OrderFailed" {
		t.Fatalf("NotifyPlayer calls = %v, want exactly one OrderFailed — a recall that "+
			"arrives too late must never be a silent no-op", hub.notified)
	}
	if len(hub.reasons) != 1 || hub.reasons[0] == "" {
		t.Fatalf("OrderFailed reason = %q, want a non-empty, actionable explanation", hub.reasons)
	}

	// The messenger's outbound→arrived claim still fires regardless — it's the
	// idempotency gate, separate from whether the order could still execute.
	var msgStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id = $1`, messengerID).Scan(&msgStatus); err != nil {
		t.Fatalf("read messenger: %v", err)
	}
	if msgStatus != "arrived" {
		t.Errorf("messenger status = %q, want arrived", msgStatus)
	}
}
