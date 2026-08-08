package economy

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// TestBackfillPlacements_PlacesForPreP4SettlementsAndSkipsAlreadyPlaced
// covers both halves of the contract: a settlement with population but zero
// placement rows (predates P4) gets workforce placed exactly like a real
// founding would; a settlement that already has ANY placement row is left
// untouched (idempotency — a second BackfillPlacements run must not double-place).
func TestBackfillPlacements_PlacesForPreP4SettlementsAndSkipsAlreadyPlaced(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	preP4 := seedFullRingFixture(t, 100, 500, "plains")
	alreadyPlaced := seedFullRingFixture(t, 100, 500, "plains")
	ringHex := hexgrid.Ring(hexgrid.Coord{Q: 0, R: 0}, hexgrid.CatchmentRadius)[0]
	placeHexGubbe(t, pool, alreadyPlaced, 1, ringHex, "grain")

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT world_id FROM settlements WHERE id = $1`, preP4).Scan(&worldID); err != nil {
		t.Fatalf("read world id: %v", err)
	}
	// seedFullRingFixture archives the PREVIOUS active world each call, so
	// preP4 and alreadyPlaced end up in two different worlds — move
	// alreadyPlaced's settlement into preP4's world so one BackfillPlacements
	// call covers both.
	if _, err := pool.Exec(ctx, `UPDATE settlements SET world_id = $1 WHERE id = $2`, worldID, alreadyPlaced); err != nil {
		t.Fatalf("move settlement into shared world: %v", err)
	}

	n, err := BackfillPlacements(ctx, pool, worldID)
	if err != nil {
		t.Fatalf("BackfillPlacements: %v", err)
	}
	if n != 1 {
		t.Errorf("backfilled = %d, want 1 (only the pre-P4 settlement)", n)
	}

	var preP4Count, alreadyPlacedCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM settlement_placement WHERE settlement_id = $1`, preP4).Scan(&preP4Count); err != nil {
		t.Fatalf("count preP4 placements: %v", err)
	}
	if preP4Count == 0 {
		t.Error("pre-P4 settlement has zero placements after backfill, want > 0")
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM settlement_placement WHERE settlement_id = $1`, alreadyPlaced).Scan(&alreadyPlacedCount); err != nil {
		t.Fatalf("count alreadyPlaced placements: %v", err)
	}
	if alreadyPlacedCount != 1 {
		t.Errorf("already-placed settlement now has %d placement rows, want exactly the 1 it started with (backfill must skip it)", alreadyPlacedCount)
	}

	// Idempotency: running it again must not add anything further.
	n2, err := BackfillPlacements(ctx, pool, worldID)
	if err != nil {
		t.Fatalf("second BackfillPlacements: %v", err)
	}
	if n2 != 0 {
		t.Errorf("second run backfilled = %d, want 0 (both settlements already have placements)", n2)
	}
}
