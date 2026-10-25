package events

import (
	"context"
	"formatet/megaron/server/internal/clock"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestDeadlineEventsWaitForSavedInstantAndOrderWithinTick(t *testing.T) {
	pool := testWorkerPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`); err != nil {
		t.Fatal(err)
	}
	var world uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick) VALUES($1,'active',104) RETURNING id`, uuid.NewString()).Scan(&world); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE id=$1`, world) })
	now := time.Date(2026, 10, 25, 6, 0, 0, 0, time.UTC)
	clk := clock.NewTestClock(now)
	sched := NewScheduler(pool, clk)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, n := range []int{2, 1} {
		if err = sched.EnqueueDeadlineTx(ctx, tx, world, ScheduledUnitArrivalV2, map[string]int{"n": n}, 104, now.Add(time.Duration(n)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(pool, clk)
	var deadlines []time.Time
	worker.Register(ScheduledUnitArrivalV2, func(_ context.Context, e ScheduledEvent) error {
		deadlines = append(deadlines, e.ProcessAfter)
		return nil
	})
	if err = worker.processBatch(ctx); err != nil {
		t.Fatal(err)
	}
	if len(deadlines) != 0 {
		t.Fatalf("future arrivals ran early: %v", deadlines)
	}
	clk.Advance(3 * time.Second)
	if err = worker.processBatch(ctx); err != nil {
		t.Fatal(err)
	}
	if len(deadlines) != 2 || !deadlines[0].Equal(now.Add(time.Second)) || !deadlines[1].Equal(now.Add(2*time.Second)) {
		t.Fatalf("wrong deadline order: %v", deadlines)
	}
}

// Every new freight envelope must obey process_after, even if its due_tick
// is already current. This exercises worker SQL, not just the type catalogue.
func TestFreightDeadlineTypesWaitForExactInstant(t *testing.T) {
	pool := testWorkerPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`); err != nil {
		t.Fatal(err)
	}
	var world uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick) VALUES($1,'active',104) RETURNING id`, uuid.NewString()).Scan(&world); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE id=$1`, world) })
	now := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)
	clk := clock.NewTestClock(now)
	sched := NewScheduler(pool, clk)
	worker := NewWorker(pool, clk)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	count := 0
	for _, typ := range []ScheduledEventType{ScheduledTransportArrivalV2, ScheduledTradeDeliveryV2, ScheduledGiftDeliveryV2, ScheduledShipmentDeliveryV3, ScheduledTradeReturnV2} {
		if err = sched.EnqueueDeadlineTx(ctx, tx, world, typ, map[string]int{"n": 1}, 104, now.Add(44*time.Second)); err != nil {
			t.Fatal(err)
		}
		worker.Register(typ, func(context.Context, ScheduledEvent) error { count++; return nil })
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = worker.processBatch(ctx); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("%d freight jobs ran early", count)
	}
	clk.Advance(44 * time.Second)
	if err = worker.processBatch(ctx); err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Fatalf("%d freight jobs ran at deadline, want5", count)
	}
}
