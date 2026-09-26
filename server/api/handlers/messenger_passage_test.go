package handlers

// DB integration tests for megaron_plan_budet_liftar.md (slice 3a): a
// messenger whose route needs sea runs to its own port and waits for a real
// carrier instead of crossing the abstract boat instantly. Same fixture
// family as messenger_trade_naval_test.go (navalPlayerFixture — two coastal
// settlements, a sea lane between).
//
// §5 acceptance covered here:
//  1. Land route (no sea) — regression: unchanged behaviour, event scheduled
//     immediately.
//  (sea, no port at all is covered directly in internal/messenger.)
//  2/3. Sea route with no ship anywhere: awaiting_passage at the sender's own
//     port; no arrival event yet.
//
// R2/R3/R4/R5's own mechanics (boarding, disembark scheduling, the reserve
// fallback, carrier loss) are proven directly against internal/messenger's
// PassageScanHandler in internal/messenger/passage_test.go — this file only
// proves the HTTP-layer wiring (Send) chooses the right mode and reports it.

import (
	"context"
	"testing"
)

// TestSend_LandRouteHasNoPassage is acceptance criterion 1 (regression): two
// settlements joined by a pure land strip get NO passage fields at all and
// the arrival event is scheduled immediately, exactly as before this slice.
func TestSend_LandRouteHasNoPassage(t *testing.T) {
	f := setupNavalPlayerFixture(t)
	originID := f.settlement(t, "Landward-Origin", 0, f.initiatorID, false)
	destID := f.settlement(t, "Landward-Dest", 3, f.initiatorID, false)
	for q := 1; q <= 2; q++ {
		f.mapTile(t, q, 0, "plains")
	}

	code, resp := f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+originID.String()+"/messengers",
		map[string]any{"destination_id": destID.String(), "message": "hello over land"})
	if code != 201 {
		t.Fatalf("Send status = %d, want 201 (%v)", code, resp)
	}
	if _, ok := resp["passage_status"]; ok {
		t.Errorf("response carries passage_status for a land route, want none: %v", resp)
	}

	var passageStatus *string
	var messengerID string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id, passage_status FROM messengers WHERE origin_id = $1 AND destination_id = $2`,
		originID, destID,
	).Scan(&messengerID, &passageStatus); err != nil {
		t.Fatalf("load messenger: %v", err)
	}
	if passageStatus != nil {
		t.Errorf("passage_status = %q, want NULL for a land route", *passageStatus)
	}

	var pending int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM scheduled_events WHERE event_type = 'MessengerArrival'
		   AND (payload->>'messenger_id') = $1`,
		messengerID,
	).Scan(&pending); err != nil {
		t.Fatalf("count scheduled arrival: %v", err)
	}
	if pending != 1 {
		t.Errorf("scheduled MessengerArrival count = %d, want exactly 1 (unaffected by this slice)", pending)
	}
}

// TestSend_SeaRouteNoShip_AwaitsPassageAtOwnPort is acceptance criteria 2/3's
// send-time half: no ship anywhere, so the messenger runs to its own coastal
// port (here, the origin itself is coastal — zero landward leg) and waits,
// and no MessengerArrival is scheduled yet.
func TestSend_SeaRouteNoShip_AwaitsPassageAtOwnPort(t *testing.T) {
	f := setupNavalPlayerFixture(t)
	sellerID := f.settlement(t, "Byblos", 0, f.initiatorID, true)
	buyerID := f.settlement(t, "Ugarit", 5, f.counterpartyID, true)
	for q := 1; q <= 4; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}

	code, resp := f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+sellerID.String()+"/messengers",
		map[string]any{"destination_id": buyerID.String(), "message": "hello over sea"})
	if code != 201 {
		t.Fatalf("Send status = %d, want 201 (%v)", code, resp)
	}
	if resp["passage_status"] != "awaiting_passage" {
		t.Fatalf("response passage_status = %v, want awaiting_passage", resp["passage_status"])
	}
	if resp["passage_port"] != "Byblos" {
		t.Errorf("response passage_port = %v, want Byblos (the sender's own coastal city)", resp["passage_port"])
	}

	var messengerID, passageStatus string
	var portID string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id, passage_status, passage_port_id FROM messengers WHERE origin_id = $1 AND destination_id = $2`,
		sellerID, buyerID,
	).Scan(&messengerID, &passageStatus, &portID); err != nil {
		t.Fatalf("load messenger: %v", err)
	}
	if passageStatus != "awaiting_passage" {
		t.Errorf("passage_status = %q, want awaiting_passage", passageStatus)
	}
	if portID != sellerID.String() {
		t.Errorf("passage_port_id = %s, want the sender's own city %s", portID, sellerID)
	}

	var pending int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM scheduled_events WHERE event_type = 'MessengerArrival'
		   AND (payload->>'messenger_id') = $1`,
		messengerID,
	).Scan(&pending); err != nil {
		t.Fatalf("count scheduled arrival: %v", err)
	}
	if pending != 0 {
		t.Errorf("scheduled MessengerArrival count = %d, want 0 (nothing scheduled until boarded or reserved)", pending)
	}
}
