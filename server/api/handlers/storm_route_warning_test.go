package handlers

// Known storms on a sea route (order stormvarning-rutt, Timothy 2026-11-18): the forecast of a
// sea voyage names a storm the Wanax sees now or remembers, and NEVER one they have not seen.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func lane(from, to, r int) []province.MapPosition {
	var out []province.MapPosition
	for q := from; q <= to; q++ {
		out = append(out, province.MapPosition{Q: q, R: r})
	}
	return out
}

func laneTiles(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID, route []province.MapPosition, terrain string) {
	t.Helper()
	for _, hx := range route {
		if _, err := pool.Exec(context.Background(),
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
			worldID, hx.Q, hx.R, terrain); err != nil {
			t.Fatal(err)
		}
	}
}

func routeLine(t *testing.T, pool *pgxpool.Pool, worldID, playerID uuid.UUID, route []province.MapPosition) string {
	t.Helper()
	return stormForecast(context.Background(), pool, clock.NewTestClock(time.Now()), worldID, playerID, "", route)
}

// A storm in sight (live), a neighbour of the route: named with its coordinate and heading.
func TestStormRoute_LiveStormNextToRouteIsNamed(t *testing.T) {
	pool, worldID, viewerID, _, _ := stormsRig(t)
	route := lane(1, 6, 0)
	laneTiles(t, pool, worldID, route, "coastal_sea")
	seedStorm(t, pool, worldID, 1, [3][2]int{{2, 1}, {3, 1}, {2, 2}}) // r=1: a neighbour of the r=0 route, in sight
	got := routeLine(t, pool, worldID, viewerID, route)
	want := fmt.Sprintf("Storm on this route at (2, 1), drifting NE. A storm takes %d hull.", combat.StormDamage)
	if got != want {
		t.Fatalf("forecast = %q, want %q", got, want)
	}
}

