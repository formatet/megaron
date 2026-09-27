package messenger

// DB integration tests for CallBack (megaron_plan_ordna_passage.md, slice
// 3b-4, R5) — acceptance criterion 2: a called-back runner goes home over
// land and never delivers; the verb is refused for a runner aboard a carrier
// or waiting in a foreign port.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// foreignSettlement creates a settlement owned by a NEW player in f's world —
// used as a passage_port_id to exercise the "foreign port" rejection, since
// setupPassageFixture's own origin/dest are both owned by the SAME test
// player (loosely called "foreign" in a comment elsewhere, but not for
// ownership purposes).
func (f *passageFixture) foreignSettlement(t *testing.T) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var foreignOwnerID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"passage-foreign-owner-"+uuid.New().String(),
	).Scan(&foreignOwnerID); err != nil {
		t.Fatalf("create foreign player: %v", err)
	}
	var provID, sID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, 9, 0, 'plains', true) RETURNING id`,
		f.worldID,
	).Scan(&provID); err != nil {
		t.Fatalf("create foreign province: %v", err)
	}
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true, 'active', 5000) RETURNING id`,
		f.worldID, provID, "Passage-Foreign-"+uuid.NewString(), foreignOwnerID,
	).Scan(&sID); err != nil {
		t.Fatalf("create foreign settlement: %v", err)
	}
	return sID
}

func testCallBackHandler(f *passageFixture) (*events.Scheduler, clock.Clock) {
	clk := clock.NewTestClock(time.Now())
	return events.NewScheduler(f.pool, clk), clk
}

// TestCallBack_OwnPortOutbound_GoesHomeUndeliveredNeverCrosses is acceptance
// 2's main case: a runner waiting in the SENDER'S OWN port is called back,
// runs home over land, and is marked withdrawn — driven through the real
// CallBack function AND the real ReturnHandler on arrival (not a fixture
// setting the end state by hand).
func TestCallBack_OwnPortOutbound_GoesHomeUndeliveredNeverCrosses(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	sched, clk := testCallBackHandler(f)
	messengerID := f.waitingMessenger(t, f.originID, f.currentTick)

	res, err := CallBack(ctx, f.pool, sched, f.worldID, messengerID, f.ownerID, clk.Now(), f.currentTick)
	if err != nil {
		t.Fatalf("CallBack: %v", err)
	}
	if !res.Started {
		t.Fatal("CallBack Started = false, want true (own-port outbound runner)")
	}

	status, passageStatus, _, _ := f.messengerRow(t, messengerID)
	if status != "returning" {
		t.Errorf("status after call-back = %q, want returning", status)
	}
	if passageStatus != nil {
		t.Errorf("passage_status after call-back = %v, want NULL — it walks home over land", *passageStatus)
	}
	var withdrawn bool
	if err := f.pool.QueryRow(ctx, `SELECT withdrawn FROM messengers WHERE id = $1`, messengerID).Scan(&withdrawn); err != nil {
		t.Fatalf("load withdrawn: %v", err)
	}
	if !withdrawn {
		t.Error("withdrawn = false, want true")
	}

	// It must never have scheduled a delivery to its original target.
	if n := f.countScheduled(t, "MessengerArrival", messengerID); n != 0 {
		t.Errorf("scheduled MessengerArrival = %d, want 0 — a withdrawn runner never delivers", n)
	}
	if n := f.countScheduled(t, "MessengerReturn", messengerID); n != 1 {
		t.Fatalf("scheduled MessengerReturn = %d, want 1", n)
	}

	// Fire the real return event through the real ReturnHandler — the
	// runner actually arrives home, still never having delivered anything.
	e := f.loadScheduledEvent(t, "MessengerReturn", messengerID)
	hub := &fakeRecallBroadcaster{}
	rh := NewReturnHandler(f.pool, events.NewStore(f.pool), hub)
	if err := rh.Handle(ctx, e); err != nil {
		t.Fatalf("ReturnHandler.Handle: %v", err)
	}
	status, _, _, _ = f.messengerRow(t, messengerID)
	if status != "arrived" {
		t.Errorf("status after homecoming = %q, want arrived", status)
	}
	var notifiedWithdrawn int
	for _, k := range hub.notified {
		if k == "MessengerReturned" {
			notifiedWithdrawn++
		}
	}
	if notifiedWithdrawn != 1 {
		t.Errorf("MessengerReturned notifications = %d, want 1", notifiedWithdrawn)
	}
}

