package messenger

// megaron_plan_budbararens_ankomst_tx.md's core proof: ArrivalHandler.Handle
// used to flip status='delivered' and schedule the auto-return in TWO
// separate commits. A crash (or any failure) between them left a messenger
// permanently stranded in 'delivered' with no return timer — and a retry
// silently no-oped on the status guard. These tests lock the transactional
// fix: the flip rolls back if scheduling fails, and concurrent handling of
// the same messenger is serialized by FOR UPDATE, not by a race.
//
// Replay-idempotency itself (exactly one flip, one timer, across two full
// successful runs) is already covered by handler_idempotent_test.go — these
// tests target the crash window and the row lock specifically.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func insertOutboundMessenger(t *testing.T, f handlerIdemFixture) uuid.UUID {
	t.Helper()
	pool := handlerIdemTestPool(t)
	var messengerID uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, hex_q, hex_r, arrives_at)
		 VALUES ($1,$2,$3,$4,'hello','outbound',0,0,now()) RETURNING id`,
		f.worldID, f.sender, f.originID, f.destID,
	).Scan(&messengerID); err != nil {
		t.Fatalf("create outbound messenger: %v", err)
	}
	return messengerID
}

// TestArrivalHandler_FailedScheduleRollsBackTheFlip is the slice's core test.
// It forces EnqueueTickTx's INSERT to fail (a scheduled_events row FK-
// references worlds(id), so an event carrying a WorldID that doesn't exist
// makes the schedule step fail while the messenger flip itself would have
// succeeded) and asserts the whole transaction — including the flip — rolled
// back. Before the fix, the flip used its own connection/commit and could
// never be rolled back by a later failure.
func TestArrivalHandler_FailedScheduleRollsBackTheFlip(t *testing.T) {
	pool := handlerIdemTestPool(t)
	ctx := context.Background()
	f := newHandlerIdemFixture(t, pool, "rollback")
	messengerID := insertOutboundMessenger(t, f)

	payload, _ := json.Marshal(ArrivalPayload{MessengerID: messengerID})
	// A WorldID with no matching row in `worlds` — EnqueueTickTx's INSERT into
	// scheduled_events violates the world_id FK, so Handle must return an
	// error and roll back everything it did in this call.
	badWorldID := uuid.New()
	evt := events.ScheduledEvent{ID: 1, WorldID: badWorldID, Payload: payload, DueTick: 500}

	clk := clock.NewTestClock(time.Now())
	h := NewArrivalHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool))

	if err := h.Handle(ctx, evt); err == nil {
		t.Fatal("Handle with an unschedulable return timer returned nil error, want an error (the schedule step must fail)")
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id = $1`, messengerID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "outbound" {
		t.Errorf("status after failed schedule = %q, want still outbound (the flip must have rolled back with the failed schedule — this is the bug the plan fixes)", status)
	}

	var timers int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM scheduled_events WHERE event_type='MessengerReturn' AND (payload->>'messenger_id')::uuid=$1`,
		messengerID,
	).Scan(&timers); err != nil {
		t.Fatalf("count return timers: %v", err)
	}
	if timers != 0 {
		t.Errorf("return timers after failed schedule = %d, want 0", timers)
	}
}

// TestArrivalHandler_LocksTheRowBeforeReading proves FOR UPDATE serializes
// two concurrent Handle calls for the SAME messenger into one winner and one
// idempotent no-op, rather than a race where both read status='outbound'
// before either commits.
// TestArrivalHandler_LocksTheRowBeforeReading proves the row lock directly
// and deterministically, rather than racing two full Handle() calls against
// each other (tried first — unreliable: one goroutine's entire Handle() call,
// a handful of fast local queries, routinely completes before the second
// goroutine's first query even reaches the connection pool, so the intended
// overlap almost never happens). Instead it holds the messenger row locked
// from OUTSIDE Handle() (a stand-in for "another worker is mid-transaction on
// this same row") and asserts Handle() itself blocks on that external lock
// rather than reading straight past it — proving Handle()'s own SELECT is the
// one carrying FOR UPDATE, not just that Postgres locking works in the
// abstract.
func TestArrivalHandler_LocksTheRowBeforeReading(t *testing.T) {
	pool := handlerIdemTestPool(t)
	ctx := context.Background()
	f := newHandlerIdemFixture(t, pool, "lock")
	messengerID := insertOutboundMessenger(t, f)

	holder, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin lock holder: %v", err)
	}
	defer holder.Rollback(ctx)
	var held string
	if err := holder.QueryRow(ctx,
		"SELECT status FROM messengers WHERE id = $1 FOR UPDATE", messengerID,
	).Scan(&held); err != nil {
		t.Fatalf("holder lock read: %v", err)
	}

	payload, _ := json.Marshal(ArrivalPayload{MessengerID: messengerID})
	evt := events.ScheduledEvent{ID: 1, WorldID: f.worldID, Payload: payload, DueTick: 500}
	clk := clock.NewTestClock(time.Now())
	h := NewArrivalHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool))

	done := make(chan error, 1)
	go func() { done <- h.Handle(ctx, evt) }()

	// Poll pg_stat_activity for the backend Handle() is blocked in, and read
	// back the QUERY TEXT it is blocked on. This is the part a "does Handle
	// eventually unblock" check can't tell apart: an UPDATE always blocks on
	// the holder's row lock regardless of whether the SELECT before it used
	// FOR UPDATE, since any write needs the lock either way. Only the blocked
	// query's own text proves WHICH statement is waiting — the SELECT (correct:
	// it carries "trade_offer", the tail of its column list) or the UPDATE
	// (mutated: FOR UPDATE dropped from the SELECT, so the SELECT returns
	// immediately and it's "SET status = 'delivered'" that blocks instead).
	var blockedQuery string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := pool.QueryRow(ctx,
			`SELECT query FROM pg_stat_activity
			   WHERE wait_event_type = 'Lock' AND query ILIKE '%messengers%' AND query NOT ILIKE '%pg_stat_activity%'
			   LIMIT 1`,
		).Scan(&blockedQuery)
		if err == nil && blockedQuery != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case err := <-done:
		t.Fatalf("Handle returned (err=%v) before we observed it blocked in pg_stat_activity — the row was still externally locked", err)
	default:
	}
	if blockedQuery == "" {
		t.Fatal("never observed Handle() blocked on the messengers row in pg_stat_activity")
	}
	if !strings.Contains(blockedQuery, "trade_offer") {
		t.Errorf("Handle() blocked on:\n%s\nwant it blocked on its SELECT (containing \"trade_offer\") — "+
			"it is instead blocked on the later UPDATE, meaning the SELECT itself is not using FOR UPDATE "+
			"and read straight past the external lock", blockedQuery)
	}

	if err := holder.Commit(ctx); err != nil {
		t.Fatalf("commit lock holder: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Handle errored after the external lock released: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Handle never unblocked after the external lock released")
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id = $1`, messengerID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "delivered" {
		t.Errorf("status = %q, want delivered", status)
	}
}
