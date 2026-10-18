package main

import (
	"context"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestSeedDailyTicks_NoLoyalty(t *testing.T) {
	pool := retentionTestPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`); err != nil {
		t.Fatal(err)
	}
	var wid uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick) VALUES('no-loyalty-ticks','active',5) RETURNING id`).Scan(&wid); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE id=$1`, wid) })
	sched := events.NewScheduler(pool, clock.NewTestClock(time.Unix(0, 0)))
	seedDailyTicks(ctx, pool, sched)
	var retired, food int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE event_type IN ('LoyaltyDecayTick','LoyaltyWelfareTick','ColonyPenaltyTick','BorrowedArmyTick')),count(*) FILTER(WHERE event_type='FoodTick') FROM scheduled_events WHERE world_id=$1`, wid).Scan(&retired, &food); err != nil {
		t.Fatal(err)
	}
	if retired != 0 || food != 1 {
		t.Fatalf("loyalty must never be seeded; retain food work: retired%d food%d", retired, food)
	}
}
