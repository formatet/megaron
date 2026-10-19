// Command seed-incense gives an EXISTING world its incense hexes (order 32,
// megaron_rokelse_plan_20261019.md). New worlds get them from world.GenerateMap;
// a world whose tiles are already persisted needs this one-off.
//
// It reuses mapgen's placement (world.AddIncenseSources) on the world's stored
// map_tiles, with the seed derived from the world's own map_seed, and writes
// map_tiles.incense_deposit plus the matching provinces rows. It is idempotent:
// a world that already has an incense hex is left untouched.
//
//	seed-incense [--world <uuid>]         show what would be placed
//	seed-incense [--world <uuid>] --yes   write it
//
// Without --world it takes the single world that is not archived. Run where the
// database is: set -a && . ../.env && go run ./cmd/seed-incense …
// Verify afterwards against the DB, not a surface:
//
//	SELECT count(*) FROM map_tiles WHERE world_id=… AND incense_deposit;
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"formatet/megaron/server/internal/world"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	yes := flag.Bool("yes", false, "write the flags (without it, only show what would be placed)")
	worldArg := flag.String("world", "", "world id (default: the single non-archived world)")
	flag.Parse()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	var worldID uuid.UUID
	if *worldArg != "" {
		worldID, err = uuid.Parse(*worldArg)
		if err != nil {
			log.Fatalf("bad --world: %v", err)
		}
	} else {
		rows, qerr := pool.Query(ctx, `SELECT id FROM worlds WHERE status <> 'archived'`)
		if qerr != nil {
			log.Fatal(qerr)
		}
		var ids []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				log.Fatal(err)
			}
			ids = append(ids, id)
		}
		rows.Close()
		if len(ids) != 1 {
			log.Fatalf("found %d non-archived worlds; pass --world", len(ids))
		}
		worldID = ids[0]
	}

	placed, hexes, err := seedIncense(ctx, pool, worldID, *yes)
	if err != nil {
		log.Fatal(err)
	}
	switch {
	case placed == 0:
		fmt.Println("Nothing to do: the world already has incense hexes (or no candidate terrain).")
	case *yes:
		fmt.Printf("Placed %d incense hexes: %v\n", placed, hexes)
	default:
		fmt.Printf("Would place %d incense hexes: %v\nNothing changed. Run again with --yes to write.\n", placed, hexes)
	}
}

// seedIncense places incense on the world's stored map and, when apply is set,
// writes it. It returns how many hexes were placed (0 when the world already had
// some) and their coordinates.
func seedIncense(ctx context.Context, pool *pgxpool.Pool, worldID uuid.UUID, apply bool) (int, [][2]int, error) {
	var seed int64
	var width, height int
	if err := pool.QueryRow(ctx, `SELECT map_seed, map_width, map_height FROM worlds WHERE id=$1`, worldID).
		Scan(&seed, &width, &height); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil, fmt.Errorf("no such world %s", worldID)
		}
		return 0, nil, err
	}
	rows, err := pool.Query(ctx,
		`SELECT q, r, terrain, COALESCE(copper_deposit,false), COALESCE(tin_deposit,false),
		        COALESCE(silver_deposit,false), COALESCE(incense_deposit,false)
		 FROM map_tiles WHERE world_id=$1 ORDER BY r, q`, worldID)
	if err != nil {
		return 0, nil, err
	}
	var tiles []world.MapTile
	for rows.Next() {
		var t world.MapTile
		var terrain string
		if err := rows.Scan(&t.Q, &t.R, &terrain, &t.CopperDeposit, &t.TinDeposit, &t.SilverDeposit, &t.IncenseDeposit); err != nil {
			rows.Close()
			return 0, nil, err
		}
		t.Terrain = world.Terrain(terrain)
		tiles = append(tiles, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, nil, err
	}

	placed := world.AddIncenseSources(tiles, seed, width, height)
	if placed == 0 {
		return 0, nil, nil
	}
	var hexes [][2]int
	var qs, rs []int32
	for _, t := range tiles {
		if t.IncenseDeposit {
			hexes = append(hexes, [2]int{t.Q, t.R})
			qs = append(qs, int32(t.Q))
			rs = append(rs, int32(t.R))
		}
	}
	if !apply {
		return placed, hexes, nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`UPDATE map_tiles mt SET incense_deposit=true
		 FROM unnest($2::int[], $3::int[]) AS w(q, r)
		 WHERE mt.world_id=$1 AND mt.q=w.q AND mt.r=w.r`, worldID, qs, rs); err != nil {
		return 0, nil, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE provinces p SET incense_deposit=true
		 FROM unnest($2::int[], $3::int[]) AS w(q, r)
		 WHERE p.world_id=$1 AND p.map_q=w.q AND p.map_r=w.r`, worldID, qs, rs); err != nil {
		return 0, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, nil, err
	}
	return placed, hexes, nil
}
