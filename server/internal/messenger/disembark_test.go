package messenger

// DB integration tests for megaron_plan_budets_tre_ben.md (slice 3c) R2: the
// four new mig 151 columns (disembark_q/r, boarded_at, disembark_at) that let
// MapMessengers draw the aboard→ashore legs a boarded messenger actually
// walks, instead of the flat origin→destination interpolation. Same fixture
// and helpers as passage_test.go (megaron_plan_budet_liftar.md, slice 3a).

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// disembarkFields reads mig 151's four new columns for assertions.
func (f *passageFixture) disembarkFields(t *testing.T, id uuid.UUID) (q, r *int, boardedAt, disembarkAt *time.Time) {
	t.Helper()
	if err := f.pool.QueryRow(context.Background(),
		`SELECT disembark_q, disembark_r, boarded_at, disembark_at FROM messengers WHERE id = $1`, id,
	).Scan(&q, &r, &boardedAt, &disembarkAt); err != nil {
		t.Fatalf("load disembark fields: %v", err)
	}
	return
}

// TestPassageScan_BoardOne_SetsDisembarkColumns is R2's core acceptance
// criterion: boarding a real carrier now records not just WHO carries the
// messenger (carrier_name, already tested by passage_test.go) but WHERE and
// WHEN it disembarks — boardOne previously computed disembarkAt/
// carrierArrivesAt and only ever folded their SUM into arrives_at, discarding
// both (the problem statement's own "servern sparar inte var eller när budet
// kliver i land").
func TestPassageScan_BoardOne_SetsDisembarkColumns(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	messengerID := f.waitingMessenger(t, f.originID, f.currentTick+1)
	shipID := f.ship(t, f.originID, "merchantman")
	transportDueTick := f.currentTick + 3
	f.departingTransport(t, f.originID, shipID, transportDueTick)

	before := time.Now().Add(-2 * time.Second)
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	after := time.Now().Add(2 * time.Second)

	q, r, boardedAt, disembarkAt := f.disembarkFields(t, messengerID)
	if q == nil || r == nil || *q != 5 || *r != 0 {
		t.Fatalf("disembark_q/r = %v,%v, want 5,0 (departingTransport's own dest_q/dest_r)", q, r)
	}
	if boardedAt == nil {
		t.Fatal("boarded_at not set")
	}
	if boardedAt.Before(before) || boardedAt.After(after) {
		t.Errorf("boarded_at = %v, want within [%v,%v] (the scan's own boarding moment)", boardedAt, before, after)
	}
	if disembarkAt == nil {
		t.Fatal("disembark_at not set")
	}
	// disembark_at is the CARRIER's own arrival (carrierArrivesAt) — strictly
	// before arrives_at, which additionally adds the landward leg's duration
	// (zero-distance here, since the carrier's destination IS the messenger's
	// final settlement, but boardOne always Adds landDur, so arrives_at is
	// never before disembark_at).
	var arrivesAt time.Time
	if err := f.pool.QueryRow(ctx, `SELECT arrives_at FROM messengers WHERE id = $1`, messengerID).Scan(&arrivesAt); err != nil {
		t.Fatalf("load arrives_at: %v", err)
	}
	if arrivesAt.Before(*disembarkAt) {
		t.Errorf("arrives_at %v is before disembark_at %v", arrivesAt, disembarkAt)
	}
}

// TestPassageScan_BoardsReturnLegFromForeignCity_SetsDisembarkColumns proves
// R2's "returbenet" requirement: a messenger boarding on its RETURN leg from a
// foreign port goes through the exact same boardOne call and gets the same
// four columns set — same fixture shape as passage_test.go's
// TestPassageScan_BoardsReturnLegFromForeignCity, which only checked
// carrier_name.
func TestPassageScan_BoardsReturnLegFromForeignCity_SetsDisembarkColumns(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	messengerID := f.waitingReturnMessenger(t, f.destID, f.currentTick+1)
	shipID := f.ship(t, f.destID, "merchantman")
	f.departingTransportTo(t, f.destID, f.originID, 0, 0, shipID, f.currentTick+3)

	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	q, r, boardedAt, disembarkAt := f.disembarkFields(t, messengerID)
	if q == nil || r == nil || *q != 0 || *r != 0 {
		t.Fatalf("disembark_q/r = %v,%v, want 0,0 (the home leg's own dest_q/dest_r)", q, r)
	}
	if boardedAt == nil {
		t.Error("boarded_at not set on the return leg")
	}
	if disembarkAt == nil {
		t.Error("disembark_at not set on the return leg")
	}
}

