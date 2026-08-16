package combat

// G2 idempotency regression for BuildCompleteHandler (ScheduledBuildComplete).
//
// build.go's Handle guards against a replay by verifying the build_queue row
// still exists (in the same transaction that later deletes it) before bumping
// buildings.level — "Already resolved" is a documented no-op path. This test
// proves that guard: it runs the SAME ScheduledBuildComplete event through
// Handle twice and asserts the building level only advances once. Without the
// build_queue existence check, the second run would UPSERT buildings.level =
// buildings.level + 1 again, silently doubling every completed building on
// any worker retry.

import (
	"context"
	"encoding/json"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestBuildCompleteHandler_ReplayIsIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "build-idem")

	queueID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO build_queue (id, settlement_id, world_id, building_type, complete_at)
		 VALUES ($1, $2, $3, 'barracks', now())`,
		queueID, f.capitalID, f.worldID,
	); err != nil {
		t.Fatalf("seed build_queue: %v", err)
	}

	payload, err := json.Marshal(BuildCompletePayload{
		SettlementID: f.capitalID,
		BuildQueueID: queueID,
		BuildingType: "barracks",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	evt := events.ScheduledEvent{WorldID: f.worldID, DueTick: f.tick, Payload: payload}

	h := NewBuildCompleteHandler(pool, events.NewStore(pool), nil)

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (first run): %v", err)
	}

	var levelAfterFirst int
	if err := pool.QueryRow(ctx,
		`SELECT level FROM buildings WHERE settlement_id = $1 AND building_type = 'barracks'`,
		f.capitalID,
	).Scan(&levelAfterFirst); err != nil {
		t.Fatalf("read building level after first run: %v", err)
	}
	if levelAfterFirst != 1 {
		t.Fatalf("building level after first run = %d, want 1", levelAfterFirst)
	}

	var queueGone bool
	if err := pool.QueryRow(ctx,
		`SELECT NOT EXISTS(SELECT 1 FROM build_queue WHERE id = $1)`, queueID,
	).Scan(&queueGone); err != nil {
		t.Fatalf("check build_queue after first run: %v", err)
	}
	if !queueGone {
		t.Fatalf("build_queue row still present after first run — handler did not consume it")
	}

	// Replay the SAME event: same BuildQueueID, now already deleted.
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (replay): %v", err)
	}

	var levelAfterReplay int
	if err := pool.QueryRow(ctx,
		`SELECT level FROM buildings WHERE settlement_id = $1 AND building_type = 'barracks'`,
		f.capitalID,
	).Scan(&levelAfterReplay); err != nil {
		t.Fatalf("read building level after replay: %v", err)
	}
	if levelAfterReplay != 1 {
		t.Errorf("building level after replay = %d, want still 1 (a non-idempotent handler would bump it to 2)", levelAfterReplay)
	}
}
