package events

import (
	"context"
	"formatet/megaron/server/internal/clock"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestRetiredSitosTick_CompletesWithoutReschedule(t *testing.T) {
	pool := testWorkerPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`); err != nil {
		t.Fatal(err)
	}
	var wid uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick) VALUES('retired-reserve','active',5) RETURNING id`).Scan(&wid); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE id=$1`, wid) })
	clk := clock.NewTestClock(time.Unix(0, 0))
	sched := NewScheduler(pool, clk)
	if err := sched.EnqueueTick(ctx, wid, ScheduledSitosTick, struct{}{}, 5); err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(pool, clk)
	worker.Register(ScheduledSitosTick, RetiredSitosTick)
	if err := worker.processBatch(ctx); err != nil {
		t.Fatal(err)
	}
	var total, done, failed int
	if err := pool.QueryRow(ctx, `SELECT count(*),count(processed_at),count(failed_at) FROM scheduled_events WHERE world_id=$1 AND event_type='SitosTick'`, wid).Scan(&total, &done, &failed); err != nil {
		t.Fatal(err)
	}
	if total != 1 || done != 1 || failed != 0 {
		t.Fatalf("legacy tick must finish silently once: total%d done%d failed%d", total, done, failed)
	}
	if err := RetiredSitosTick(ctx, ScheduledEvent{WorldID: wid, DueTick: 5}); err != nil {
		t.Fatal(err)
	}
}
