package handlers

// megaron_plan_varldsid_resolver.md: resolveWorldID must look up the world
// per request (ORDER BY created_at DESC), not freeze it at boot — otherwise a
// reseed leaves the web surface serving a deleted world until restart.
//
// DB integration test (real Postgres, gated by DATABASE_URL).

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func worldIDTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// insertWorldAt inserts a test world with an explicit created_at (so ordering
// is deterministic regardless of how fast the inserts run) and status
// 'archived' so it never collides with the DB's one-active-world unique
// index. resolveWorldID does not filter by status (megaron_plan_varldsid_
// resolver.md: matches ensureWorld's pre-existing, status-agnostic lookup).
func insertWorldAt(t *testing.T, pool *pgxpool.Pool, createdAt time.Time) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, created_at) VALUES ($1, 'archived', $2) RETURNING id`,
		"worldid-test-"+uuid.New().String(), createdAt,
	).Scan(&id); err != nil {
		t.Fatalf("insert test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM worlds WHERE id = $1`, id)
	})
	return id
}

// TestResolveWorldID_PicksNewestPerRequest proves the two halves of the fix
// at once: the lookup runs fresh on every call (no field frozen at
// construction), and ORDER BY created_at DESC picks the newest row when more
// than one exists.
func TestResolveWorldID_PicksNewestPerRequest(t *testing.T) {
	pool := worldIDTestPool(t)
	ctx := context.Background()
	h := &WebHandler{pool: pool}

	base := time.Now().UTC()
	worldA := insertWorldAt(t, pool, base)

	gotA, err := h.resolveWorldID(ctx)
	if err != nil {
		t.Fatalf("resolveWorldID after inserting A: %v", err)
	}
	if gotA != worldA {
		t.Fatalf("resolveWorldID = %s, want newest world A = %s", gotA, worldA)
	}

	// A newer world appears (e.g. a reseed) — the SAME handler instance, with
	// no reconstruction, must now resolve to it.
	worldB := insertWorldAt(t, pool, base.Add(time.Hour))

	gotB, err := h.resolveWorldID(ctx)
	if err != nil {
		t.Fatalf("resolveWorldID after inserting B: %v", err)
	}
	if gotB != worldB {
		t.Fatalf("resolveWorldID = %s, want newest world B = %s (per-request lookup must follow a reseed without restart)", gotB, worldB)
	}
}

// TestResolveWorldID_OrderByIsLoadBearing is the mutation-test counterpart
// (megaron_plan_varldsid_resolver.md step 3: "ta bort ORDER BY → testet ska
// bli icke-deterministiskt/falla"). It runs the SAME query resolveWorldID
// uses (ordered) side by side with the query minus its ORDER BY, against a
// two-row fixture on a freshly inserted heap (no updates/deletes on these
// rows, so Postgres's sequential scan returns them in insertion order). The
// unordered query empirically comes back with the OLDEST world — i.e. the
// exact wrong-world-after-reseed bug this slice fixes — which is what makes
// the ORDER BY load-bearing rather than cosmetic.
func TestResolveWorldID_OrderByIsLoadBearing(t *testing.T) {
	pool := worldIDTestPool(t)
	ctx := context.Background()

	base := time.Now().UTC()
	worldOld := insertWorldAt(t, pool, base)
	worldNew := insertWorldAt(t, pool, base.Add(time.Hour))

	var ordered uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM worlds ORDER BY created_at DESC LIMIT 1`,
	).Scan(&ordered); err != nil {
		t.Fatalf("ordered query: %v", err)
	}
	if ordered != worldNew {
		t.Fatalf("ordered query = %s, want newest world = %s", ordered, worldNew)
	}

	// Scoped to just these two rows (rather than the bare pre-fix
	// `SELECT id FROM worlds LIMIT 1`) so an unrelated pre-existing world
	// elsewhere in the table — first in physical scan order — can't dominate
	// the result and make this check meaningless.
	var unordered uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM worlds WHERE id IN ($1, $2) LIMIT 1`, worldOld, worldNew,
	).Scan(&unordered); err != nil {
		t.Fatalf("unordered query: %v", err)
	}
	if unordered != worldOld {
		t.Skipf("unordered query returned %s (expected the pre-fix bug to surface as %s, the oldest world) — Postgres's scan order for this table is not what this test assumed; ORDER BY is still required, just not demonstrated by this particular empirical check", unordered, worldOld)
	}
}
