package main

// DB integration tests for the retention job (gated by DATABASE_URL, same
// convention as api/handlers/cities_test.go). Run against a private,
// disposable database — poleia_test_retention — never poleia_test, which
// three other parallel slices' agents share. Every test resets exactly the
// tables it touches before asserting, so tests are order-independent within
// a single `go test -p 1` run.
//
// See megaron_plan_retention_obundna_tabeller.md acceptance criteria 2-4:
// one falsifiable test per table proving "deletes the old, keeps what's
// needed", the window=0 disable proof, and a >=100k-row drift simulation.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func retentionTestPool(t *testing.T) *pgxpool.Pool {
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

// resetRetentionTables empties exactly the tables named, so each test starts
// from a known-empty state regardless of what earlier tests in the same run
// left behind. Safe here because poleia_test_retention is this slice's own
// private, disposable database — never done against a live DB.
func resetRetentionTables(t *testing.T, pool *pgxpool.Pool, tables ...string) {
	t.Helper()
	ctx := context.Background()
	for _, tbl := range tables {
		if _, err := pool.Exec(ctx, `DELETE FROM `+tbl); err != nil {
			t.Fatalf("reset %s: %v", tbl, err)
		}
	}
}

// ensureTestWorld creates a throwaway world row (scheduled_events.world_id
// has an ON DELETE CASCADE FK onto worlds, mig 104, so it needs a real
// parent) and deletes it again on test cleanup.
func ensureTestWorld(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, map_seed, map_width, map_height) VALUES ($1, 1, 30, 20) RETURNING id`,
		"retention-test-"+uuid.New().String(),
	).Scan(&id)
	if err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM worlds WHERE id = $1`, id)
	})
	return id
}

// TestPruneHeartbeats_KeepsLatestRowRegardlessOfAge is the load-bearing
// invariant for server_heartbeats: absorbStartupDowntime (main.go) reads
// `ORDER BY beat_at DESC LIMIT 1` on every boot, so retention must never
// leave the table with zero rows — not even when every row, including the
// newest one, is older than the configured window.
//
// Falsified by: removing the `AND id <> (SELECT id ... LIMIT 1)` guard from
// heartbeatDeleteSQL — with the guard neutralised, this test fails because
// remaining == 0 instead of 1.
func TestPruneHeartbeats_KeepsLatestRowRegardlessOfAge(t *testing.T) {
	pool := retentionTestPool(t)
	ctx := context.Background()
	resetRetentionTables(t, pool, "server_heartbeats")

	base := time.Now().Add(-10 * 24 * time.Hour) // 10 days old; window under test is 1h
	for i := 0; i < 3; i++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO server_heartbeats (beat_at) VALUES ($1)`,
			base.Add(time.Duration(i)*time.Minute),
		); err != nil {
			t.Fatalf("insert old heartbeat: %v", err)
		}
	}
	// The "latest" row is ALSO older than the window — that's the case that
	// matters. It must survive anyway because it's the global max(beat_at).
	var latestID int64
	latestBeat := base.Add(time.Hour)
	if err := pool.QueryRow(ctx,
		`INSERT INTO server_heartbeats (beat_at) VALUES ($1) RETURNING id`,
		latestBeat,
	).Scan(&latestID); err != nil {
		t.Fatalf("insert latest heartbeat: %v", err)
	}

	cfg := retentionConfig{heartbeatWindow: time.Hour, batchSize: 1000, maxBatchesPerTable: 10}
	pruneHeartbeats(ctx, pool, cfg)

	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM server_heartbeats`).Scan(&remaining); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("expected exactly 1 row to remain (the latest, kept regardless of age), got %d", remaining)
	}
	var remainingID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM server_heartbeats`).Scan(&remainingID); err != nil {
		t.Fatalf("read remaining row: %v", err)
	}
	if remainingID != latestID {
		t.Fatalf("expected the latest row (id=%d) to survive, got id=%d instead", latestID, remainingID)
	}
}

// TestPruneSitosTicks_KeepsClaimsForPendingEvents is the load-bearing
// invariant for processed_sitos_ticks (mig 097): a claim may only be pruned
// once its window has passed AND the scheduled_events row that produced it
// is no longer pending — otherwise a re-run of that event double-taxes an
// already-committed settlement (see sitos_tick.go / mig 097's doc comment).
//
// Falsified by: dropping the `AND NOT EXISTS (...)` clause from
// sitosTickDeleteSQL — with it gone, this test fails because the claim tied
// to the still-pending event gets deleted too.
func TestPruneSitosTicks_KeepsClaimsForPendingEvents(t *testing.T) {
	pool := retentionTestPool(t)
	ctx := context.Background()
	resetRetentionTables(t, pool, "processed_sitos_ticks", "scheduled_events")
	worldID := ensureTestWorld(t, pool)

	var doneEventID, pendingEventID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO scheduled_events (world_id, event_type, payload, process_after, processed_at)
		 VALUES ($1, 'SitosTick', '{}'::jsonb, now() - interval '2 days', now() - interval '2 days')
		 RETURNING id`, worldID,
	).Scan(&doneEventID); err != nil {
		t.Fatalf("insert done event: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO scheduled_events (world_id, event_type, payload, process_after)
		 VALUES ($1, 'SitosTick', '{}'::jsonb, now() - interval '2 days')
		 RETURNING id`, worldID,
	).Scan(&pendingEventID); err != nil {
		t.Fatalf("insert pending event: %v", err)
	}

	oldProcessedAt := time.Now().Add(-48 * time.Hour) // older than the 1h test window
	if _, err := pool.Exec(ctx,
		`INSERT INTO processed_sitos_ticks (event_id, settlement_id, processed_at) VALUES ($1, $2, $3)`,
		doneEventID, uuid.New(), oldProcessedAt,
	); err != nil {
		t.Fatalf("insert done claim: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO processed_sitos_ticks (event_id, settlement_id, processed_at) VALUES ($1, $2, $3)`,
		pendingEventID, uuid.New(), oldProcessedAt,
	); err != nil {
		t.Fatalf("insert pending claim: %v", err)
	}

	cfg := retentionConfig{sitosTickWindow: time.Hour, batchSize: 1000, maxBatchesPerTable: 10}
	pruneSitosTicks(ctx, pool, cfg)

	var doneExists, pendingExists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM processed_sitos_ticks WHERE event_id=$1)`, doneEventID).Scan(&doneExists); err != nil {
		t.Fatalf("check done claim: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM processed_sitos_ticks WHERE event_id=$1)`, pendingEventID).Scan(&pendingExists); err != nil {
		t.Fatalf("check pending claim: %v", err)
	}
	if doneExists {
		t.Errorf("claim for a DONE event should have been pruned, but still exists")
	}
	if !pendingExists {
		t.Errorf("claim for a PENDING event must NEVER be pruned — its event might still be re-run and would double-tax the settlement")
	}
}

