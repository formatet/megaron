package messenger

// Acceptance 1 (megaron_plan_ordna_passage.md, slice 3b-2): an unanswered
// messenger's stay ending must NOT flip it straight to 'arrived' — it must
// turn around and walk home, taking the real land travel time.
//
// RED on master (2026-09-27, commit 89f4a2c): the timer ArrivalHandler arms
// for "nobody replied" IS ScheduledMessengerReturn, and ReturnHandler's own
// job is to flip a 'delivered' messenger straight to 'arrived' — no travel at
// all, not even across a route that visibly takes several ticks to walk.
// Fixed by 3b-2 R1/R2: a NEW ScheduledMessengerStayEnd timer starts the real
// return leg (messenger.StartReturnLeg) instead of teleporting; this file is
// rewritten alongside that fix to drive the new timer/handler, but the
// acceptance criterion it proves — turn around first, arrive only later —
// stays the same.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestUnansweredLandMessenger_DoesNotTeleportHomeAtStayEnd(t *testing.T) {
	pool := handlerIdemTestPool(t)
	ctx := context.Background()
	f := newHandlerIdemFixture(t, pool, "stayend-land")

	var messengerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, hex_q, hex_r, arrives_at)
		 VALUES ($1,$2,$3,$4,'hello','outbound',0,0,now()) RETURNING id`,
		f.worldID, f.sender, f.originID, f.destID,
	).Scan(&messengerID); err != nil {
		t.Fatalf("create outbound messenger: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	ah := NewArrivalHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil)
	deliverTick := 500
	if err := ah.Handle(ctx, events.ScheduledEvent{
		WorldID: f.worldID, DueTick: deliverTick,
		Payload: mustJSON(t, ArrivalPayload{MessengerID: messengerID}),
	}); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	// Load the timer ArrivalHandler armed for "nobody replied" and fire it
	// through the handler TODAY registered for that event type (ReturnHandler,
	// on master — see the file comment).
	var payload []byte
	var dueTick int
	if err := pool.QueryRow(ctx,
		`SELECT payload, due_tick FROM scheduled_events
		  WHERE event_type = 'MessengerReturn' AND (payload->>'messenger_id')::uuid = $1`,
		messengerID,
	).Scan(&payload, &dueTick); err != nil {
		t.Fatalf("load stay's-end timer: %v", err)
	}
	if dueTick != deliverTick+ReplyStayTicks {
		t.Fatalf("stay's-end timer due_tick = %d, want %d (delivery + ReplyStayTicks)", dueTick, deliverTick+ReplyStayTicks)
	}

	rh := NewReturnHandler(pool, events.NewStore(pool), nil)
	if err := rh.Handle(ctx, events.ScheduledEvent{
		WorldID: f.worldID, DueTick: dueTick, Payload: payload,
	}); err != nil {
		t.Fatalf("fire stay's-end timer: %v", err)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id = $1`, messengerID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "returning" {
		t.Fatalf("status right when the stay ends = %q, want returning — the runner must actually WALK home "+
			"(taking the real land travel time), not teleport straight to arrived", status)
	}
}
