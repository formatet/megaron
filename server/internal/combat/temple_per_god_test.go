package combat

import (
	"context"
	"encoding/json"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// ORDER 15: a completed temple belongs to the god its queue entry names (the
// payload never carried one). Another god's temple is a new building; the
// same god again raises that temple.
func TestBuildCompleteTempleFollowsQueuedGod(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID, _, settlementID := newBuildNotifyFixture(t, pool)
	complete := func(god string) {
		t.Helper()
		queueID := uuid.New()
		if _, err := pool.Exec(ctx,
			`INSERT INTO build_queue (id, settlement_id, world_id, building_type, complete_at, god_id) VALUES ($1, $2, $3, 'temple', now(), $4)`,
			queueID, settlementID, worldID, god); err != nil {
			t.Fatal(err)
		}
		payload, _ := json.Marshal(BuildCompletePayload{SettlementID: settlementID, BuildQueueID: queueID, BuildingType: "temple"})
		h := NewBuildCompleteHandler(pool, events.NewStore(pool), &fakeBroadcaster{})
		if err := h.Handle(ctx, events.ScheduledEvent{WorldID: worldID, DueTick: 1, Payload: payload}); err != nil {
			t.Fatal(err)
		}
	}
	complete("potnia")
	complete("velchanos")
	complete("potnia")
	var got string
	if err := pool.QueryRow(ctx, `SELECT string_agg(god_id || ':' || level, ',' ORDER BY god_id) FROM buildings WHERE settlement_id=$1 AND building_type='temple'`, settlementID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "potnia:2,velchanos:1" {
		t.Fatalf("temples %q, want potnia:2,velchanos:1", got)
	}
}