// TestSealLostCarrier_NullsDisembarkColumns is R2's lifecycle requirement: a
// messenger whose carrier is lost mid-voyage seals back to 'returning_sealed'
// with an UNKNOWN real position (R3's own doc: "hamnen är det enda ärliga
// ankaret") — the disembark point/time it had while aboard the now-lost
// carrier must not linger and be drawn as if still valid.
func TestSealLostCarrier_NullsDisembarkColumns(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	messengerID := f.waitingMessenger(t, f.originID, f.currentTick+1)
	shipID := f.ship(t, f.originID, "merchantman")
	transportID := f.departingTransport(t, f.originID, shipID, f.currentTick+3)

	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle (board): %v", err)
	}
	if q, r, boardedAt, disembarkAt := f.disembarkFields(t, messengerID); q == nil || r == nil || boardedAt == nil || disembarkAt == nil {
		t.Fatalf("precondition: disembark columns must be set after boarding, got q=%v r=%v boarded_at=%v disembark_at=%v",
			q, r, boardedAt, disembarkAt)
	}

	if _, err := f.pool.Exec(ctx, `UPDATE transports SET status = 'intercepted' WHERE id = $1`, transportID); err != nil {
		t.Fatalf("simulate seizure: %v", err)
	}
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle (detect loss): %v", err)
	}

	q, r, boardedAt, disembarkAt := f.disembarkFields(t, messengerID)
	if q != nil || r != nil {
		t.Errorf("disembark_q/r after loss = %v,%v, want NULL", q, r)
	}
	if boardedAt != nil {
		t.Errorf("boarded_at after loss = %v, want NULL", boardedAt)
	}
	if disembarkAt != nil {
		t.Errorf("disembark_at after loss = %v, want NULL", disembarkAt)
	}
}

// TestPromoteSealed_DisembarkColumnsStayNull is the promotion path's own half
// of R2's lifecycle instruction ("promotionen tillbaka till awaiting_passage
// ... ska nolla alla fyra kolumnerna") — belt-and-suspenders, since
// sealLostCarrier already nulled them the instant the row left 'aboard': a
// freshly re-'awaiting_passage' messenger must not carry a stale prior
// voyage's disembark point forward into a NEW wait.
func TestPromoteSealed_DisembarkColumnsStayNull(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	messengerID := f.waitingMessenger(t, f.originID, f.currentTick+1)
	shipID := f.ship(t, f.originID, "merchantman")
	transportID := f.departingTransport(t, f.originID, shipID, f.currentTick+3)

	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle (board): %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE transports SET status = 'intercepted' WHERE id = $1`, transportID); err != nil {
		t.Fatalf("simulate seizure: %v", err)
	}
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle (seal): %v", err)
	}

	f.setTick(t, f.currentTick+PassageLostDelayTicks)
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle (promote): %v", err)
	}

	_, passageStatus, _, _ := f.messengerRow(t, messengerID)
	if passageStatus == nil || *passageStatus != "awaiting_passage" {
		t.Fatalf("passage_status after promotion = %v, want awaiting_passage", passageStatus)
	}
	q, r, boardedAt, disembarkAt := f.disembarkFields(t, messengerID)
	if q != nil || r != nil || boardedAt != nil || disembarkAt != nil {
		t.Errorf("disembark columns after promotion = q=%v r=%v boarded_at=%v disembark_at=%v, want all NULL",
			q, r, boardedAt, disembarkAt)
	}
}