// TestPruneScheduledEvents_KeepsPendingRows is the load-bearing invariant for
// scheduled_events: only terminal rows (processed_at set, or failed_at set —
// a dead-letter, never re-claimed per events.Worker's claim query) are ever
// candidates. A row with both NULL is still claimable at any age and must
// never be touched.
//
// Falsified by: changing `processed_at IS NOT NULL AND processed_at < $1` to
// just `processed_at < $1` (or the failed_at equivalent) in the delete SQL —
// with NULL no longer excluded, `NULL < $1` is NULL/false in SQL so this
// particular mistake happens to be caught by SQL's own NULL semantics; the
// real failure mode this guards is copy-pasting the wrong column check
// between the two statements, verified directly by keeping both id-level
// assertions below.
func TestPruneScheduledEvents_KeepsPendingRows(t *testing.T) {
	pool := retentionTestPool(t)
	ctx := context.Background()
	resetRetentionTables(t, pool, "processed_sitos_ticks", "scheduled_events")
	worldID := ensureTestWorld(t, pool)

	var processedID, failedID, pendingID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO scheduled_events (world_id, event_type, payload, process_after, processed_at)
		 VALUES ($1, 'SitosTick', '{}'::jsonb, now() - interval '10 days', now() - interval '10 days')
		 RETURNING id`, worldID).Scan(&processedID); err != nil {
		t.Fatalf("insert processed row: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO scheduled_events (world_id, event_type, payload, process_after, failed_at, attempts)
		 VALUES ($1, 'SitosTick', '{}'::jsonb, now() - interval '10 days', now() - interval '10 days', 3)
		 RETURNING id`, worldID).Scan(&failedID); err != nil {
		t.Fatalf("insert failed row: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO scheduled_events (world_id, event_type, payload, process_after)
		 VALUES ($1, 'SitosTick', '{}'::jsonb, now() - interval '10 days')
		 RETURNING id`, worldID).Scan(&pendingID); err != nil {
		t.Fatalf("insert pending row: %v", err)
	}

	cfg := retentionConfig{
		scheduledEventsWindow:       time.Hour,
		scheduledEventsFailedWindow: time.Hour,
		batchSize:                   1000,
		maxBatchesPerTable:          10,
	}
	pruneScheduledEvents(ctx, pool, cfg)

	exists := func(id int64) bool {
		var e bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM scheduled_events WHERE id=$1)`, id).Scan(&e); err != nil {
			t.Fatalf("check row %d: %v", id, err)
		}
		return e
	}
	if exists(processedID) {
		t.Errorf("processed row (id=%d) should have been pruned", processedID)
	}
	if exists(failedID) {
		t.Errorf("failed/dead-letter row (id=%d) should have been pruned past its own window", failedID)
	}
	if !exists(pendingID) {
		t.Errorf("pending row (id=%d) must NEVER be pruned while still claimable", pendingID)
	}
}

