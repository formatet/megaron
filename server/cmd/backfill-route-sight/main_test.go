package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"formatet/megaron/server/internal/movement"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBackfillOnlyWitnessedPrefixAndDryRunRollback(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("requires migrated test DB")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	w, p, u := uuid.New(), uuid.New(), uuid.New()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO worlds(id,name,status) VALUES($1,$2,'archived')`, w, "backfill-"+w.String())
	exec(`INSERT INTO players(id,username,password_hash) VALUES($1,$2,'x')`, p, "backfill-"+p.String())
	r := movement.StoredRoute{Version: 2, StartTick: 0, EndTick: 8, OffsetMilli: 733, Hexes: [][2]int{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}, {5, 0}, {6, 0}, {7, 0}, {8, 0}}, Costs: []int64{1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000}}
	raw, _ := json.Marshal(r)
	exec(`INSERT INTO units(id,world_id,owner_id,type,category,size,status,q,r,march_route) VALUES($1,$2,$3,'spearman','land',100,'positioned',1,0,$4)`, u, w, p, raw)
	// Use DB's canonical jsonb bytes, exactly as the physical scanner does.
	if err = pool.QueryRow(ctx, `SELECT march_route FROM units WHERE id=$1`, u).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf("u:%s:%x", u, sha256.Sum256(raw))
	exec(`INSERT INTO place_reach_progress(journey_key,world_id,last_milli,last_tick) VALUES($1,$2,1733,1)`, key, w)
	exec(`INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,0,0,'plains'),($1,1,0,'plains'),($1,3,-2,'plains'),($1,8,2,'plains')`, w)
	count := func() int {
		var n int
		if e := pool.QueryRow(ctx, `SELECT count(*) FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2`, w, p).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	if err = run(ctx, w.String(), p.String(), "", false); err != nil {
		t.Fatal(err)
	}
	if count() != 0 {
		t.Fatal("dry run persisted memory")
	}
	for i := 0; i < 2; i++ {
		if err = run(ctx, w.String(), p.String(), "", true); err != nil {
			t.Fatal(err)
		}
		if count() != 3 {
			t.Fatalf("prefix missing or future revealed: got%d want3", count())
		}
	}
	// Replacing the stored route makes the old progress hash inapplicable.
	exec(`DELETE FROM player_scouted_tiles WHERE world_id=$1`, w)
	exec(`UPDATE units SET march_route=jsonb_set(march_route,'{offset_milli}','734') WHERE id=$1`, u)
	if err = run(ctx, w.String(), p.String(), "", true); err != nil {
		t.Fatal(err)
	}
	if count() != 0 {
		t.Fatal("unwitnessed replacement route was backfilled")
	}
}
