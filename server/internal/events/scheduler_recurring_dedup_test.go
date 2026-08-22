package events

// G2 tick-dedup (megaron_plan_tickdedup.md): the self-rescheduling handlers
// (colony.go, kharis/tick.go, loyalty/decay.go, upkeep, …) call
// EnqueueTickRecurring as the LAST line of their handler body. If
// events.Worker retries a handler — which it does on timeout/error, up to
// DeadLetterAttempts times — that line runs twice and two rows land in
// scheduled_events for the same next tick. Each subsequent firing doubles
// again (2 → 4 → 8), silently corrupting the tick cadence.
//
// Fix: a partial unique index on (world_id, event_type, due_tick, payload)
// WHERE processed_at IS NULL, plus ON CONFLICT DO NOTHING in the INSERT.
// payload MUST be part of the index — BattleTick and OccupationCheck
// legitimately have several concurrent rows sharing (world_id, event_type,
// due_tick) that differ only in payload (one row per active battle /
// occupied settlement). An index without payload would silently merge two
// unrelated battles — worse than the bug being fixed here.

import (
	"context"
	"os"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testSchedulerPool(t *testing.T) *pgxpool.Pool {
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

func testSchedulerWorld(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 0) RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})
	return worldID
}

func countPendingScheduled(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID, eventType ScheduledEventType, dueTick int) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM scheduled_events
		 WHERE world_id = $1 AND event_type = $2 AND due_tick = $3 AND processed_at IS NULL`,
		worldID, string(eventType), dueTick,
	).Scan(&n); err != nil {
		t.Fatalf("count scheduled_events: %v", err)
	}
	return n
}

// TestEnqueueTickRecurring_WorkerRetryDoesNotDuplicate is the plan's core
// test. It must show 2 rows against unfixed code and 1 row after the fix —
// a test that has never been red proves nothing.
func TestEnqueueTickRecurring_WorkerRetryDoesNotDuplicate(t *testing.T) {
	pool := testSchedulerPool(t)
	ctx := context.Background()
	worldID := testSchedulerWorld(t, pool)

	clk := clock.NewTestClock(time.Now())
	scheduler := NewScheduler(pool, clk)

	const fixtureType ScheduledEventType = "TestRecurringDedupFixture"
	payload := map[string]any{"probe": true}
	const lastDue = 0
	const interval = 1

	// First firing of the handler's self-reschedule line.
	if err := scheduler.EnqueueTickRecurring(ctx, worldID, fixtureType, payload, lastDue, interval); err != nil {
		t.Fatalf("first EnqueueTickRecurring: %v", err)
	}
	// Simulated worker retry: the SAME handler invocation runs again (e.g.
	// after a timeout) and calls the SAME self-reschedule line a second time
	// with identical arguments.
	if err := scheduler.EnqueueTickRecurring(ctx, worldID, fixtureType, payload, lastDue, interval); err != nil {
		t.Fatalf("retried EnqueueTickRecurring: %v", err)
	}

	got := countPendingScheduled(t, pool, worldID, fixtureType, lastDue+interval)
	if got != 1 {
		t.Errorf("pending rows for retried recurring event = %d, want 1 (dedup failed — a worker retry doubles the tick cadence)", got)
	}
}

// TestEnqueueTickRecurring_DifferentPayloadSameDueTickBothSurvive is the
// plan's most important regression test: BattleTick and OccupationCheck
// legitimately schedule several concurrent rows sharing (world_id,
// event_type, due_tick) that differ only in payload. A dedup index that
// omits payload would silently collapse two unrelated battles into one.
func TestEnqueueTickRecurring_DifferentPayloadSameDueTickBothSurvive(t *testing.T) {
	pool := testSchedulerPool(t)
	ctx := context.Background()
	worldID := testSchedulerWorld(t, pool)

	clk := clock.NewTestClock(time.Now())
	scheduler := NewScheduler(pool, clk)

	const battleTickType = ScheduledBattleTick
	const lastDue = 0
	const interval = 1

	if err := scheduler.EnqueueTickRecurring(ctx, worldID, battleTickType,
		map[string]any{"battle_id": "11111111-1111-1111-1111-111111111111"}, lastDue, interval); err != nil {
		t.Fatalf("enqueue battle A: %v", err)
	}
	if err := scheduler.EnqueueTickRecurring(ctx, worldID, battleTickType,
		map[string]any{"battle_id": "22222222-2222-2222-2222-222222222222"}, lastDue, interval); err != nil {
		t.Fatalf("enqueue battle B: %v", err)
	}

	got := countPendingScheduled(t, pool, worldID, battleTickType, lastDue+interval)
	if got != 2 {
		t.Errorf("pending BattleTick rows with different payloads = %d, want 2 (dedup index must include payload — it must not merge two different battles)", got)
	}
}

// TestEnqueueTickRecurring_NormalDriftUnchanged: a handler that reschedules
// itself exactly once (the normal, non-retry path) still produces exactly
// one row — the fix must not break ordinary operation.
func TestEnqueueTickRecurring_NormalDriftUnchanged(t *testing.T) {
	pool := testSchedulerPool(t)
	ctx := context.Background()
	worldID := testSchedulerWorld(t, pool)

	clk := clock.NewTestClock(time.Now())
	scheduler := NewScheduler(pool, clk)

	const fixtureType ScheduledEventType = "TestRecurringNormalFixture"
	const lastDue = 5
	const interval = 1

	if err := scheduler.EnqueueTickRecurring(ctx, worldID, fixtureType,
		map[string]any{"probe": true}, lastDue, interval); err != nil {
		t.Fatalf("EnqueueTickRecurring: %v", err)
	}

	got := countPendingScheduled(t, pool, worldID, fixtureType, lastDue+interval)
	if got != 1 {
		t.Errorf("pending rows for a single normal reschedule = %d, want 1", got)
	}
}
