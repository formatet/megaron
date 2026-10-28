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
	// Once scheduled, the offer deadline is frozen even if a later carrier
	// resolution changes arrival. Reuse the timer and preserve expires_at.
	before := f.loadScheduledEvent(t, "OfferExpiry", id)
	var expiresBefore time.Time
	if err := f.pool.QueryRow(ctx, `SELECT expires_at FROM messengers WHERE id=$1`, id).Scan(&expiresBefore); err != nil {
		t.Fatal(err)
	}
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
	if n := f.countScheduled(t, string(events.ScheduledOfferExpiry), id); n != 1 {
		t.Fatalf("replanned offer has%d timers,want1 immutable timer", n)
	}
	after := f.loadScheduledEvent(t, "OfferExpiry", id)
	var expiresAfter time.Time
	if err := f.pool.QueryRow(ctx, `SELECT expires_at FROM messengers WHERE id=$1`, id).Scan(&expiresAfter); err != nil {
		t.Fatal(err)
	}
	if after.ID != before.ID || after.DueTick != before.DueTick || !expiresAfter.Equal(expiresBefore) {
		t.Fatalf("replanning changed frozen deadline: before=%+v after=%+v wall=%v -> %v", before, after, expiresBefore, expiresAfter)
	}

}
