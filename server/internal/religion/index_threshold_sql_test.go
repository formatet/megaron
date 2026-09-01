package religion

// Smoke test for the per-good holder-threshold SQL added when mig 136 rescaled
// every good's stored amount. The Go floor (CountsAsHolder) is unit-tested in
// valuation_test.go, but the parallel SQL path in RecomputeDivineValuations —
// unnest($3::text[], $4::float8[]) into a threshold CTE, then a COALESCE join
// so goods mig 136 skipped fall back to the flat floor — only exists in the
// query string and cannot be exercised by Go compilation. This runs the real
// function against an empty world so a malformed array binding or CTE join
// fails loudly here instead of silently in production.
//
// DATABASE_URL-gated, same skip contract as the handler-package DB tests.

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRecomputeDivineValuations_PerGoodThresholdSQLExecutes(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	defer pool.Close()

	// current_world_tick() (used by the valuation upsert) reads a status='active'
	// world, so the smoke world must be active. Only name is NOT NULL without a
	// default; status is set explicitly to satisfy that lookup.
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ('holder-threshold-smoke', 'active') RETURNING id`,
	).Scan(&worldID); err != nil {
		t.Fatalf("insert smoke world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM divine_valuations WHERE world_id=$1`, worldID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM worlds WHERE id=$1`, worldID)
	})

	// An empty world still returns one row per good (goods LEFT JOIN empty stock
	// LEFT JOIN threshold), which is exactly what exercises the unnest + COALESCE
	// binding. A binding/type error surfaces as a non-nil error here.
	if err := RecomputeDivineValuations(ctx, pool, worldID); err != nil {
		t.Fatalf("RecomputeDivineValuations with per-good thresholds failed to execute: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM divine_valuations WHERE world_id=$1`, worldID,
	).Scan(&n); err != nil {
		t.Fatalf("count valuations: %v", err)
	}
	if n == 0 {
		t.Error("recompute wrote no divine_valuations — the threshold query produced no rows")
	}
}
