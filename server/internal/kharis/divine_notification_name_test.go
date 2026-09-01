package kharis

// megaron_plan_tre_tysta_notiserna.md: DivinePunishment/DivineBlessing now
// carry the settlement's name — without it a Wanax with several cities only
// learns that ONE of them was struck, never which. Additive field (CLAUDE.md
// §Events: old fields unchanged). Reuses starvationWarningFixture/
// seedDivineGarrison from divine_effects_test.go — same DB integration
// pattern, DATABASE_URL-gated.

import (
	"context"
	"encoding/json"
	"testing"

	"formatet/megaron/server/internal/events"
)

func TestApplyDivinePunishment_EventAndNotificationCarrySettlementName(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID, settlementID, ownerID := starvationWarningFixture(t, 400, 0)
	seedDivineGarrison(t, pool, ctx, worldID, settlementID, ownerID)

	rec := &divineNotifyRecorder{}
	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), rec)
	h.applyDivinePunishment(ctx, settlementID, worldID, ownerID)

	var raw []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM events WHERE stream_id = $1 AND event_type = 'DivinePunishment' ORDER BY id DESC LIMIT 1`,
		settlementID,
	).Scan(&raw); err != nil {
		t.Fatalf("read DivinePunishment event: %v", err)
	}
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal event payload: %v", err)
	}
	if p.Name != "Starveton" {
		t.Errorf("DivinePunishment event payload name = %q, want %q", p.Name, "Starveton")
	}

	if len(rec.calls) != 1 {
		t.Fatalf("NotifyPlayer called %d times, want 1", len(rec.calls))
	}
	notifiedName, _ := rec.calls[0].payload["name"].(string)
	if notifiedName != "Starveton" {
		t.Errorf("DivinePunishment notification payload name = %q, want %q", notifiedName, "Starveton")
	}
}

func TestApplyDivineBlessing_EventAndNotificationCarrySettlementName(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID, settlementID, ownerID := starvationWarningFixture(t, 400, 0)
	seedDivineGarrison(t, pool, ctx, worldID, settlementID, ownerID)

	rec := &divineNotifyRecorder{}
	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), rec)
	h.applyDivineBlessing(ctx, settlementID, worldID, ownerID)

	var raw []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM events WHERE stream_id = $1 AND event_type = 'DivineBlessing' ORDER BY id DESC LIMIT 1`,
		settlementID,
	).Scan(&raw); err != nil {
		t.Fatalf("read DivineBlessing event: %v", err)
	}
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal event payload: %v", err)
	}
	if p.Name != "Starveton" {
		t.Errorf("DivineBlessing event payload name = %q, want %q", p.Name, "Starveton")
	}

	if len(rec.calls) != 1 {
		t.Fatalf("NotifyPlayer called %d times, want 1", len(rec.calls))
	}
	notifiedName, _ := rec.calls[0].payload["name"].(string)
	if notifiedName != "Starveton" {
		t.Errorf("DivineBlessing notification payload name = %q, want %q", notifiedName, "Starveton")
	}
}
