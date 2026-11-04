package handlers

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
)

// Poseidon's galley needs a waterway to the Thalassa (Timothy 2026-11-01): a
// founding on a closed lake keeps its harbour right but gets no ship.
func foundAndCountGalleys(t *testing.T, tiles map[[2]int]string) (galleys int, gift bool) {
	t.Helper()
	pool := escortTestPool(t)
	ctx := context.Background()
	worldID, playerID := seedEscortWorld(t, pool)
	if _, err := pool.Exec(ctx, `UPDATE map_tiles SET coastal = true WHERE world_id=$1 AND q=0 AND r=0`, worldID); err != nil {
		t.Fatal(err)
	}
	for c, terrain := range tiles {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1,$2,$3,$4)
			 ON CONFLICT (world_id, q, r) DO UPDATE SET terrain = EXCLUDED.terrain`,
			worldID, c[0], c[1], terrain); err != nil {
			t.Fatal(err)
		}
	}
	eventStore := events.NewStore(pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := seedNomadicHost(ctx, tx, eventStore, worldID, playerID, 0, 0); err != nil {
		t.Fatalf("seedNomadicHost: %v", err)
	}
	founded, err := foundMetropolisFromNomadicHost(ctx, tx, eventStore, clock.NewTestClock(time.Unix(0, 0)), worldID, playerID, "Iolkos", "achaean", nil)
	if err != nil {
		t.Fatalf("found: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM units WHERE settlement_id=$1 AND type='galley'`, founded.SettlementID).Scan(&galleys); err != nil {
		t.Fatal(err)
	}
	return galleys, founded.GalleyID != nil
}

func TestFoundingClosedLakeGivesNoGalley(t *testing.T) {
	galleys, gift := foundAndCountGalleys(t, map[[2]int]string{{1, 0}: "lake"})
	if galleys != 0 || gift {
		t.Fatalf("closed lake: galleys=%d gift=%v, want none", galleys, gift)
	}
}

func TestFoundingLakeWithOutletGivesGalley(t *testing.T) {
	galleys, gift := foundAndCountGalleys(t, map[[2]int]string{
		{1, 0}: "lake", {2, 0}: "river", {3, 0}: "river_ford", {4, 0}: "coastal_sea"})
	if galleys != 1 || !gift {
		t.Fatalf("lake with outlet: galleys=%d gift=%v, want 1", galleys, gift)
	}
}

func TestFoundingSeaCoastGivesGalley(t *testing.T) {
	galleys, gift := foundAndCountGalleys(t, map[[2]int]string{{1, 0}: "coastal_sea"})
	if galleys != 1 || !gift {
		t.Fatalf("sea coast: galleys=%d gift=%v, want 1", galleys, gift)
	}
}
