package main

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type rig struct {
	pool            *pgxpool.Pool
	active, archive uuid.UUID
	a, b, c         uuid.UUID // a: both worlds, b: active only, c: archived only
}

func newRig(t *testing.T) *rig {
	t.Helper()
	ctx := context.Background()
	pool := testPool(t)
	if _, err := pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`); err != nil {
		t.Fatal(err)
	}
	r := &rig{pool: pool}
	for _, w := range []struct {
		id     *uuid.UUID
		status string
	}{{&r.active, "active"}, {&r.archive, "archived"}} {
		if err := pool.QueryRow(ctx, `INSERT INTO worlds(name, status, current_tick) VALUES($1,$2,7) RETURNING id`,
			"announce-"+uuid.NewString(), w.status).Scan(w.id); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []*uuid.UUID{&r.a, &r.b, &r.c} {
		if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`,
			"ann-"+uuid.NewString()).Scan(p); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range []struct{ w, p uuid.UUID }{{r.active, r.a}, {r.archive, r.a}, {r.active, r.b}, {r.archive, r.c}} {
		if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id, world_id) VALUES($1,$2)`, m.p, m.w); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM client_releases WHERE commit LIKE 'tst%'`)
		pool.Exec(ctx, `DELETE FROM worlds WHERE id = ANY($1)`, []uuid.UUID{r.active, r.archive})
		pool.Exec(ctx, `DELETE FROM players WHERE id = ANY($1)`, []uuid.UUID{r.a, r.b, r.c})
	})
	return r
}

func (r *rig) notices(t *testing.T, player uuid.UUID, commit string) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM notifications WHERE player_id=$1 AND kind='KeryxReleased' AND body_json->>'commit'=$2`,
		player, commit).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *rig) releases(t *testing.T, commit string) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(context.Background(), `SELECT count(*) FROM client_releases WHERE client='keryx' AND commit=$1`, commit).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAnnounceIsIdempotentAndReachesEachWanaxOnce(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	commit := "tst" + uuid.NewString()[:6]
	if _, err := announce(ctx, r.pool, commit, keryxURL); err != nil {
		t.Fatal(err)
	}
	if _, err := announce(ctx, r.pool, commit, keryxURL); err != nil {
		t.Fatalf("second run with the same commit must be a quiet no-op: %v", err)
	}
	if got := r.releases(t, commit); got != 1 {
		t.Errorf("client_releases rows = %d, want 1", got)
	}
	if got := r.notices(t, r.a, commit); got != 1 {
		t.Errorf("a Wanax in two worlds got %d notices, want 1 (only the active world counts)", got)
	}
	if got := r.notices(t, r.b, commit); got != 1 {
		t.Errorf("active-world Wanax got %d notices, want 1", got)
	}
	if got := r.notices(t, r.c, commit); got != 0 {
		t.Errorf("a Wanax only in an archived world got %d notices, want 0", got)
	}
	var tick int
	var url string
	if err := r.pool.QueryRow(ctx, `SELECT world_tick, body_json->>'url' FROM notifications WHERE player_id=$1 AND body_json->>'commit'=$2`, r.b, commit).Scan(&tick, &url); err != nil {
		t.Fatal(err)
	}
	if tick != 7 || url != keryxURL {
		t.Errorf("notice tick=%d url=%q", tick, url)
	}
}

func TestAnnounceNewCommitGivesNewNotice(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	c1, c2 := "tst"+uuid.NewString()[:6], "tst"+uuid.NewString()[:6]
	for _, c := range []string{c1, c2} {
		if _, err := announce(ctx, r.pool, c, keryxURL); err != nil {
			t.Fatal(err)
		}
	}
	if r.notices(t, r.b, c1) != 1 || r.notices(t, r.b, c2) != 1 {
		t.Error("each new commit must notify once")
	}
}
