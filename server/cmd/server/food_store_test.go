package main

import (
	"context"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestSeedDailyTicks_NoRetiredReserve(t *testing.T) {
	pool := retentionTestPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`); err != nil {
		t.Fatal(err)
	}
	var wid uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick) VALUES('single-food-store','active',5) RETURNING id`).Scan(&wid); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE id=$1`, wid) })
	sched := events.NewScheduler(pool, clock.NewTestClock(time.Unix(0, 0)))
	seedDailyTicks(ctx, pool, sched)
	var retired, food int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE event_type='SitosTick'),count(*) FILTER(WHERE event_type='FoodTick') FROM scheduled_events WHERE world_id=$1`, wid).Scan(&retired, &food); err != nil {
		t.Fatal(err)
	}
	if retired != 0 || food != 1 {
		t.Fatalf("wrong scheduled food work: retired%d food%d", retired, food)
	}
}
