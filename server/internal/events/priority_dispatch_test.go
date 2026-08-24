package events

// The behavioural half of the ordering fix. priority_test.go pins the ladder's
// VALUES; this pins that processBatch actually dispatches by them.
//
// It has to be a DB test: the whole defect lived in the seam between the claim
// query and the Go loop — Postgres guarantees no ordering for RETURNING, so a
// unit test over an in-memory slice would have passed happily against the
// broken code.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"github.com/google/uuid"
)

func TestProcessBatch_DispatchesInTickPriorityOrder(t *testing.T) {
	pool := testWorkerPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 5) RETURNING id`,
		"test-order-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	clk := clock.NewTestClock(time.Now())
	scheduler := NewScheduler(pool, clk)

	// Enqueue in deliberately WRONG order — growth first, then the eyes, then
	// the obligation, then the arrival. Insertion order and id order therefore
	// both disagree with the day's order, so a passing test can only be the
	// priority sort doing the work.
	enqueue := []ScheduledEventType{
		ScheduledKharisTick,
		ScheduledMarchSightingScan,
		ScheduledUpkeepTick,
		ScheduledUnitArrival,
		ScheduledSitosTick,
	}
	for _, et := range enqueue {
		if err := scheduler.EnqueueTick(ctx, worldID, et, map[string]any{"probe": true}, 5); err != nil {
			t.Fatalf("enqueue %s: %v", et, err)
		}
	}

	worker := NewWorker(pool, clk)
	var seen []ScheduledEventType
	record := func(_ context.Context, e ScheduledEvent) error {
		seen = append(seen, e.EventType)
		return nil
	}
	for _, et := range enqueue {
		worker.Register(et, record)
	}

	if err := worker.processBatch(ctx); err != nil {
		t.Fatalf("processBatch: %v", err)
	}

	want := []ScheduledEventType{
		ScheduledUnitArrival,       // 10 — the world moves
		ScheduledMarchSightingScan, // 20 — the eyes read the result
		ScheduledSitosTick,         // 40 — the granary settles
		ScheduledUpkeepTick,        // 50 — the army eats
		ScheduledKharisTick,        // 60 — the surplus grows the city
	}
	if len(seen) != len(want) {
		t.Fatalf("dispatched %d events, want %d: %v", len(seen), len(want), seen)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("dispatch order wrong at position %d:\n got %v\nwant %v", i, seen, want)
		}
	}
}

// The claim query's ORDER BY does a different job from the Go sort, and needs
// its own test: it decides WHICH rows a batch claims. LIMIT 20 means a tick in
// a living world spans several batches, so without priority in the subquery a
// city's KharisTick could be claimed in batch 1 while its UpkeepTick waited for
// batch 2 — and no amount of in-batch sorting would fix that. Here 25 events
// are enqueued with the day's order deliberately opposite to id order, and the
// first batch must still be the high-priority ones.
func TestProcessBatch_ClaimsHighestPriorityAcrossBatchBoundary(t *testing.T) {
	pool := testWorkerPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 5) RETURNING id`,
		"test-batch-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	clk := clock.NewTestClock(time.Now())
	scheduler := NewScheduler(pool, clk)

	// 22 growth events enqueued FIRST (so they hold the lowest ids), then 3
	// arrivals. Under id order the arrivals would all fall into batch 2.
	for i := 0; i < 22; i++ {
		if err := scheduler.EnqueueTick(ctx, worldID, ScheduledKharisTick, map[string]any{"n": i}, 5); err != nil {
			t.Fatalf("enqueue growth %d: %v", i, err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := scheduler.EnqueueTick(ctx, worldID, ScheduledUnitArrival, map[string]any{"n": i}, 5); err != nil {
			t.Fatalf("enqueue arrival %d: %v", i, err)
		}
	}

	worker := NewWorker(pool, clk)
	var arrivalsInFirstBatch int
	worker.Register(ScheduledUnitArrival, func(_ context.Context, _ ScheduledEvent) error {
		arrivalsInFirstBatch++
		return nil
	})
	worker.Register(ScheduledKharisTick, func(_ context.Context, _ ScheduledEvent) error { return nil })

	if err := worker.processBatch(ctx); err != nil {
		t.Fatalf("processBatch: %v", err)
	}

	if arrivalsInFirstBatch != 3 {
		t.Fatalf("first batch claimed %d of 3 arrivals — the claim query is not ordering by priority, "+
			"so a tick's order breaks apart the moment it spans more than one batch", arrivalsInFirstBatch)
	}
}
