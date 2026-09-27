package messenger

// Acceptance 1-4 (megaron_plan_ordna_passage.md, slice 3b-2): "auto-returen
// blir en verklig hemresa". Driven through the REAL flow — a handler fires,
// its scheduled event is loaded and fired through the handler actually
// registered for it, never a hand-set end state (megaron_arbetssatt.md's own
// rule: "varje acceptans ska drivas genom det VERKLIGA flödet").
//
// Acceptance 1 was RED on master (2026-09-27, commit 89f4a2c, see the
// standalone commit that added it first): the timer ArrivalHandler armed for
// "nobody replied" was ScheduledMessengerReturn itself, and ReturnHandler's
// job is to flip 'delivered' straight to 'arrived' — no travel at all, not
// even across a route that visibly takes several ticks to walk. Fixed by
// R1/R2: ArrivalHandler now arms a NEW ScheduledMessengerStayEnd timer, whose
// handler calls messenger.StartReturnLeg (return_leg.go) — the same function
// a spoken Reply uses — to start the real return leg instead of teleporting.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// setupLandFixture is setupPassageFixture's land-only twin: a pure land route
// (all "plains", no sea) between two settlements 3 hexes apart, so
// PlanReturnRoute's own land check (province.CategoryCourierLand) finds a
// real route instead of falling back to the sea-lift. Reuses passageFixture's
// type and its existing helper methods (messengerRow, countScheduled,
// loadScheduledEvent, setTick, …) — only the geography differs.
func setupLandFixture(t *testing.T) *passageFixture {
	t.Helper()
	pool := passageTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	f := &passageFixture{pool: pool, currentTick: 500}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"stayend-land-"+uuid.New().String(), f.currentTick,
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})

	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"stayend-land-owner-"+uuid.New().String(),
	).Scan(&f.ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	for q := 0; q <= 3; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			f.worldID, q,
		); err != nil {
			t.Fatalf("insert map tile (%d,0): %v", q, err)
		}
	}
	mkSettlement := func(q int, name string) uuid.UUID {
		var prov, sid uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
			f.worldID, q,
		).Scan(&prov); err != nil {
			t.Fatalf("create province %s: %v", name, err)
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true, 'active', 5000) RETURNING id`,
			f.worldID, prov, name, f.ownerID,
		).Scan(&sid); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return sid
	}
	f.originID = mkSettlement(0, "Stayend-Land-Origin-"+uuid.NewString())
	f.destID = mkSettlement(3, "Stayend-Land-Dest-"+uuid.NewString())
	return f
}

// outboundMessenger inserts a plain 'outbound' diplomatic messenger from
// f.originID to f.destID, exactly the shape Send would have created.
func (f *passageFixture) outboundMessenger(t *testing.T) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, hex_q, hex_r, arrives_at)
		 VALUES ($1,$2,$3,$4,'hello','outbound',0,0,now()) RETURNING id`,
		f.worldID, f.ownerID, f.originID, f.destID,
	).Scan(&id); err != nil {
		t.Fatalf("create outbound messenger: %v", err)
	}
	return id
}

func (f *passageFixture) arrivalHandler() *ArrivalHandler {
	clk := clock.NewTestClock(time.Now())
	return NewArrivalHandler(f.pool, events.NewScheduler(f.pool, clk), events.NewStore(f.pool), nil)
}

func (f *passageFixture) stayEndHandler() *StayEndHandler {
	clk := clock.NewTestClock(time.Now())
	return NewStayEndHandler(f.pool, events.NewScheduler(f.pool, clk), clk)
}

func (f *passageFixture) returnHandler() *ReturnHandler {
	return NewReturnHandler(f.pool, events.NewStore(f.pool), nil)
}

func deliver(t *testing.T, pool *pgxpool.Pool, ah *ArrivalHandler, worldID, messengerID uuid.UUID, deliverTick int) {
	t.Helper()
	if err := ah.Handle(context.Background(), events.ScheduledEvent{
		WorldID: worldID, DueTick: deliverTick,
		Payload: mustJSON(t, ArrivalPayload{MessengerID: messengerID}),
	}); err != nil {
		t.Fatalf("deliver: %v", err)
	}
}

