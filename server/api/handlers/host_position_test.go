package handlers

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
)

func TestHostPhysicalPositionStatusEyesAndFounding(t *testing.T) {
	f, r := setupNomadicHostRecallWorld(t)
	pool := unitLoadTestPool(t)
	ctx := context.Background()
	clk := clock.NewTestClock(f.now)
	// Weighted route: at tick3, Host stands in (2,0), not departure (0,0).
	route := `{"start_tick":0,"end_tick":6,"hexes":[[0,0],[1,0],[2,0],[3,0]],"costs":[1000,3000,2000]}`
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=3,last_tick_at=$2 WHERE id=$1`, f.worldID, f.now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE units SET target_q=3,depart_tick=0,arrive_tick=6,march_route=$2,departs_at=$3,arrives_at=$4 WHERE id=$1`, f.hostID, route, f.now.Add(-3*time.Hour), f.now.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE units SET status='disbanded',q=NULL,r=NULL WHERE id=$1`, f.spearmanID); err != nil {
		t.Fatal(err)
	}
	// Complete current site's catchment. No injected state after the API order.
	for q := -1; q <= 5; q++ {
		for rr := -2; rr <= 2; rr++ {
			if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,$3,'plains') ON CONFLICT(world_id,q,r) DO NOTHING`, f.worldID, q, rr); err != nil {
				t.Fatal(err)
			}
		}
	}
	store := events.NewStore(pool)
	scheduler := events.NewScheduler(pool, clk)
	jh := NewJoinHandler(pool, store, clk, nil)
	uh := NewUnitHandler(pool, scheduler, store, clk)
	r.Get("/worlds/{worldID}/founding/status", jh.FoundingStatus)
	r.Post("/worlds/{worldID}/founding/settle", jh.Settle)
	r.Get("/worlds/{worldID}/units", uh.ListUnits)
	base := "/worlds/" + f.worldID.String()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, base+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+f.accessToken)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	assertState := func(wantStatus string, q, rr int, canFound bool) {
		t.Helper()
		rec := request("GET", "/founding/status", "")
		if rec.Code != 200 {
			t.Fatalf("status %d %s", rec.Code, rec.Body.String())
		}
		var fp struct {
			Q, R  int
			State hostMapState `json:"host_state"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &fp); err != nil {
			t.Fatal(err)
		}
		if fp.Q != q || fp.R != rr || fp.State.Q != q || fp.State.R != rr || fp.State.Status != wantStatus || fp.State.CanFoundHere != canFound {
			t.Fatalf("founding position/state=%s, want (%d,%d)/%s canFound=%v", rec.Body, q, rr, wantStatus, canFound)
		}
		listed := request("GET", "/units", "")
		if listed.Code != 200 {
			t.Fatalf("units %d %s", listed.Code, listed.Body.String())
		}
		var result struct {
			Units []unitSummary `json:"units"`
		}
		if err := json.Unmarshal(listed.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, u := range result.Units {
			if u.ID == f.hostID {
				found = true
				if u.HostState == nil || *u.HostState != fp.State || u.CurrentQ == nil || *u.CurrentQ != q || u.CurrentR == nil || *u.CurrentR != rr {
					t.Fatalf("marker projection disagrees with founding: %+v / %+v", u.HostState, fp.State)
				}
			}
		}
		if !found {
			t.Fatal("host missing")
		}
		eyes := loadLiveEyes(ctx, pool, f.worldID, f.playerID, clk.Now())
		if len(eyes) != 1 || eyes[0].Pos != (province.MapPosition{Q: q, R: rr}) {
			t.Fatalf("eyes=%+v, want Host only at (%d,%d)", eyes, q, rr)
		}
	}
	assertState("marching", 2, 0, true)
	// Read projections never reprice or rewrite the saved march.
	var unchanged bool
	if err := pool.QueryRow(ctx, `SELECT q=0 AND r=0 AND march_route=$2::jsonb AND depart_tick=0 AND arrive_tick=6 FROM units WHERE id=$1`, f.hostID, route).Scan(&unchanged); err != nil || !unchanged {
		t.Fatalf("saved march changed: %v %v", unchanged, err)
	}
	// A stationary invalid site waits; a valid site is explicitly ready.
	if _, err := pool.Exec(ctx, `UPDATE units SET status='positioned',q=2,r=0 WHERE id=$1`, f.hostID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE map_tiles SET terrain='semi_desert' WHERE world_id=$1 AND q=2 AND r=0`, f.worldID); err != nil {
		t.Fatal(err)
	}
	assertState("waiting", 2, 0, false)
	rec := request("POST", "/founding/settle", `{}`)
	if rec.Code != 422 {
		t.Fatalf("invalid site accepted %d %s", rec.Code, rec.Body.String())
	}
	if _, err := pool.Exec(ctx, `UPDATE map_tiles SET terrain='plains' WHERE world_id=$1 AND q=2 AND r=0`, f.worldID); err != nil {
		t.Fatal(err)
	}
	assertState("can_found_here", 2, 0, true)
	// Actual moving settlement, not a stationary fixture: restore the same
	// saved march, then the player sends only the ordinary settle request.
	if _, err := pool.Exec(ctx, `UPDATE units SET status='marching',q=0,r=0 WHERE id=$1`, f.hostID); err != nil {
		t.Fatal(err)
	}
	assertState("marching", 2, 0, true)
	rec = request("POST", "/founding/settle", `{"expected_q":0,"expected_r":0}`)
	if rec.Code != 409 {
		t.Fatalf("stale founding position accepted: %d %s", rec.Code, rec.Body.String())
	}
	rec = request("POST", "/founding/settle", `{"name":"Physical metropolis","expected_q":2,"expected_r":0}`)
	if rec.Code != 201 {
		t.Fatalf("settle %d %s", rec.Code, rec.Body.String())
	}
	var q, rr int
	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT s.id,p.map_q,p.map_r FROM settlements s JOIN provinces p ON p.id=s.province_id WHERE s.world_id=$1 AND s.owner_id=$2`, f.worldID, f.playerID).Scan(&settlementID, &q, &rr); err != nil {
		t.Fatal(err)
	}
	if q != 2 || rr != 0 {
		t.Fatalf("metropolis founded at (%d,%d), want actual (2,0), not departure (0,0)", q, rr)
	}
	after := request("GET", "/founding/status", "")
	if !strings.Contains(after.Body.String(), `"active":false`) {
		t.Fatal(after.Body.String())
	}
	// The queued arrival (and retry) after dissolution cannot resurrect the Host.
	handler := combat.NewUnitArrivalHandler(pool, store, nil, scheduler, clk)
	arrive := 6
	payload, _ := json.Marshal(unit.ScheduledUnitArrivalPayload{UnitID: f.hostID, WorldID: f.worldID, ArriveTick: &arrive})
	for i := 0; i < 2; i++ {
		if err := handler.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: payload}); err != nil {
			t.Fatal(err)
		}
	}
	var status string
	var active bool
	if err := pool.QueryRow(ctx, `SELECT u.status,fp.active FROM units u JOIN founder_phase fp ON fp.world_id=u.world_id AND fp.owner_id=u.owner_id WHERE u.id=$1`, f.hostID).Scan(&status, &active); err != nil {
		t.Fatal(err)
	}
	if status != "disbanded" || active {
		t.Fatalf("Host resurrected: %s active=%v", status, active)
	}
	t.Logf("weighted Host: departure(0,0), marker/API/eyes(2,0), actual metropolis(2,0); old arrival and retry harmless")
}
