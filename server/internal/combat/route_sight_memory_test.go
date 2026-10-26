package combat

import (
	"context"
	"testing"
)

func TestRouteHexRetainsFullSightBetweenPolls(t *testing.T) {
	f := reachWorld(t)
	ctx := context.Background()
	// A side tile seen only from the middle of the journey, never from its endpoints.
	if _, err := f.pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,3,2,'plains'),($1,8,2,'plains')`, f.world); err != nil {
		t.Fatal(err)
	}
	r := reachRoute(0, 8, 0, 8)
	r.Version = 2
	r.OffsetMilli = 733
	f.mover(t, "infantry", f.owner, r)
	// One catch-up scan, no map reads at intermediate positions.
	f.scan(t, 5)
	f.scan(t, 5)
	var middle, future bool
	if err := f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2 AND q=3 AND r=2), EXISTS(SELECT 1 FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2 AND q=8 AND r=2)`, f.world, f.owner).Scan(&middle, &future); err != nil {
		t.Fatal(err)
	}
	if !middle {
		t.Error("passed middle hex lost its full radius-2 sight between polls")
	}
	if future {
		t.Error("future eye position leaked its sight")
	}
}

func TestRouteHexShipRetainsCoastButNotInland(t *testing.T) {
	f := reachWorld(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE map_tiles SET terrain='deep_sea' WHERE world_id=$1;`, f.world); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,3,1,'plains'),($1,3,2,'plains')`, f.world); err != nil {
		t.Fatal(err)
	}
	r := reachRoute(0, 8, 0, 8)
	r.Version = 2
	r.OffsetMilli = 733
	f.mover(t, "galley", f.owner, r)
	f.scan(t, 5)
	var coast, inland bool
	if err := f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2 AND q=3 AND r=1),EXISTS(SELECT 1 FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2 AND q=3 AND r=2)`, f.world, f.owner).Scan(&coast, &inland); err != nil {
		t.Fatal(err)
	}
	if !coast || inland {
		t.Fatalf("ship sight: coast=%v inland=%v", coast, inland)
	}
}

func TestRouteHexSightStartsAtExactBoundary(t *testing.T) {
	f := reachWorld(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,3,-2,'plains'),($1,0,-2,'plains')`, f.world); err != nil {
		t.Fatal(err)
	}
	r := reachRoute(0, 8, 0, 8)
	r.Version = 2
	r.OffsetMilli = 733
	f.mover(t, "infantry", f.owner, r)
	for _, at := range []int64{732, 733, 1732, 1733} {
		tx, err := f.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = recordPlaceEntriesTx(ctx, tx, f.h.store, f.world, at); err != nil {
			tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		var start, first bool
		if err = f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2 AND q=0 AND r=-2),EXISTS(SELECT 1 FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2 AND q=3 AND r=-2)`, f.world, f.owner).Scan(&start, &first); err != nil {
			t.Fatal(err)
		}
		if start != (at >= 733) || first != (at >= 1733) {
			t.Fatalf("at%d start=%v first=%v", at, start, first)
		}
	}
}
