package messenger

import (
	"context"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"testing"
	"time"
)

// Boarding and actual landing can resolve the same arrival day. Generation-
// specific arrivals differ; the unchanged expiry must stay one active timer.
func TestScheduleCompletionSameArrivalRetainsOfferExpiry(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	id := f.waitingMessenger(t, f.originID, 500)
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status='aboard',trade_offer='{}'::jsonb WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC))
	sched := events.NewScheduler(f.pool, clk)
	for pass := 0; pass < 2; pass++ {
		tx, err := f.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := scheduleCompletion(ctx, tx, sched, id, 502, clk.Now()); err != nil {
			tx.Rollback(ctx)
			t.Fatalf("completion %d: %v", pass, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.countScheduled(t, string(events.ScheduledOfferExpiry), id); n != 1 {
		t.Fatalf("expiry timers=%d, want1", n)
	}
	if n := f.countScheduled(t, string(events.ScheduledMessengerArrival), id); n != 2 {
		t.Fatalf("generation-specific arrivals=%d, want2", n)
	}
	var generation int
	if err := f.pool.QueryRow(ctx, `SELECT passage_generation FROM messengers WHERE id=$1`, id).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	if generation != 2 {
		t.Fatalf("generation=%d, want2", generation)
	}
	// A genuinely changed arrival must retain its own later expiry, rather
	// than reusing any timer merely because it belongs to this messenger.
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduleCompletion(ctx, tx, sched, id, 503, clk.Now()); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if n := f.countScheduled(t, string(events.ScheduledOfferExpiry), id); n != 2 {
		t.Fatalf("changed deadline has%d timers,want2", n)
	}

}
