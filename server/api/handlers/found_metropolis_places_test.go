package handlers

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
)

// Founding a metropolis claims the unclaimed places in its own catchment for the
// founder and writes ONE PlacesInNewCity notice (megaron_styrande_beslut:
// "PlaceFirstReached för platser i en ny stads egen catchment").
func TestFoundMetropolisAnnouncesCatchmentPlacesOnce(t *testing.T) {
	pool := escortTestPool(t)
	ctx := context.Background()
	worldID, playerID := seedEscortWorld(t, pool)
	eventStore := events.NewStore(pool)
	// A ford next to the founding hex is a river; the six remaining plains fall
	// below the plain minimum, so the river is the only place.
	if _, err := pool.Exec(ctx, `UPDATE map_tiles SET terrain='river_ford' WHERE world_id=$1 AND q=1 AND r=0`, worldID); err != nil {
		t.Fatal(err)
	}
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
	if founded.Places == nil {
		t.Fatal("no PlacesInNewCity notice returned for a catchment with a river")
	}
	var notices, claimed int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND player_id=$2 AND kind='PlacesInNewCity'`, worldID, playerID).Scan(&notices); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM place_names WHERE world_id=$1 AND claimed_by=$2 AND kind='river'`, worldID, playerID).Scan(&claimed); err != nil {
		t.Fatal(err)
	}
	if notices != 1 || claimed != 1 {
		t.Fatalf("notices=%d claimed=%d want 1/1", notices, claimed)
	}
}