// TestUnansweredLandMessenger_WalksHomeAtStayEnd is acceptance 1: a land
// messenger nobody replies to turns around ('returning') the instant its stay
// ends, and only becomes 'arrived' once the real land travel time has
// elapsed — never before.
func TestUnansweredLandMessenger_WalksHomeAtStayEnd(t *testing.T) {
	f := setupLandFixture(t)
	ctx := context.Background()
	messengerID := f.outboundMessenger(t)

	deliverTick := f.currentTick
	deliver(t, f.pool, f.arrivalHandler(), f.worldID, messengerID, deliverTick)

	stayEndEvt := f.loadScheduledEvent(t, "MessengerStayEnd", messengerID)
	if stayEndEvt.DueTick != deliverTick+ReplyStayTicks {
		t.Fatalf("stay-end due_tick = %d, want %d (delivery + ReplyStayTicks)", stayEndEvt.DueTick, deliverTick+ReplyStayTicks)
	}

	if err := f.stayEndHandler().Handle(ctx, stayEndEvt); err != nil {
		t.Fatalf("fire stay end: %v", err)
	}

	status, passageStatus, _, _ := f.messengerRow(t, messengerID)
	if status != "returning" {
		t.Fatalf("status right when the stay ends = %q, want returning — the runner must actually WALK home, "+
			"not teleport straight to arrived", status)
	}
	if passageStatus != nil {
		t.Fatalf("passage_status = %v, want NULL on a pure land route", *passageStatus)
	}

	// R5: the eye/notice both key on return_departs_at — verify it, don't assume it.
	var returnDepartsAt *time.Time
	if err := f.pool.QueryRow(ctx, `SELECT return_departs_at FROM messengers WHERE id = $1`, messengerID).Scan(&returnDepartsAt); err != nil {
		t.Fatalf("load return_departs_at: %v", err)
	}
	if returnDepartsAt == nil {
		t.Fatal("return_departs_at not set at stay's end — the homeward eye has nothing to interpolate from")
	}

	// The REAL homecoming is a separate, later event — never the stay-end tick itself.
	returnEvt := f.loadScheduledEvent(t, "MessengerReturn", messengerID)
	if returnEvt.DueTick <= stayEndEvt.DueTick {
		t.Fatalf("real return due_tick = %d, want strictly after the stay-end tick %d (actual travel time)", returnEvt.DueTick, stayEndEvt.DueTick)
	}

	if err := f.returnHandler().Handle(ctx, returnEvt); err != nil {
		t.Fatalf("fire real return: %v", err)
	}
	status, _, _, _ = f.messengerRow(t, messengerID)
	if status != "arrived" {
		t.Fatalf("status after the real return fires = %q, want arrived", status)
	}
}