// TestRetentionWindowZero_DisablesPruning is acceptance criterion 3: setting
// a window to 0 (or negative) must disable pruning for that table entirely —
// proven here by leaving deliberately ancient rows (1 year old) in place
// across all three tables and confirming runRetentionPass deletes nothing.
func TestRetentionWindowZero_DisablesPruning(t *testing.T) {
	pool := retentionTestPool(t)
	ctx := context.Background()
	resetRetentionTables(t, pool, "processed_sitos_ticks", "scheduled_events", "server_heartbeats")
	worldID := ensureTestWorld(t, pool)

	veryOld := time.Now().Add(-365 * 24 * time.Hour)

	if _, err := pool.Exec(ctx, `INSERT INTO server_heartbeats (beat_at) VALUES ($1)`, veryOld); err != nil {
		t.Fatalf("seed heartbeat: %v", err)
	}
	// A second row so "keep latest" isn't the reason the row survives.
	if _, err := pool.Exec(ctx, `INSERT INTO server_heartbeats (beat_at) VALUES ($1)`, veryOld.Add(time.Minute)); err != nil {
		t.Fatalf("seed heartbeat 2: %v", err)
	}

	var seID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO scheduled_events (world_id, event_type, payload, process_after, processed_at)
		 VALUES ($1, 'SitosTick', '{}'::jsonb, $2, $2) RETURNING id`, worldID, veryOld,
	).Scan(&seID); err != nil {
		t.Fatalf("seed scheduled_events: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO processed_sitos_ticks (event_id, settlement_id, processed_at) VALUES ($1, $2, $3)`,
		seID, uuid.New(), veryOld,
	); err != nil {
		t.Fatalf("seed processed_sitos_ticks: %v", err)
	}

	cfg := retentionConfig{
		heartbeatWindow:             0,
		sitosTickWindow:             0,
		scheduledEventsWindow:       0,
		scheduledEventsFailedWindow: 0,
		batchSize:                   1000,
		maxBatchesPerTable:          10,
	}
	runRetentionPass(ctx, pool, cfg)

	var hb, se, pst int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM server_heartbeats`).Scan(&hb); err != nil {
		t.Fatalf("count heartbeats: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events`).Scan(&se); err != nil {
		t.Fatalf("count scheduled_events: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM processed_sitos_ticks`).Scan(&pst); err != nil {
		t.Fatalf("count processed_sitos_ticks: %v", err)
	}
	if hb != 2 || se != 1 || pst != 1 {
		t.Fatalf("window<=0 must disable pruning entirely; got heartbeats=%d scheduled_events=%d processed_sitos_ticks=%d, want 2/1/1", hb, se, pst)
	}
}

// TestRetentionDriftSimulation100k is acceptance criterion 4: seed a
// realistic-scale table (>=100k rows, spanning well past the retention
// window plus a small "recent" tail that must survive), run one real prune
// pass, and report before/after counts and time per batch. Uses server_
// heartbeats since it's the simplest single-condition case; the EXPLAIN
// plans in the proof package cover the other two tables' query shapes at
// comparable (200k-600k row) volumes.
func TestRetentionDriftSimulation100k(t *testing.T) {
	if testing.Short() {
		t.Skip("drift simulation seeds 100k+ rows — skipped with -short")
	}
	pool := retentionTestPool(t)
	ctx := context.Background()
	resetRetentionTables(t, pool, "server_heartbeats")

	const totalRows = 120000
	const keepRecent = 500 // inside the window — must survive the prune

	// Bulk insert server-side via generate_series: a 120k-row client-side
	// INSERT loop would spend its wall time on round-trips that have nothing
	// to do with what this test measures (the prune's own batching).
	if _, err := pool.Exec(ctx, `
		INSERT INTO server_heartbeats (beat_at)
		SELECT now() - interval '200 hours' - (n * interval '10 seconds')
		FROM generate_series(1, $1) AS n
	`, totalRows-keepRecent); err != nil {
		t.Fatalf("seed old rows: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO server_heartbeats (beat_at)
		SELECT now() - (n * interval '10 seconds')
		FROM generate_series(1, $1) AS n
	`, keepRecent); err != nil {
		t.Fatalf("seed recent rows: %v", err)
	}

	var before int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM server_heartbeats`).Scan(&before); err != nil {
		t.Fatalf("count before: %v", err)
	}

	cfg := retentionConfig{heartbeatWindow: 168 * time.Hour, batchSize: 5000, maxBatchesPerTable: 100}
	start := time.Now()
	pruneHeartbeats(ctx, pool, cfg)
	elapsed := time.Since(start)

	var after int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM server_heartbeats`).Scan(&after); err != nil {
		t.Fatalf("count after: %v", err)
	}

	deleted := before - after
	wantDeleted := totalRows - keepRecent
	if deleted != wantDeleted {
		t.Fatalf("expected %d rows deleted (everything older than the 168h window), got %d (before=%d after=%d)", wantDeleted, deleted, before, after)
	}
	if after != keepRecent {
		t.Fatalf("expected exactly the %d recent rows to survive, got %d remaining", keepRecent, after)
	}
	batches := (deleted + cfg.batchSize - 1) / cfg.batchSize
	t.Logf("drift simulation: %d -> %d rows (%d deleted) in %v across %d batches (%v/batch avg)",
		before, after, deleted, elapsed, batches, elapsed/time.Duration(batches))
}
