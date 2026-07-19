package events

// P3 soak fix (2026-07-19, "marsch-timeout returnerar tyst framgång"): before
// this fix, a scheduled event (e.g. a march's ScheduledUnitArrival or
// ScheduledOrderDelivery) that kept failing under load until dead-lettered
// vanished with only a server-side ERROR log — the player who dispatched the
// march got an honest 202 promising an arrival/courier, and nothing ever told
// them it never happened. This test drives Worker.dispatch (via processBatch)
// through three consecutive failures of a fixture handler and verifies the
// dead-letter hook fires exactly once — on the attempt that actually gets
// dead-lettered, not on every failed attempt, and not never.

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/clock"
)

func testWorkerPool(t *testing.T) *pgxpool.Pool {
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

func TestWorker_DeadLetterHookFiresOnceAfterRepeatedFailures(t *testing.T) {
	pool := testWorkerPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	clk := clock.NewTestClock(time.Now())
	scheduler := NewScheduler(pool, clk)

	const fixtureType ScheduledEventType = "TestDeadLetterFixture"
	if err := scheduler.EnqueueTick(ctx, worldID, fixtureType, map[string]any{"probe": true}, 0); err != nil {
		t.Fatalf("enqueue fixture event: %v", err)
	}

	worker := NewWorker(pool, clk)

	var handlerCalls int32
	worker.Register(fixtureType, func(_ context.Context, _ ScheduledEvent) error {
		atomic.AddInt32(&handlerCalls, 1)
		return fmt.Errorf("simulated persistent failure")
	})

	var hookCalls int32
	var hookSawAttempts int
	worker.RegisterDeadLetterHook(fixtureType, func(_ context.Context, e ScheduledEvent) error {
		atomic.AddInt32(&hookCalls, 1)
		hookSawAttempts = e.Attempts
		return nil
	})

	// Drive three claim-and-dispatch cycles by hand — one per consecutive
	// failure — mirroring what Worker.Run's poll ticker would do over time.
	for i := 0; i < DeadLetterAttempts; i++ {
		if err := worker.processBatch(ctx); err != nil {
			t.Fatalf("processBatch attempt %d: %v", i+1, err)
		}
	}

	if handlerCalls != DeadLetterAttempts {
		t.Errorf("handler called %d times, want %d (one per processBatch cycle)", handlerCalls, DeadLetterAttempts)
	}
	if hookCalls != 1 {
		t.Errorf("dead-letter hook called %d times, want exactly 1 — either the P3 notification never fires (silent drop is back) or it fires on every failed attempt instead of only at dead-letter time", hookCalls)
	}
	if hookSawAttempts != DeadLetterAttempts {
		t.Errorf("hook saw Attempts=%d, want %d (should fire on the dead-lettering attempt)", hookSawAttempts, DeadLetterAttempts)
	}

	var failed bool
	if err := pool.QueryRow(ctx,
		`SELECT failed_at IS NOT NULL FROM scheduled_events WHERE world_id = $1 AND event_type = $2`,
		worldID, string(fixtureType),
	).Scan(&failed); err != nil {
		t.Fatalf("read fixture event: %v", err)
	}
	if !failed {
		t.Error("scheduled_events.failed_at was not set — event was never dead-lettered as expected")
	}
}