// TestUnansweredSeaMessenger_AwaitsPassageAtStayEnd is acceptance 2: a
// messenger nobody replies to, standing in a FOREIGN city across the sea
// from home, ends its stay 'awaiting_passage' at that city's port — not
// crossing on its own.
func TestUnansweredSeaMessenger_AwaitsPassageAtStayEnd(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	messengerID := f.outboundMessenger(t)

	deliverTick := f.currentTick
	deliver(t, f.pool, f.arrivalHandler(), f.worldID, messengerID, deliverTick)

	stayEndEvt := f.loadScheduledEvent(t, "MessengerStayEnd", messengerID)
	if err := f.stayEndHandler().Handle(ctx, stayEndEvt); err != nil {
		t.Fatalf("fire stay end: %v", err)
	}

	status, passageStatus, _, _ := f.messengerRow(t, messengerID)
	if status != "returning" {
		t.Fatalf("status = %q, want returning (turned around, waiting for a carrier)", status)
	}
	if passageStatus == nil || *passageStatus != "awaiting_passage" {
		t.Fatalf("passage_status = %v, want awaiting_passage — it must wait for a real ship, not cross on its own", passageStatus)
	}

	var portID uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT passage_port_id FROM messengers WHERE id = $1`, messengerID).Scan(&portID); err != nil {
		t.Fatalf("load passage_port_id: %v", err)
	}
	if portID != f.destID {
		t.Errorf("passage_port_id = %v, want the foreign city it stands in (%v) — R6, the return leg's port is wherever it already stands", portID, f.destID)
	}

	// No terminal event yet — PassageScanHandler schedules it once a carrier boards.
	if n := f.countScheduled(t, "MessengerReturn", messengerID); n != 0 {
		t.Errorf("scheduled MessengerReturn count = %d, want 0 (no carrier has boarded it yet)", n)
	}
}

// TestReplyNearStayEnd_NotArrivedUntilRealHomecoming is acceptance 3: a reply
// that lands just before the stay would have ended, on a route with a real
// (non-zero) return travel time, must not be short-circuited by the
// already-queued stay-end timer firing later — the messenger only becomes
// 'arrived' at the real homecoming.
func TestReplyNearStayEnd_NotArrivedUntilRealHomecoming(t *testing.T) {
	f := setupLandFixture(t)
	ctx := context.Background()
	messengerID := f.outboundMessenger(t)

	deliverTick := f.currentTick
	deliver(t, f.pool, f.arrivalHandler(), f.worldID, messengerID, deliverTick)
	stayEndEvt := f.loadScheduledEvent(t, "MessengerStayEnd", messengerID)

	// A reply lands one tick before the stay would have ended.
	replyTick := stayEndEvt.DueTick - 1
	replyText := "Bring copper and we shall speak"
	result, err := StartReturnLeg(ctx, f.pool, events.NewScheduler(f.pool, clock.NewTestClock(time.Now())),
		f.worldID, messengerID, time.Now(), replyTick, &replyText)
	if err != nil {
		t.Fatalf("reply: %v", err)
	}
	if !result.Started {
		t.Fatal("reply did not start the return leg — messenger was not 'delivered'")
	}

	status, _, _, _ := f.messengerRow(t, messengerID)
	if status != "returning" {
		t.Fatalf("status right after the reply = %q, want returning", status)
	}

	// The stale stay-end timer (queued at delivery time, before the reply)
	// fires anyway — StayEndHandler must no-op, not clobber the reply.
	if err := f.stayEndHandler().Handle(ctx, stayEndEvt); err != nil {
		t.Fatalf("fire stale stay end: %v", err)
	}
	status, _, _, _ = f.messengerRow(t, messengerID)
	if status != "returning" {
		t.Fatalf("status after the stale stay-end fired = %q, want still returning (unclobbered)", status)
	}
	var replyTextAfter *string
	if err := f.pool.QueryRow(ctx, `SELECT reply_text FROM messengers WHERE id = $1`, messengerID).Scan(&replyTextAfter); err != nil {
		t.Fatalf("load reply_text: %v", err)
	}
	if replyTextAfter == nil || *replyTextAfter != replyText {
		t.Fatalf("reply_text after the stale stay-end fired = %v, want unclobbered %q", replyTextAfter, replyText)
	}

	// Only the REAL return event, at its own (later) due tick, brings it home.
	returnEvt := f.loadScheduledEvent(t, "MessengerReturn", messengerID)
	if returnEvt.DueTick <= replyTick {
		t.Fatalf("real return due_tick = %d, want strictly after the reply tick %d", returnEvt.DueTick, replyTick)
	}
	if err := f.returnHandler().Handle(ctx, returnEvt); err != nil {
		t.Fatalf("fire real return: %v", err)
	}
	status, _, _, _ = f.messengerRow(t, messengerID)
	if status != "arrived" {
		t.Fatalf("status after the real return fires = %q, want arrived", status)
	}
}

// TestStayEnd_DoubleRunIsNoOp is acceptance 4's second half: firing the same
// ScheduledMessengerStayEnd event twice (a worker retry, G2) must not arm a
// second return timer or otherwise double-mutate the messenger.
func TestStayEnd_DoubleRunIsNoOp(t *testing.T) {
	f := setupLandFixture(t)
	ctx := context.Background()
	messengerID := f.outboundMessenger(t)

	deliverTick := f.currentTick
	deliver(t, f.pool, f.arrivalHandler(), f.worldID, messengerID, deliverTick)
	stayEndEvt := f.loadScheduledEvent(t, "MessengerStayEnd", messengerID)

	h := f.stayEndHandler()
	if err := h.Handle(ctx, stayEndEvt); err != nil {
		t.Fatalf("first run: %v", err)
	}
	returnEvt := f.loadScheduledEvent(t, "MessengerReturn", messengerID)

	if err := h.Handle(ctx, stayEndEvt); err != nil {
		t.Fatalf("replay: %v", err)
	}

	if n := f.countScheduled(t, "MessengerReturn", messengerID); n != 1 {
		t.Errorf("scheduled MessengerReturn count after replay = %d, want still 1 (a non-idempotent handler would arm a second)", n)
	}
	status, _, _, _ := f.messengerRow(t, messengerID)
	if status != "returning" {
		t.Errorf("status after replay = %q, want still returning", status)
	}
	var returnDepartsAt time.Time
	if err := f.pool.QueryRow(ctx, `SELECT return_departs_at FROM messengers WHERE id = $1`, messengerID).Scan(&returnDepartsAt); err != nil {
		t.Fatalf("load return_departs_at: %v", err)
	}

	// Firing the (still queued, now stale) real return event unchanged by the replay.
	if err := f.returnHandler().Handle(ctx, returnEvt); err != nil {
		t.Fatalf("fire real return: %v", err)
	}
	status, _, _, _ = f.messengerRow(t, messengerID)
	if status != "arrived" {
		t.Errorf("status after the real return fires = %q, want arrived", status)
	}
}
