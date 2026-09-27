package handlers

// DB integration test for the planner's review fix on 3b-4 (megaron_plan_
// ordna_passage.md, slice 3b-4, R5): a called-back messenger's return leg
// starts at the PORT it turned back from, never the destination it never
// reached — the road from destination to origin may not even exist (that is
// exactly why it needed a ship in the first place).
//
// TestLoadLiveEyes_CalledBackSeesPortNotFarShore is the RED-first case,
// through the REAL dispatch flow: POST .../messengers (ResolveDeparture) then
// POST .../call-back (messenger.CallBack), same naval fixture as
// messenger_passage_eye_test.go (setupNavalPlayerFixture).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/province"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestLoadLiveEyes_CalledBackSeesPortNotFarShore(t *testing.T) {
	f := setupNavalPlayerFixture(t)
	ctx := context.Background()

	// Inland (0,0), not coastal: the sender's settlement, with no port of its
	// own — ResolveDeparture routes it to a real owned port instead.
	inlandID := f.settlement(t, "Inland", 0, f.initiatorID, false)
	f.mapTile(t, 1, 0, "plains")
	// Port (2,0), the initiator's own coastal city — the port the runner
	// walks to, waits at, and — once called back — walks home FROM.
	portID := f.settlement(t, "Port", 2, f.initiatorID, true)
	for q := 3; q <= 6; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	// Ugarit (7,0), the counterparty's coastal city — the far shore the
	// runner never reached.
	ugaritID := f.settlement(t, "Ugarit", 7, f.counterpartyID, true)

	code, resp := f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+inlandID.String()+"/messengers",
		map[string]any{"destination_id": ugaritID.String(), "message": "hello over sea"})
	if code != 201 {
		t.Fatalf("Send status = %d, want 201 (%v)", code, resp)
	}
	var messengerID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`SELECT id FROM messengers WHERE origin_id = $1 AND destination_id = $2`,
		inlandID, ugaritID,
	).Scan(&messengerID); err != nil {
		t.Fatalf("load messenger: %v", err)
	}
	var passagePortID uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT passage_port_id FROM messengers WHERE id = $1`, messengerID).Scan(&passagePortID); err != nil {
		t.Fatalf("load passage_port_id: %v", err)
	}
	if passagePortID != portID {
		t.Fatalf("passage_port_id = %s, want the owned port %s", passagePortID, portID)
	}

	code, resp = f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/messengers/"+messengerID.String()+"/call-back", map[string]any{})
	if code != 200 {
		t.Fatalf("CallBack status = %d, want 200 (%v)", code, resp)
	}

	var status string
	var withdrawn bool
	if err := f.pool.QueryRow(ctx, `SELECT status, withdrawn FROM messengers WHERE id = $1`, messengerID).
		Scan(&status, &withdrawn); err != nil {
		t.Fatalf("load messenger after call-back: %v", err)
	}
	if status != "returning" || !withdrawn {
		t.Fatalf("messenger after call-back: status=%q withdrawn=%v, want returning/true", status, withdrawn)
	}

	// At the exact instant of the call-back, return_departs_at == now — the
	// interpolation's own progress is 0%, so the eye must sit at the leg's
	// START. Buggy: FindPath(destination(7,0) -> origin(0,0)) fails (sea
	// blocks the courier, 3b-4 R3) and the code falls back to sq,sr — the
	// destination (7,0) — verbatim. Fixed: the leg starts at the port (2,0).
	eyes := province.LoadLiveEyes(ctx, f.pool, f.worldID, f.initiatorID, f.clk.Now())
	var gotEye *province.Eye
	for i := range eyes {
		if eyes[i].Kind == province.EyeLandUnit {
			gotEye = &eyes[i]
		}
	}
	if gotEye == nil {
		t.Fatalf("no land-unit eye for the called-back runner; eyes = %+v", eyes)
	}
	if gotEye.Pos.Q == 7 && gotEye.Pos.R == 0 {
		t.Fatalf("runner eye sits at the far shore (7,0) it never reached — the return leg must start " +
			"at the port (2,0) it was called back from, not the destination")
	}
	if gotEye.Pos.Q != 2 || gotEye.Pos.R != 0 {
		t.Fatalf("runner eye = %+v, want the port (2,0) at the instant of call-back", gotEye.Pos)
	}
}

// TestMapMessengers_CalledBackShowsPortNotFarShore proves the SAME fix for
// MapMessengers' own-runner marker (the map's origin_q/origin_r for a
// 'returning' bud). It shares the exact code path a foreign viewer's sight
// check reads too (seesInterpolatedActor is called with this same, already-
// corrected m.OriginQ/m.OriginR, before the own/foreign branch) — so this one
// case proves both surfaces the planner named, not just the own marker.
func TestMapMessengers_CalledBackShowsPortNotFarShore(t *testing.T) {
	f := setupNavalPlayerFixture(t)
	ctx := context.Background()

	inlandID := f.settlement(t, "Inland", 0, f.initiatorID, false)
	f.mapTile(t, 1, 0, "plains")
	portID := f.settlement(t, "Port", 2, f.initiatorID, true)
	for q := 3; q <= 6; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	ugaritID := f.settlement(t, "Ugarit", 7, f.counterpartyID, true)

	code, resp := f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+inlandID.String()+"/messengers",
		map[string]any{"destination_id": ugaritID.String(), "message": "hello over sea"})
	if code != 201 {
		t.Fatalf("Send status = %d, want 201 (%v)", code, resp)
	}
	var messengerID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`SELECT id FROM messengers WHERE origin_id = $1 AND destination_id = $2`,
		inlandID, ugaritID,
	).Scan(&messengerID); err != nil {
		t.Fatalf("load messenger: %v", err)
	}

	code, resp = f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/messengers/"+messengerID.String()+"/call-back", map[string]any{})
	if code != 200 {
		t.Fatalf("CallBack status = %d, want 200 (%v)", code, resp)
	}

	// Same secret as setupNavalPlayerFixture's own auth.NewService — a fresh
	// instance validates the SAME bearer token (JWT signature check is
	// stateless), no need to widen the fixture's own struct just for this.
	authSvc := auth.NewService(f.pool, "test-secret")
	wh := NewWorldHandler(f.pool, authSvc, f.clk)
	r := chi.NewRouter()
	r.With(auth.Middleware(authSvc)).Get("/worlds/{worldID}/messengers", wh.MapMessengers)

	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/messengers", nil)
	req.Header.Set("Authorization", "Bearer "+f.initiatorToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("MapMessengers = %d %q, want 200", rec.Code, rec.Body.String())
	}
	var markers []struct {
		ID      uuid.UUID `json:"id"`
		OriginQ int       `json:"origin_q"`
		OriginR int       `json:"origin_r"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &markers); err != nil {
		t.Fatalf("decode markers: %v", err)
	}
	var found bool
	for _, m := range markers {
		if m.ID != messengerID {
			continue
		}
		found = true
		if m.OriginQ == 7 && m.OriginR == 0 {
			t.Fatalf("marker origin sits at the far shore (7,0) it never reached — same bug as LoadLiveEyes, "+
				"the return leg must start at the port (%s=2,0) it was called back from", portID)
		}
		if m.OriginQ != 2 || m.OriginR != 0 {
			t.Fatalf("marker origin = (%d,%d), want the port (2,0)", m.OriginQ, m.OriginR)
		}
	}
	if !found {
		t.Fatalf("called-back messenger %s not found in MapMessengers response", messengerID)
	}
}