// TestCallBack_RejectsForeignPortPickup is acceptance 2's refusal half for a
// return leg: a runner waiting for its FOREIGN-port pickup (3b-3's
// "hämtning", status='returning') cannot be called back — it is not
// outbound, so R5's "bara för ett utgående bud" rejects it (Started=false),
// same real-flow path a Wanax would actually hit trying this.
func TestCallBack_RejectsForeignPortPickup(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	sched, clk := testCallBackHandler(f)
	foreignPort := f.foreignSettlement(t)
	messengerID := f.waitingReturnMessenger(t, foreignPort, f.currentTick)

	res, err := CallBack(ctx, f.pool, sched, f.worldID, messengerID, f.ownerID, clk.Now(), f.currentTick)
	if err != nil {
		t.Fatalf("CallBack: %v", err)
	}
	if res.Started {
		t.Error("CallBack Started = true, want false — a return-leg pickup is never call-back-able")
	}
	status, passageStatus, _, _ := f.messengerRow(t, messengerID)
	if status != "returning" || passageStatus == nil || *passageStatus != "awaiting_passage" {
		t.Errorf("messenger state changed by a rejected call-back: status=%q passage_status=%v", status, passageStatus)
	}
}

// TestCallBack_RejectsOwnPortInvariantViolation defends CallBack's own
// ownPort check directly: ResolveDeparture only ever picks the SENDER's own
// port for an outbound runner (ownedCoastalPorts filters on owner_id), so an
// outbound row with a foreign passage_port_id should never occur through the
// real dispatch flow — but CallBack must still refuse it if it somehow did,
// rather than trust a column it did not itself just write. Hand-crafted row,
// deliberately violating that invariant, to prove the defence fires.
func TestCallBack_RejectsOwnPortInvariantViolation(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	sched, clk := testCallBackHandler(f)
	foreignPort := f.foreignSettlement(t)

	var messengerID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, kind,
		                          hex_q, hex_r, arrives_at, passage_status, passage_port_id, passage_since_tick)
		 VALUES ($1,$2,$3,$4,'hello','outbound','diplomatic',5,0,now(),'awaiting_passage',$5,$6) RETURNING id`,
		f.worldID, f.ownerID, f.originID, f.destID, foreignPort, f.currentTick,
	).Scan(&messengerID); err != nil {
		t.Fatalf("create invariant-violating messenger: %v", err)
	}

	_, err := CallBack(ctx, f.pool, sched, f.worldID, messengerID, f.ownerID, clk.Now(), f.currentTick)
	if err != ErrCallBackNotOwnPort {
		t.Errorf("CallBack err = %v, want ErrCallBackNotOwnPort", err)
	}
}

// TestCallBack_RejectsAboardMessenger: a runner already 'aboard' a real
// carrier cannot be called back — Started=false, no error, nothing touched
// (it is already, physically, on its way; R5 only ever applies to a runner
// still standing in port).
func TestCallBack_RejectsAboardMessenger(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	sched, clk := testCallBackHandler(f)
	messengerID := f.waitingMessenger(t, f.originID, f.currentTick)
	shipID := f.ship(t, f.originID, "merchantman")
	f.departingTransport(t, f.originID, shipID, f.currentTick)
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle (board): %v", err)
	}
	_, boardedStatus, _, _ := f.messengerRow(t, messengerID)
	if boardedStatus == nil || *boardedStatus != "aboard" {
		t.Fatalf("fixture broken: messenger not aboard after boarding, passage_status=%v", boardedStatus)
	}

	res, err := CallBack(ctx, f.pool, sched, f.worldID, messengerID, f.ownerID, clk.Now(), f.currentTick)
	if err != nil {
		t.Fatalf("CallBack: %v", err)
	}
	if res.Started {
		t.Error("CallBack Started = true, want false — an aboard runner cannot be called back")
	}
	status, passageStatus, _, _ := f.messengerRow(t, messengerID)
	if status != "outbound" || passageStatus == nil || *passageStatus != "aboard" {
		t.Errorf("messenger state changed by a rejected call-back: status=%q passage_status=%v", status, passageStatus)
	}
}
