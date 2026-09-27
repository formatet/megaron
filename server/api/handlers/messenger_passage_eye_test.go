package handlers

// DB integration tests for megaron_plan_ordna_passage.md slice 3b-1: a
// messenger's own live-vision eye (province.LoadLiveEyes) and its map marker
// (MapMessengers) must follow the LEG it is actually on, not the flat
// origin→final-target courier interpolation, which assumes an unbroken
// crossing. passage_status splits that assumption:
//
//   - awaiting_passage: physically standing at its port. arrives_at is when
//     it reached the port (already in the past while it waits) — the eye must
//     sit at the port, never at the far-shore destination.
//   - aboard / returning_sealed: sealed to/from a carrier. The runner reveals
//     nothing itself.
//   - NULL: a purely land courier — unaffected (regression).
//
// TestLoadLiveEyes_AwaitingPassageSeesPortNotFarShore is the RED-first case
// (acceptance 1): it goes through the REAL dispatch flow (POST .../messengers
// -> messenger.ResolveDeparture) so the awaiting_passage row it produces is
// exactly what the game itself creates, not a hand-picked end state — same
// naval fixture as messenger_passage_test.go (setupNavalPlayerFixture).

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
)

// TestLoadLiveEyes_AwaitingPassageSeesPortNotFarShore is acceptance 1: an
// awaiting_passage messenger whose arrives_at (the landward leg's own arrival,
// at the port) has long passed must NOT give an eye at its final destination
// across the sea. On master this is red — LoadLiveEyes gives the far-shore
// eye described in megaron_plan_ordna_passage.md's "Gemensamt kodläge".
func TestLoadLiveEyes_AwaitingPassageSeesPortNotFarShore(t *testing.T) {
	f := setupNavalPlayerFixture(t)
	ctx := context.Background()

	// Inland (0,0), not coastal: the sender's settlement, with no port of its
	// own — ResolveDeparture must route it to a real owned port instead.
	inlandID := f.settlement(t, "Inland", 0, f.initiatorID, false)
	f.mapTile(t, 1, 0, "plains")
	// Port (2,0), the initiator's own coastal city — the port the runner
	// walks to and waits at.
	portID := f.settlement(t, "Port", 2, f.initiatorID, true)
	// Sea between the port and the foreign destination.
	for q := 3; q <= 6; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	// Ugarit (7,0), the counterparty's coastal city — the far shore.
	ugaritID := f.settlement(t, "Ugarit", 7, f.counterpartyID, true)

	code, resp := f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+inlandID.String()+"/messengers",
		map[string]any{"destination_id": ugaritID.String(), "message": "hello over sea"})
	if code != 201 {
		t.Fatalf("Send status = %d, want 201 (%v)", code, resp)
	}
	if resp["passage_status"] != "awaiting_passage" {
		t.Fatalf("response passage_status = %v, want awaiting_passage", resp["passage_status"])
	}
	if resp["passage_port"] != "Port" {
		t.Fatalf("response passage_port = %v, want Port", resp["passage_port"])
	}

	var passagePortID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`SELECT passage_port_id FROM messengers WHERE origin_id = $1 AND destination_id = $2`,
		inlandID, ugaritID,
	).Scan(&passagePortID); err != nil {
		t.Fatalf("load messenger: %v", err)
	}
	if passagePortID != portID {
		t.Fatalf("passage_port_id = %s, want the owned port %s", passagePortID, portID)
	}

	// Let a long wait pass in port — arrives_at (the landward arrival, already
	// in the past) stays exactly as Send wrote it; nothing updates it again
	// until a carrier is boarded or the reserve is taken.
	tc, ok := f.clk.(*clock.TestClock)
	if !ok {
		t.Fatalf("fixture clock is not a *clock.TestClock: %T", f.clk)
	}
	tc.Advance(24 * time.Hour)

	eyes := province.LoadLiveEyes(ctx, f.pool, f.worldID, f.initiatorID, f.clk.Now())
	var gotEye *province.Eye
	for i := range eyes {
		if eyes[i].Kind == province.EyeLandUnit {
			gotEye = &eyes[i]
		}
	}
	if gotEye == nil {
		t.Fatalf("no land-unit eye for the waiting runner; eyes = %+v", eyes)
	}
	if gotEye.Pos.Q == 7 && gotEye.Pos.R == 0 {
		t.Fatalf("runner eye sits at the far shore (7,0) — the FOW leak this slice fixes: "+
			"awaiting_passage must place the eye at its port (2,0), not the destination across the sea")
	}
	if gotEye.Pos.Q != 2 || gotEye.Pos.R != 0 {
		t.Fatalf("runner eye = %+v, want the port (2,0)", gotEye.Pos)
	}
}
