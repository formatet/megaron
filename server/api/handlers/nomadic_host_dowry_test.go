package handlers

// SLICE A (soldatens föda, Timothy 2026-08-05): the founder phase's opening
// grain_amount is a balance figure — the escort's dowry — NOT a derivation of
// the upkeep table. Before this slice it happened to equal the table's
// derivation (2 spearmen × 5 grain/day ÷ 24 ticks/day × 2880 ticks = 1200);
// after slice A recalibrates land grain upkeep (×10 base, ×2 more in the
// field), naively re-deriving it from the same formula would silently
// twentyfold the dowry to 24000 — nobody asked for that. This test locks the
// number the plan actually wants: 1200, unconditionally.
//
// DB integration test (real Postgres, gated by DATABASE_URL), same rig as
// escort_adoption_test.go (escortTestPool / seedEscortWorld / seedNomadicHost).

import (
	"context"
	"math"
	"testing"

	"formatet/megaron/server/internal/events"
)

func TestSeedNomadicHost_DowryGrainIsPinnedAt1200(t *testing.T) {
	pool := escortTestPool(t)
	ctx := context.Background()
	worldID, playerID := seedEscortWorld(t, pool)
	eventStore := events.NewStore(pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	if _, err := seedNomadicHost(ctx, tx, eventStore, worldID, playerID, 0, 0); err != nil {
		t.Fatalf("seedNomadicHost: %v", err)
	}

	var grainAmount float64
	if err := tx.QueryRow(ctx,
		`SELECT grain_amount FROM founder_phase WHERE world_id = $1 AND owner_id = $2`,
		worldID, playerID,
	).Scan(&grainAmount); err != nil {
		t.Fatalf("read founder_phase.grain_amount: %v", err)
	}

	const eps = 1e-9
	if math.Abs(grainAmount-1200) > eps {
		t.Errorf("founder_phase.grain_amount = %v, want 1200 — the dowry is a named balance constant "+
			"(nomadicHostDowryGrain), pinned since 2026-08-05, not a derivation of the (recalibrated) "+
			"upkeep table", grainAmount)
	}
}
