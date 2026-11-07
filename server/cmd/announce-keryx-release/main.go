// announce-keryx-release records a new keryx release and tells every Wanax.
// Run on the server host by tools/announce-keryx-release.sh; never over HTTP.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

// keryxURL is where the client is released (same tag every time; the Codex
// article keryx.md links to it).
const keryxURL = "https://github.com/formatet/megaron/releases/download/keryx-alfa30/kx"

const keryxReleasedKind = "KeryxReleased"

// announce records the release and writes one KeryxReleased notice per Wanax
// in every active world, in ONE transaction. A commit already recorded does
// nothing more (returns 0). It returns the number of notices written.
func announce(ctx context.Context, pool *pgxpool.Pool, commit, url string) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx,
		`INSERT INTO client_releases(client, commit) VALUES('keryx', $1) ON CONFLICT DO NOTHING`, commit)
	if err != nil {
		return 0, fmt.Errorf("record release: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return 0, nil
	}
	raw, err := json.Marshal(map[string]string{"commit": commit, "url": url})
	if err != nil {
		return 0, err
	}
	tag, err = tx.Exec(ctx,
		`INSERT INTO notifications(world_id, player_id, kind, level, body_json, world_tick)
		 SELECT w.id, pwr.player_id, $1, 3, $2, w.current_tick
		   FROM worlds w JOIN player_world_records pwr ON pwr.world_id = w.id
		  WHERE w.status = 'active'`, keryxReleasedKind, raw)
	if err != nil {
		return 0, fmt.Errorf("write notices: %w", err)
	}
	return tag.RowsAffected(), tx.Commit(ctx)
}

func main() {
	if len(os.Args) != 2 || len(os.Args[1]) < 7 {
		log.Fatal("usage: announce-keryx-release <commit (7+ chars)>")
	}
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
	n, err := announce(ctx, pool, os.Args[1], keryxURL)
	if err != nil {
		log.Fatal(err)
	}
	if n == 0 {
		fmt.Println("already announced (or no Wanax in an active world): nothing written")
		return
	}
	fmt.Printf("announced %s to %d Wanax\n", os.Args[1], n)
}
