package kharis

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"formatet/megaron/server/internal/events"
)

// Haiku-7: the galley "arrived", then vanished from unit list. With only a
// galley and no grain, the punishment can only be ship_loss; the event and the
// notification must name the ship it sank.
func TestApplyDivinePunishment_ShipLossNamesTheShip(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID, settlementID, ownerID := starvationWarningFixture(t, 0, 0)
	if _, err := pool.Exec(ctx, `DELETE FROM units WHERE settlement_id = $1`, settlementID); err != nil {
		t.Fatalf("clear garrison: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'garrison', $3)`,
		worldID, ownerID, settlementID,
	); err != nil {
		t.Fatalf("seed galley: %v", err)
	}

	rec := &divineNotifyRecorder{}
	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), rec)
	h.applyDivinePunishment(ctx, settlementID, worldID, ownerID)

	if len(rec.calls) != 1 {
		t.Fatalf("NotifyPlayer called %d times, want 1", len(rec.calls))
	}
	if typ, _ := rec.calls[0].payload["type"].(string); typ != "ship_loss" {
		t.Fatalf("punishment type = %q, want ship_loss", typ)
	}
	name, _ := rec.calls[0].payload["unit_name"].(string)
	if !strings.Contains(name, "Galley") {
		t.Errorf("notification unit_name = %q, want the galley's display name", name)
	}
	var raw []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM events WHERE stream_id = $1 AND event_type = 'DivinePunishment' ORDER BY id DESC LIMIT 1`,
		settlementID,
	).Scan(&raw); err != nil {
		t.Fatalf("read event: %v", err)
	}
	var p struct {
		UnitName string `json:"unit_name"`
	}
	_ = json.Unmarshal(raw, &p)
	if p.UnitName != name {
		t.Errorf("event unit_name = %q, notification %q", p.UnitName, name)
	}
}
