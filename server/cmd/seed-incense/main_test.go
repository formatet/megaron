package main

// seed-incense on a stored world: a dry run writes nothing, --yes writes exactly
// the hexes mapgen would have placed, and running it twice changes nothing. DB test.

import (
	"context"
	"os"
	"reflect"
	"sort"
	"testing"

	"formatet/megaron/server/internal/world"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSeedIncense_IdempotentAndMatchesMapgen(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	const w, h = 64, 64
	generated, eff := world.GenerateMap(uuid.Nil, 1009, w, h)
	var want [][2]int
	for _, tile := range generated {
		if tile.IncenseDeposit {
			want = append(want, [2]int{tile.Q, tile.R})
		}
	}
	if len(want) == 0 {
		t.Fatal("fixture map has no incense")
	}

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds(name, status, map_seed, map_width, map_height) VALUES($1,'archived',$2,$3,$4) RETURNING id`,
		"seed-incense-"+uuid.NewString(), eff, w, h).Scan(&worldID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id=$1`, worldID) })
	// Persist the map the way an OLD world has it: no incense flags.
	for _, tile := range generated {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles(world_id,q,r,terrain,coastal,fertility,mineral,copper_deposit,tin_deposit,silver_deposit,cedar_deposit)
			 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			worldID, tile.Q, tile.R, string(tile.Terrain), tile.Coastal, tile.Fertility, tile.Mineral,
			tile.CopperDeposit, tile.TinDeposit, tile.SilverDeposit, tile.CedarDeposit); err != nil {
			t.Fatal(err)
		}
	}
	// A settled province on the first incense hex: its flag must follow.
	if _, err := pool.Exec(ctx,
		`INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,$2,$3,'scrub_maquis')`,
		worldID, want[0][0], want[0][1]); err != nil {
		t.Fatal(err)
	}

	read := func() [][2]int {
		rows, err := pool.Query(ctx, `SELECT q,r FROM map_tiles WHERE world_id=$1 AND incense_deposit ORDER BY q,r`, worldID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out [][2]int
		for rows.Next() {
			var q, r int
			if err := rows.Scan(&q, &r); err != nil {
				t.Fatal(err)
			}
			out = append(out, [2]int{q, r})
		}
		return out
	}
	sortHexes := func(x [][2]int) {
		sort.Slice(x, func(i, j int) bool {
			if x[i][0] != x[j][0] {
				return x[i][0] < x[j][0]
			}
			return x[i][1] < x[j][1]
		})
	}
	sortHexes(want)

	n, _, err := seedIncense(ctx, pool, worldID, false)
	if err != nil || n != len(want) {
		t.Fatalf("dry run: placed %d (err %v), want %d", n, err, len(want))
	}
	if got := read(); len(got) != 0 {
		t.Fatalf("dry run wrote %d rows", len(got))
	}

	if n, _, err = seedIncense(ctx, pool, worldID, true); err != nil || n != len(want) {
		t.Fatalf("first run: placed %d (err %v), want %d", n, err, len(want))
	}
	first := read()
	if !reflect.DeepEqual(first, want) {
		t.Fatalf("seeded hexes differ from mapgen's:\n got %v\nwant %v", first, want)
	}
	var provFlag bool
	if err := pool.QueryRow(ctx, `SELECT incense_deposit FROM provinces WHERE world_id=$1`, worldID).Scan(&provFlag); err != nil || !provFlag {
		t.Fatalf("province flag not set (err %v)", err)
	}

	if n, _, err = seedIncense(ctx, pool, worldID, true); err != nil || n != 0 {
		t.Fatalf("second run: placed %d (err %v), want 0", n, err)
	}
	if second := read(); !reflect.DeepEqual(second, first) {
		t.Fatalf("second run changed the hexes: %v vs %v", second, first)
	}
}