// A storm the Wanax saw before and remembers, lying on the route: coordinate and age.
func TestStormRoute_RememberedStormOnRouteSaysGameDaysAgo(t *testing.T) {
	pool, worldID, viewerID, _, _ := stormsRig(t) // world tick 7
	route := lane(1, 16, 0)
	laneTiles(t, pool, worldID, route, "coastal_sea")
	id := seedStorm(t, pool, worldID, 0, [3][2]int{{20, 5}, {21, 5}, {20, 6}}) // where it truly is now
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO player_storm_sightings (player_id, storm_id, seen_tick, hexes) VALUES ($1, $2, 4, '[{"q":12,"r":0},{"q":13,"r":0},{"q":12,"r":1}]')`,
		viewerID, id); err != nil {
		t.Fatal(err)
	}
	got := routeLine(t, pool, worldID, viewerID, route)
	if !strings.HasPrefix(got, "Storm seen on this route at (12, 0), 3 game days ago.") || !strings.Contains(got, fmt.Sprintf("takes %d hull", combat.StormDamage)) {
		t.Fatalf("forecast = %q", got)
	}
}

// THE CANON GATE: a storm on the route that the Wanax neither sees nor remembers is never mentioned.
func TestStormRoute_UnseenStormOnRouteIsNeverNamed(t *testing.T) {
	pool, worldID, viewerID, _, _ := stormsRig(t)
	route := lane(1, 16, 0)
	laneTiles(t, pool, worldID, route, "coastal_sea")
	seedStorm(t, pool, worldID, 0, [3][2]int{{12, 0}, {13, 0}, {12, 1}}) // squarely on the route, far out of sight, never seen
	if got := routeLine(t, pool, worldID, viewerID, route); got != "" {
		t.Fatalf("an unseen storm leaked into the forecast: %q", got)
	}
}

// Land hexes are not touched by storms: a land march past a coastal storm gets no line.
func TestStormRoute_LandRouteGetsNoLine(t *testing.T) {
	pool, worldID, viewerID, _, _ := stormsRig(t)
	route := lane(1, 4, 0)
	laneTiles(t, pool, worldID, route, "plains")
	seedStorm(t, pool, worldID, 1, [3][2]int{{2, 1}, {3, 1}, {2, 2}}) // in sight, next to the land route
	// seedStorm made its own hexes sea; the route hexes stay plains (ON CONFLICT DO NOTHING above ran first)
	if got := routeLine(t, pool, worldID, viewerID, route); got != "" {
		t.Fatalf("a land route got a storm line: %q", got)
	}
}

// Several known storms: the nearest along the voyage is named, then "+N more".
func TestStormRoute_SeveralStormsNameFirstAndCountRest(t *testing.T) {
	pool, worldID, viewerID, _, _ := stormsRig(t)
	route := lane(1, 6, 0)
	laneTiles(t, pool, worldID, route, "coastal_sea")
	seedStorm(t, pool, worldID, 1, [3][2]int{{2, 1}, {3, 1}, {2, 2}})
	seedStorm(t, pool, worldID, 0, [3][2]int{{3, -1}, {4, -1}, {3, -2}})
	got := routeLine(t, pool, worldID, viewerID, route)
	if !strings.Contains(got, "(+1 more)") || strings.Count(got, "on this route") != 1 {
		t.Fatalf("forecast = %q, want one line with (+1 more)", got)
	}
}

// ---- one test per surface: the field is actually filled (produced != consumed) ----

func seaFixtureWithLiveStorm(t *testing.T) (*passageArrangeFixture, uuid.UUID) {
	f := setupPassageArrangeFixture(t)
	homeID := f.settlement(t, "Storm-Home", 0, f.initiatorID, true)
	f.settlement(t, "Storm-Foreign", 5, f.counterpartyID, true)
	for q := 1; q <= 4; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	seedStorm(t, f.pool, f.worldID, 1, [3][2]int{{2, 1}, {3, 1}, {2, 2}}) // live from home, beside the lane
	return f, homeID
}

func (f *navalPlayerFixture) getJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+f.initiatorToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	out["_code"] = float64(rec.Code)
	return out
}

func wantRouteStorm(t *testing.T, surface string, v any) {
	t.Helper()
	got, _ := v.(string)
	if !strings.Contains(got, "Storm on this route at (") || !strings.Contains(got, "drifting NE") {
		t.Errorf("%s: storm_warning = %q, want the live-storm route line", surface, got)
	}
}

func TestStormRoute_MarchPreviewAndReceipt(t *testing.T) {
	f, homeID := seaFixtureWithLiveStorm(t)
	f.good(t, homeID, "grain", 100000)
	ship := f.ship(t, homeID, f.initiatorID, "galley")
	uh := NewUnitHandler(f.pool, events.NewScheduler(f.pool, f.clk), events.NewStore(f.pool), f.clk)
	f.router.Get("/worlds/{worldID}/units/{unitID}/march-preview", uh.MarchPreview)
	f.router.Post("/worlds/{worldID}/units/{unitID}/march", uh.March)
	base := "/worlds/" + f.worldID.String() + "/units/" + ship.String()

	prev := f.getJSON(t, base+"/march-preview?target_q=4&target_r=0&intent=patrol")
	if prev["available"] != true {
		t.Fatalf("preview unavailable: %v", prev)
	}
	wantRouteStorm(t, "march-preview", prev["storm_warning"])

	code, rec := f.post(t, f.initiatorToken, base+"/march", map[string]any{"target_q": 4, "target_r": 0, "intent": "patrol"})
	if code != 202 {
		t.Fatalf("march %d %v", code, rec)
	}
	wantRouteStorm(t, "march receipt", rec["storm_warning"])
}

func TestStormRoute_ExploreGetsNoRouteLine(t *testing.T) {
	f, homeID := seaFixtureWithLiveStorm(t)
	f.good(t, homeID, "grain", 100000)
	ship := f.ship(t, homeID, f.initiatorID, "galley")
	if _, err := f.pool.Exec(context.Background(), `UPDATE units SET hull=1 WHERE id=$1`, ship); err != nil {
		t.Fatal(err)
	}
	uh := NewUnitHandler(f.pool, events.NewScheduler(f.pool, f.clk), events.NewStore(f.pool), f.clk)
	f.router.Get("/worlds/{worldID}/units/{unitID}/march-preview", uh.MarchPreview)
	f.router.Post("/worlds/{worldID}/units/{unitID}/march", uh.March)
	base := "/worlds/" + f.worldID.String() + "/units/" + ship.String()

	// The hull line still shows (the field is live), the route line does not: the route is hidden.
	prev := f.getJSON(t, base+"/march-preview?target_q=4&target_r=0&intent=explore&ticks=12")
	if w, _ := prev["storm_warning"].(string); !strings.Contains(w, "will sink it") || strings.Contains(w, "on this route") {
		t.Errorf("explore preview storm_warning = %q (code %v), want the hull line only", w, prev["_code"])
	}
	code, rec := f.post(t, f.initiatorToken, base+"/march", map[string]any{"target_q": 4, "target_r": 0, "intent": "explore", "ticks": 12})
	if code != 202 {
		t.Fatalf("explore march %d %v", code, rec)
	}
	if w, _ := rec["storm_warning"].(string); !strings.Contains(w, "will sink it") || strings.Contains(w, "on this route") {
		t.Errorf("explore receipt storm_warning = %q, want the hull line only", w)
	}
}

func TestStormRoute_PassageReceipt(t *testing.T) {
	f, homeID := seaFixtureWithLiveStorm(t)
	foreign := f.lookupSettlement(t, "Storm-Foreign")
	ship := f.ship(t, homeID, f.initiatorID, "merchantman")
	code, sendResp := f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+homeID.String()+"/messengers",
		map[string]any{"destination_id": foreign.String(), "message": "ahoy"})
	if code != 201 {
		t.Fatalf("send %d %v", code, sendResp)
	}
	var messengerID uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `SELECT id FROM messengers WHERE origin_id = $1`, homeID).Scan(&messengerID); err != nil {
		t.Fatal(err)
	}
	code, resp := f.post(t, f.initiatorToken, "/worlds/"+f.worldID.String()+"/messengers/"+messengerID.String()+"/passage", map[string]any{"ship_id": ship.String()})
	if code != 202 {
		t.Fatalf("passage %d %v", code, resp)
	}
	wantRouteStorm(t, "passage receipt", resp["storm_warning"])
}

func (f *navalPlayerFixture) lookupSettlement(t *testing.T, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `SELECT id FROM settlements WHERE world_id=$1 AND name=$2`, f.worldID, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestStormRoute_TradePreviewAndTransferReceipt(t *testing.T) {
	f := setupTradeNavalFixture(t)
	origin, prov := f.settlement(t, "Origin", 0, true, true)
	dest, _ := f.settlement(t, "Destination", 5, true, false)
	for q := 1; q < 5; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	f.ship(t, origin, "merchantman")
	seedStorm(t, f.pool, f.worldID, 1, [3][2]int{{2, 1}, {3, 1}, {2, 2}})
	h := NewProvinceHandler(f.pool, f.scheduler, f.clk, events.NewStore(f.pool), nil)
	f.router.Get("/worlds/{worldID}/provinces/{provinceID}/trade/preview", h.TradePreview)
	req := httptest.NewRequest("GET", fmt.Sprintf("/worlds/%s/provinces/%s/trade/preview?destination_id=%s&quantity=10&by=sea", f.worldID, prov, dest), nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("preview %d %s", rec.Code, rec.Body.String())
	}
	var prev map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &prev)
	wantRouteStorm(t, "trade preview", prev["storm_warning"])

	code, body := f.post(t, fmt.Sprintf("/worlds/%s/provinces/%s/trade", f.worldID, prov), map[string]any{"destination_id": dest, "good_key": "grain", "quantity": 10, "by": "sea"})
	if code != 201 {
		t.Fatalf("dispatch %d %+v", code, body)
	}
	wantRouteStorm(t, "transfer receipt", body["storm_warning"])
}
