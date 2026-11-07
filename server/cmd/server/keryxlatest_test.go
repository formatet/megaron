package main

import (
	"context"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"testing"
)

func serveWith(t *testing.T, latest *keryxLatest, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	latest.middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func TestKeryxLatestHeader(t *testing.T) {
	commit := ""
	l := newKeryxLatest(func() string { return commit })
	if got := serveWith(t, l, "/api/v1/x").Header().Get(keryxLatestHeader); got != "" {
		t.Fatalf("no release row: no header, got %q", got)
	}
	commit = "abc1234"
	l.at = l.at.Add(-l.every) // cache expired
	if got := serveWith(t, l, "/api/v1/x").Header().Get(keryxLatestHeader); got != "abc1234" {
		t.Fatalf("header = %q, want abc1234", got)
	}
	if got := serveWith(t, l, "/static/a.js").Header().Get(keryxLatestHeader); got != "" {
		t.Fatalf("static files carry no header, got %q", got)
	}
}

func TestKeryxLatestIsCached(t *testing.T) {
	calls := 0
	l := newKeryxLatest(func() string { calls++; return "abc1234" })
	serveWith(t, l, "/api/v1/x")
	serveWith(t, l, "/api/v1/x")
	if calls != 1 {
		t.Fatalf("loader called %d times within the cache window, want 1", calls)
	}
}

func TestPoolKeryxLatestReadsNewestRow(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	defer pool.Exec(ctx, `DELETE FROM client_releases WHERE commit LIKE 'tst%'`)
	load := poolKeryxLatest(pool)
	pool.Exec(ctx, `DELETE FROM client_releases WHERE client='keryx'`)
	if got := load(); got != "" {
		t.Fatalf("no rows: %q", got)
	}
	pool.Exec(ctx, `INSERT INTO client_releases(client, commit, released_at) VALUES ('keryx','tstold1', now() - interval '1 hour'), ('keryx','tstnew2', now()), ('other','tstzzz3', now() + interval '1 hour')`)
	if got := load(); got != "tstnew2" {
		t.Fatalf("newest keryx row = %q, want tstnew2", got)
	}
}
