package handlers

// Slice 16 — demolish a building ("absolut kunna riva för att bygga om").
// Real Postgres, gated by DATABASE_URL (same harness as province_build_hex_test.go).

import (
	"context"
	"net/http"
	"testing"

	"formatet/megaron/server/internal/auth"
	"github.com/google/uuid"
)

func demolishPath(f *mineGateFixture, id any) string {
	return "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceID.String() + "/buildings/" + toStr(id)
}

func toStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case uuid.UUID:
		return x.String()
	}
	return ""
}

// seedQuarryWithCrew puts a finished stonequarry on hills hex (1,0) and five
// gubbar on its stone (hills: 2 base places, 6 with the quarry).
func seedQuarryWithCrew(t *testing.T, f *mineGateFixture) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE map_tiles SET terrain = 'hills' WHERE world_id = $1 AND q = 1 AND r = 0`, f.worldID); err != nil {
		t.Fatalf("hills: %v", err)
	}
	var id uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level, hex_q, hex_r) VALUES ($1, 'stonequarry', 1, 1, 0) RETURNING id`,
		f.settlementID).Scan(&id); err != nil {
		t.Fatalf("seed quarry: %v", err)
	}
	for o := 1; o <= 5; o++ {
		if _, err := f.pool.Exec(ctx,
			`INSERT INTO settlement_placement (settlement_id, gubbe_ordinal, target_kind, hex_q, hex_r, good_key)
			 VALUES ($1, $2, 'hex', 1, 0, 'stone')`, f.settlementID, o); err != nil {
			t.Fatalf("seed placement %d: %v", o, err)
		}
	}
	return id
}

func countRows(t *testing.T, f *mineGateFixture, q string) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), q, f.settlementID).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func TestDemolishBuilding_FreesHexLiftsCitizensAndIsIdempotent(t *testing.T) {
	f := buildHexFixture(t)
	ctx := context.Background()
	id := seedQuarryWithCrew(t, f)

	code, resp := f.do(t, http.MethodDelete, demolishPath(f, id), nil)
	if code != http.StatusOK {
		t.Fatalf("demolish = %d: %v, want 200", code, resp)
	}
	if got := int(resp["citizens_lifted"].(float64)); got != 3 {
		t.Errorf("citizens_lifted = %d, want 3 (5 stone workers, 2 base places remain)", got)
	}
	if n := countRows(t, f, `SELECT count(*) FROM buildings WHERE settlement_id = $1`); n != 0 {
		t.Errorf("buildings left = %d, want 0", n)
	}
	// Lifted gubbar are idle, not gone: population untouched, 2 still work the hex.
	if n := countRows(t, f, `SELECT count(*) FROM settlement_placement WHERE settlement_id = $1`); n != 2 {
		t.Errorf("placements left = %d, want 2", n)
	}
	if n := countRows(t, f, `SELECT population FROM settlements WHERE id = $1`); n != 500 {
		t.Errorf("population = %d, want 500", n)
	}
	// Audit event with the outcome.
	if n := countRows(t, f, `SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'BuildingDemolished' AND (payload->>'citizens_lifted')::int = 3`); n != 1 {
		t.Errorf("BuildingDemolished events = %d, want 1", n)
	}

	// The hex is free: a DIFFERENT building goes in at once.
	code, resp = f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "farm", "hex_q": 1, "hex_r": 0})
	if code != http.StatusCreated {
		t.Fatalf("farm on freed hex = %d: %v, want 201", code, resp)
	}

	// Second demolish of the same id: 404, no change.
	code, resp = f.do(t, http.MethodDelete, demolishPath(f, id), nil)
	if code != http.StatusNotFound {
		t.Fatalf("second demolish = %d: %v, want 404", code, resp)
	}
	code, _ = f.do(t, http.MethodDelete, demolishPath(f, uuid.New()), nil)
	if code != http.StatusNotFound {
		t.Fatalf("unknown id = %d, want 404", code)
	}
	_ = ctx
}

func TestDemolishBuilding_OtherWanaxGets403(t *testing.T) {
	f := buildHexFixture(t)
	id := seedQuarryWithCrew(t, f)

	other := auth.NewService(f.pool, "test-secret")
	tok, _, err := other.Register(context.Background(), "intruder-"+uuid.New().String(), "x")
	if err != nil {
		t.Fatalf("register intruder: %v", err)
	}
	mine := f.accessToken
	f.accessToken = tok
	code, resp := f.do(t, http.MethodDelete, demolishPath(f, id), nil)
	f.accessToken = mine
	if code != http.StatusForbidden {
		t.Fatalf("demolish in someone else's city = %d: %v, want 403", code, resp)
	}
	if n := countRows(t, f, `SELECT count(*) FROM buildings WHERE settlement_id = $1`); n != 1 {
		t.Errorf("building must survive a forbidden demolish, have %d", n)
	}
	if n := countRows(t, f, `SELECT count(*) FROM settlement_placement WHERE settlement_id = $1`); n != 5 {
		t.Errorf("placements must be untouched, have %d", n)
	}
}

func TestDemolishBuilding_QueuedBuildIsCancelled(t *testing.T) {
	f := buildHexFixture(t)
	code, resp := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "farm", "hex_q": 1, "hex_r": 0})
	if code != http.StatusCreated {
		t.Fatalf("queue farm = %d: %v", code, resp)
	}
	qid, _ := resp["queue_id"].(string)
	code, resp = f.do(t, http.MethodDelete, demolishPath(f, qid), nil)
	if code != http.StatusOK || resp["cancelled"] != true {
		t.Fatalf("demolish queued = %d: %v, want 200 cancelled", code, resp)
	}
	if n := countRows(t, f, `SELECT count(*) FROM build_queue WHERE settlement_id = $1`); n != 0 {
		t.Errorf("queue rows left = %d, want 0", n)
	}
}
