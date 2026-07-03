package combat

// Fas 3.5 verification (temenos_capabilities.md — "recruit→100-pipen"):
// capabilities' colonize hint tells a locked Wanax to "recruit 100 men of one
// land type in this settlement". This test proves that path is actually
// walkable end-to-end, not just plausible from reading the code: two
// successive recruit batches into the SAME settlement+unit type reinforce a
// single forming unit (mirroring api/handlers/province.go Recruit's own
// find-existing-forming-unit query — its "Reinforce existing forming unit"
// branch), and once size reaches 100, TrainCompleteHandler.Handle — the REAL
// production code a scheduled TrainComplete event drives — flips it to
// 'garrison'. That is exactly the state capabilities.deployableLandUnits()
// and canColonize require, so this closes the loop the plan asked to verify
// "before Fas 4".

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/poleia/server/internal/events"
)

func TestRecruit_AggregatesToOneDeployableUnit(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// world state='forming' (not 'active') so this fixture never collides
	// with the single-active-world partial unique index — mirrors
	// internal/capabilities/capabilities_test.go's fixture pattern. Not
	// active is fine here: TrainCompleteHandler's forming→garrison flip does
	// not depend on current_world_tick(); its one active-world-dependent
	// step (RecomputeProduction) is best-effort and its failure is only
	// logged, never returned (see train.go).
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, state, status, map_width, map_height)
		 VALUES ($1, 'forming', 'archived', 10, 10) RETURNING id`,
		"train-test-"+uuid.NewString(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM worlds WHERE id = $1`, worldID) })

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"recruiter-"+uuid.NewString(), uuid.NewString()+"@test.invalid",
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

	// Batch 1 (mirrors Recruit's "no existing forming unit" branch): a fresh
	// forming spearman unit at 50/100 men.
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 50, 0, 'forming', $3) RETURNING id`,
		worldID, ownerID, settlementID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create forming unit (batch 1): %v", err)
	}

	// Batch 2 (mirrors Recruit's "reinforce existing forming unit" branch):
	// find the forming unit of the same type in this settlement (ORDER BY
	// created_at LIMIT 1, exactly as province.go does) and grow it — proving
	// two separate recruit calls land on the SAME unit row rather than
	// splitting into two sub-100 units that could never deploy.
	var foundUnitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM units WHERE settlement_id = $1 AND type = 'spearman' AND status = 'forming'
		 ORDER BY created_at LIMIT 1`,
		settlementID,
	).Scan(&foundUnitID); err != nil {
		t.Fatalf("find forming unit for reinforcement: %v", err)
	}
	if foundUnitID != unitID {
		t.Fatalf("second batch found a different unit (%s) than batch 1 created (%s) — recruit would NOT aggregate to a single deployable unit", foundUnitID, unitID)
	}
	if _, err := pool.Exec(ctx, `UPDATE units SET size = size + 50 WHERE id = $1`, foundUnitID); err != nil {
		t.Fatalf("reinforce unit (batch 2): %v", err)
	}

	var sizeAfterBatches int
	var statusAfterBatches string
	if err := pool.QueryRow(ctx, `SELECT size, status FROM units WHERE id = $1`, unitID).
		Scan(&sizeAfterBatches, &statusAfterBatches); err != nil {
		t.Fatalf("read unit after both batches: %v", err)
	}
	if sizeAfterBatches != 100 {
		t.Fatalf("size after two 50-man batches = %d, want 100", sizeAfterBatches)
	}
	if statusAfterBatches != "forming" {
		t.Fatalf("status before TrainComplete = %q, want still 'forming' (the flip only happens on TrainComplete)", statusAfterBatches)
	}

	// Drive the ACTUAL production flip: TrainCompleteHandler.Handle is what a
	// real batch-of-10's ScheduledTrainComplete event invokes (province.go
	// schedules one per batch; the last one to run, once size>=100, performs
	// the forming→garrison transition).
	h := NewTrainCompleteHandler(pool, events.NewStore(pool), nil)
	payload, err := json.Marshal(TrainCompletePayload{
		SettlementID: settlementID,
		UnitType:     "spearman",
		Count:        10,
		UnitID:       unitID,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := h.Handle(ctx, events.ScheduledEvent{
		WorldID:   worldID,
		EventType: events.ScheduledTrainComplete,
		Payload:   payload,
	}); err != nil {
		t.Fatalf("TrainCompleteHandler.Handle: %v", err)
	}

	var finalStatus string
	var finalSize int
	if err := pool.QueryRow(ctx, `SELECT status, size FROM units WHERE id = $1`, unitID).
		Scan(&finalStatus, &finalSize); err != nil {
		t.Fatalf("read unit after TrainComplete: %v", err)
	}
	if finalStatus != "garrison" {
		t.Fatalf("status after TrainComplete = %q, want 'garrison' — the unit recruit built across two batches never became deployable", finalStatus)
	}
	if finalSize != 100 {
		t.Fatalf("size after TrainComplete = %d, want 100", finalSize)
	}
}
