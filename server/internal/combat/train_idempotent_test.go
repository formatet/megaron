package combat

// G2 idempotency regression for TrainCompleteHandler (ScheduledTrainComplete).
//
// train.go's doc comment claims: "Idempotent: the units UPDATEs use a
// conditional status check so re-running a completed batch is a safe no-op."
// This test proves that claim for the land-unit path: it drives the SAME
// ScheduledTrainComplete event through Handle twice and asserts the unit's
// status/size after the replay match the state after the first run exactly.
// Without the `WHERE status IN ('training','forming')` guard, a replay could
// re-run RecomputeProduction or (if the guard were ever weakened to also
// re-touch size/population) double-apply a training-complete side effect.

import (
	"context"
	"encoding/json"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestTrainCompleteHandler_ReplayIsIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// Mirrors TestRecruit_AggregatesToOneDeployableUnit's fixture: world
	// state='forming'/status='archived' is fine because TrainCompleteHandler's
	// forming→garrison flip does not depend on current_world_tick(); its one
	// active-world-dependent step (RecomputeProduction) is best-effort and its
	// failure is only logged, never returned (train.go).
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, state, status, map_width, map_height)
		 VALUES ($1, 'forming', 'archived', 10, 10) RETURNING id`,
		"train-idem-"+uuid.NewString(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM worlds WHERE id = $1`, worldID) })

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"train-idem-owner-"+uuid.NewString(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
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
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, is_capital, state, population)
		 VALUES ($1, $2, 'Traintown', 'akhaier', $3, true, 'active', 500) RETURNING id`,
		worldID, provinceID, ownerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'training', $3) RETURNING id`,
		worldID, ownerID, settlementID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create training unit: %v", err)
	}

	payload, err := json.Marshal(TrainCompletePayload{
		SettlementID: settlementID,
		UnitType:     "spearman",
		Count:        10,
		UnitID:       unitID,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	evt := events.ScheduledEvent{
		WorldID:   worldID,
		EventType: events.ScheduledTrainComplete,
		Payload:   payload,
	}

	h := NewTrainCompleteHandler(pool, events.NewStore(pool), nil)

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (first run): %v", err)
	}

	var statusAfterFirst string
	var sizeAfterFirst int
	if err := pool.QueryRow(ctx, `SELECT status, size FROM units WHERE id = $1`, unitID).
		Scan(&statusAfterFirst, &sizeAfterFirst); err != nil {
		t.Fatalf("read unit after first run: %v", err)
	}
	if statusAfterFirst != "garrison" {
		t.Fatalf("status after first run = %q, want garrison", statusAfterFirst)
	}
	if sizeAfterFirst != 100 {
		t.Fatalf("size after first run = %d, want 100", sizeAfterFirst)
	}

	var updatedAtFirst string
	if err := pool.QueryRow(ctx, `SELECT updated_at::text FROM units WHERE id = $1`, unitID).Scan(&updatedAtFirst); err != nil {
		t.Fatalf("read updated_at after first run: %v", err)
	}

	// Replay the SAME event: unit is already 'garrison', so the status guard
	// (`WHERE status IN ('training','forming')`) must make this a no-op.
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (replay): %v", err)
	}

	var statusAfterReplay string
	var sizeAfterReplay int
	if err := pool.QueryRow(ctx, `SELECT status, size FROM units WHERE id = $1`, unitID).
		Scan(&statusAfterReplay, &sizeAfterReplay); err != nil {
		t.Fatalf("read unit after replay: %v", err)
	}
	if statusAfterReplay != "garrison" {
		t.Errorf("status after replay = %q, want still garrison", statusAfterReplay)
	}
	if sizeAfterReplay != 100 {
		t.Errorf("size after replay = %d, want still 100 (a non-idempotent handler could re-touch size)", sizeAfterReplay)
	}

	var updatedAtReplay string
	if err := pool.QueryRow(ctx, `SELECT updated_at::text FROM units WHERE id = $1`, unitID).Scan(&updatedAtReplay); err != nil {
		t.Fatalf("read updated_at after replay: %v", err)
	}
	if updatedAtReplay != updatedAtFirst {
		t.Errorf("units.updated_at changed on replay (%s → %s) — the status guard should have skipped the UPDATE entirely", updatedAtFirst, updatedAtReplay)
	}
}
