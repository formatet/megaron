package messenger

import (
	"context"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/tick"
	"testing"
	"time"
)

func TestNewSeaOfferDeadlineIsResolvedArrivalPlus24(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	id := f.waitingMessenger(t, f.originID, 500)
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status='aboard',trade_offer='{"status":"pending"}' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	arrives := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	sched := events.NewScheduler(f.pool, clock.NewTestClock(arrives))
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := scheduleCompletion(ctx, tx, sched, id, 507, arrives); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var expires time.Time
	if err := f.pool.QueryRow(ctx, `SELECT expires_at FROM messengers WHERE id=$1`, id).Scan(&expires); err != nil {
		t.Fatal(err)
	}
	expiry := f.loadScheduledEvent(t, "OfferExpiry", id)
	if expiry.DueTick != 507+24 || expires.Sub(arrives) != tick.RealUntil(24, 0) {
		t.Fatalf("resolved sea expiry must equal arrival+24: due=%d wall=%v", expiry.DueTick, expires.Sub(arrives))
	}
}

func TestExistingOfferArrivalKeepsPersisted168Deadline(t *testing.T) {
	f := setupLandFixture(t)
	ctx := context.Background()
	id := f.outboundMessenger(t)
	sched := events.NewScheduler(f.pool, clock.NewTestClock(time.Now()))
	deadline := f.currentTick + 168
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET trade_offer='{"status":"pending"}',expires_at=arrives_at+$2::interval WHERE id=$1`, id, tick.RealUntil(168, 0).String()); err != nil {
		t.Fatal(err)
	}
	if err := sched.EnqueueTick(ctx, f.worldID, events.ScheduledOfferExpiry, map[string]any{"messenger_id": id.String()}, deadline); err != nil {
		t.Fatal(err)
	}
	deliver(t, f.pool, f.arrivalHandler(), f.worldID, id, f.currentTick)
	stay := f.loadScheduledEvent(t, "MessengerStayEnd", id)
	if stay.DueTick != deadline {
		t.Fatalf("existing offer stay=%d must retain persisted deadline=%d", stay.DueTick, deadline)
	}
	expiry := f.loadScheduledEvent(t, "OfferExpiry", id)
	if expiry.DueTick != deadline {
		t.Fatalf("existing expiry changed to%d", expiry.DueTick)
	}
}

func TestExistingSeaOfferReplanKeepsScheduled168Deadline(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	id := f.waitingMessenger(t, f.originID, 500)
	arrives := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	expires := arrives.Add(tick.RealUntil(168, 0))
	sched := events.NewScheduler(f.pool, clock.NewTestClock(arrives))
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status='aboard',trade_offer='{"status":"pending"}',arrives_at=$2,expires_at=$3 WHERE id=$1`, id, arrives, expires); err != nil {
		t.Fatal(err)
	}
	if err := sched.EnqueueTick(ctx, f.worldID, events.ScheduledOfferExpiry, map[string]any{"messenger_id": id.String()}, 668); err != nil {
		t.Fatal(err)
	}
	before := f.loadScheduledEvent(t, "OfferExpiry", id)
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := scheduleCompletion(ctx, tx, sched, id, 507, arrives.Add(tick.RealUntil(7, 0))); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	after := f.loadScheduledEvent(t, "OfferExpiry", id)
	var saved time.Time
	if err := f.pool.QueryRow(ctx, `SELECT expires_at FROM messengers WHERE id=$1`, id).Scan(&saved); err != nil {
		t.Fatal(err)
	}
	if after.ID != before.ID || after.DueTick != 668 || !after.ProcessAfter.Equal(before.ProcessAfter) || !saved.Equal(expires) || f.countScheduled(t, "OfferExpiry", id) != 1 {
		t.Fatalf("replan changed existing168 expiry: before=%+v after=%+v wall=%v", before, after, saved)
	}
	arrival := f.loadScheduledEvent(t, "MessengerArrival", id)
	if err := NewArrivalHandler(f.pool, sched, events.NewStore(f.pool), nil).Handle(ctx, arrival); err != nil {
		t.Fatal(err)
	}
	stay := f.loadScheduledEvent(t, "MessengerStayEnd", id)
	if stay.DueTick != 668 {
		t.Fatalf("replanned legacy offer stay=%d want frozen668", stay.DueTick)
	}
}
