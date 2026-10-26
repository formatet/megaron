// backfill-route-sight repairs terrain memory from retained, witnessed v2 routes.
// It never searches a replacement route or changes physical state. Dry run is
// the default; an explicit -apply commits the single transaction.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"formatet/megaron/server/internal/movement"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	world := flag.String("world", "", "world UUID")
	player := flag.String("player", "", "player UUID; operator must verify historical ownership")
	witnesses := flag.String("witnesses", "", "optional JSON file of independently verified land-eye positions; never guessed paths")
	apply := flag.Bool("apply", false, "commit repair (default rolls back)")
	flag.Parse()
	if err := run(context.Background(), *world, *player, *witnesses, *apply); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, worldArg, playerArg, witnessFile string, apply bool) error {
	world, err := uuid.Parse(worldArg)
	if err != nil {
		return err
	}
	player, err := uuid.Parse(playerArg)
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Match the physical-entry/replacement lock, then freeze the selected units.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,617))`, world.String()); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT id,type,march_route FROM units WHERE world_id=$1 AND owner_id=$2 AND march_route->>'version'='2' ORDER BY id FOR UPDATE`, world, player)
	if err != nil {
		return err
	}
	type source struct {
		id  uuid.UUID
		typ string
		raw []byte
	}
	var sources []source
	for rows.Next() {
		var s source
		if err = rows.Scan(&s.id, &s.typ, &s.raw); err != nil {
			rows.Close()
			return err
		}
		sources = append(sources, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	count := func() (int, error) {
		var n int
		e := tx.QueryRow(ctx, `SELECT count(*) FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2`, world, player).Scan(&n)
		return n, e
	}
	before, err := count()
	if err != nil {
		return err
	}
	journeys, eyes, skipped := 0, 0, 0
	for _, s := range sources {
		var r movement.StoredRoute
		if err = json.Unmarshal(s.raw, &r); err != nil {
			return err
		}
		bounds, e := movement.Boundaries(r.Move())
		if e != nil {
			return e
		}
		key := fmt.Sprintf("u:%s:%x", s.id, sha256.Sum256(s.raw))
		var last int64
		err = tx.QueryRow(ctx, `SELECT last_milli FROM place_reach_progress WHERE world_id=$1 AND journey_key=$2`, world, key).Scan(&last)
		if err == pgx.ErrNoRows {
			skipped++
			continue
		}
		if err != nil {
			return err
		}
		var path []province.MapPosition
		for i, at := range bounds {
			if int64(at) <= last {
				path = append(path, province.MapPosition{Q: r.Hexes[i][0], R: r.Hexes[i][1]})
			}
		}
		kind := province.EyeLandUnit
		if unit.CategoryOf(unit.Type(s.typ)) == unit.CategoryNaval {
			kind = province.EyeShip
		}
		if _, err = province.SweepLiveRadiusAlong(ctx, tx, world, player, path, kind); err != nil {
			return err
		}
		journeys++
		eyes += len(path)
	}
	var witness []province.MapPosition
	if witnessFile != "" {
		raw, e := os.ReadFile(witnessFile)
		if e != nil {
			return e
		}
		if err = json.Unmarshal(raw, &witness); err != nil {
			return err
		}
		if _, err = province.SweepLiveRadiusAlong(ctx, tx, world, player, witness, province.EyeLandUnit); err != nil {
			return err
		}
	}
	after, err := count()
	if err != nil {
		return err
	}
	if apply {
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"applied": apply, "retained_journeys": journeys, "route_eyes": eyes, "unwitnessed_routes_skipped": skipped, "explicit_land_witnesses": len(witness), "before": before, "after": after, "added": after - before})
}
