package economy

// megaron_plan_tre_tysta_notiserna.md: FoodShortfall now carries the
// settlement's name in both the event payload and the NotifyPlayer call —
// without it a Wanax with several cities only learns that ONE went hungry,
// never which. Additive field (CLAUDE.md §Events).

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

type foodShortfallNotifyRecorder struct {
	mu    sync.Mutex
	calls []map[string]any
}

func (r *foodShortfallNotifyRecorder) BroadcastEvent(worldID uuid.UUID, kind string, payload any) {}

func (r *foodShortfallNotifyRecorder) NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, _ := payload.(map[string]any)
	r.calls = append(r.calls, m)
	return nil
}

func TestFoodTick_ShortfallEventAndNotificationCarrySettlementName(t *testing.T) {
	pool := testPool(t)
	const population = 1000 // demand = 1000 * 0.005 (GrainConsumptionPerCitizenPerTick) = 5/tick
	f := newFoodFixture(t, pool, "shortfall-name", population)
	seedFoodStock(t, pool, f, 1, 0, 0) // far short of the day's demand

	rec := &foodShortfallNotifyRecorder{}
	h := NewFoodTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), rec)
	if err := h.Handle(context.Background(), events.ScheduledEvent{
		ID: 900002, WorldID: f.worldID, DueTick: f.tick,
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if foodUnmetOf(t, pool, f.settlementID) <= 0 {
		t.Fatal("fixture did not produce a shortfall — test would not discriminate")
	}

	var raw []byte
	if err := pool.QueryRow(context.Background(),
		`SELECT payload FROM events WHERE stream_id = $1 AND event_type = 'FoodShortfall' ORDER BY id DESC LIMIT 1`,
		f.settlementID,
	).Scan(&raw); err != nil {
		t.Fatalf("read FoodShortfall event: %v", err)
	}
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal event payload: %v", err)
	}
	if p.Name != "Foodtown" {
		t.Errorf("FoodShortfall event payload name = %q, want %q", p.Name, "Foodtown")
	}

	if len(rec.calls) != 1 {
		t.Fatalf("NotifyPlayer called %d times, want 1", len(rec.calls))
	}
	notifiedName, _ := rec.calls[0]["name"].(string)
	if notifiedName != "Foodtown" {
		t.Errorf("FoodShortfall notification payload name = %q, want %q", notifiedName, "Foodtown")
	}
}
