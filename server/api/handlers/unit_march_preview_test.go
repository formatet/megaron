package handlers

// Preview contract through the real authenticated HTTP routes and PostgreSQL.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"strings"
)

func TestMarchPreview_ReadOnlyAndTiming(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	const worldTick = 5
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-world-"+uuid.New().String(), worldTick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	username := "ticker-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, _ := authSvc.ValidateAccessToken(accessToken)
	playerID := claims.PlayerID

	// A capital keeps the player under the settlement cap so colonize is allowed.
	var capProvID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&capProvID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	var capSettID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Capital', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, capProvID, playerID,
	).Scan(&capSettID); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	// A unit GARRISONED at the capital: distance 0 to the commanding city, so
	// the march executes immediately (order-courier latency applies only to
	// field units — temenos_orderlopare_plan.md Fas 2) and the HTTP response
	// carries the K4 tick contract this test guards. One-hex march over plains
	// (0.75 h) still rounds to the 1-tick floor the assertions expect.
	for q := 0; q <= 1; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			worldID, q,
		); err != nil {
			t.Fatalf("create map tile (%d,0): %v", q, err)
		}
	}
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, settlement_id, type, category, size, status)
		 VALUES ($1, $2, $3, 'spearman', 'land', 100, 'garrison') RETURNING id`,
		worldID, playerID, capSettID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create garrisoned unit: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/units/{unitID}/march", uh.March)
	r.Get("/worlds/{worldID}/units/{unitID}/march-preview", uh.MarchPreview)

	base := "/worlds/" + worldID.String() + "/units/" + unitID.String()
	request := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	snapshot := func() string {
		t.Helper()
		var state string
		err := pool.QueryRow(ctx, `SELECT json_build_object(
   'units',(SELECT json_agg(u ORDER BY id) FROM units u WHERE world_id=$1),
   'goods',(SELECT json_agg(g ORDER BY good_key) FROM settlement_goods g WHERE settlement_id=$2),
   'jobs',(SELECT count(*) FROM scheduled_events WHERE world_id=$1),
   'events',(SELECT count(*) FROM events WHERE world_id=$1),
   'messengers',(SELECT count(*) FROM messengers WHERE world_id=$1))::text`, worldID, capSettID).Scan(&state)
		if err != nil {
			t.Fatal(err)
		}
		return state
	}
	before := snapshot()
	rec := request(base + "/march-preview?target_q=1&target_r=0")
	if rec.Code != 200 {
		t.Fatalf("preview %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("preview cached")
	}
	if snapshot() != before {
		t.Fatal("preview mutated game state")
	}
	var preview struct {
		Available     bool
		ArrivalTick   int       `json:"arrival_tick"`
		DurationTicks int       `json:"duration_ticks"`
		Arrives       time.Time `json:"arrives_at_utc"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if !preview.Available {
		t.Fatal("preview unavailable")
	}
	// Ownership is checked before terrain or ETA can be disclosed.
	otherToken, _, err := authSvc.Register(ctx, "other-"+uuid.New().String(), "x")
	if err != nil {
		t.Fatal(err)
	}
	otherReq := httptest.NewRequest(http.MethodGet, base+"/march-preview?target_q=1&target_r=0", nil)
	otherReq.Header.Set("Authorization", "Bearer "+otherToken)
	otherRec := httptest.NewRecorder()
	r.ServeHTTP(otherRec, otherReq)
	if otherRec.Code != 403 {
		t.Fatalf("foreign unit preview: %d", otherRec.Code)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,40,0,'plains')`, worldID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		query  string
		status int
		reason string
	}{
		{"?target_q=40&target_r=0", 422, ""},
		{"?target_q=40&target_r=0&intent=explore", 200, "unknown_terrain"},
		{"?target_q=bad&target_r=0", 400, ""},
	} {
		rec := request(base + "/march-preview" + tc.query)
		if rec.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.query, rec.Code, rec.Body.String())
		}
		if tc.reason != "" && (!strings.Contains(rec.Body.String(), tc.reason) || strings.Contains(rec.Body.String(), "arrival_tick")) {
			t.Fatalf("hidden preview: %s", rec.Body.String())
		}
	}
	// Positioned units receive a physical Runner, not an instant march ETA.
	if _, err := pool.Exec(ctx, `UPDATE units SET status='positioned',settlement_id=NULL,q=1,r=0 WHERE id=$1`, unitID); err != nil {
		t.Fatal(err)
	}
	before = snapshot()
	rec = request(base + "/march-preview?target_q=0&target_r=0")
	if snapshot() != before {
		t.Fatal("courier preview mutated game state")
	}
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "courier_required") || strings.Contains(rec.Body.String(), "arrival_tick") {
		t.Fatalf("courier preview: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := pool.Exec(ctx, `UPDATE units SET status='garrison',settlement_id=$2,q=NULL,r=NULL WHERE id=$1`, unitID, capSettID); err != nil {
		t.Fatal(err)
	}
	body := bytes.NewBufferString(`{"target_q":1,"target_r":0}`)
	req := httptest.NewRequest(http.MethodPost, base+"/march", body)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 202 {
		t.Fatalf("march: %d %s", rec.Code, rec.Body.String())
	}
	var actual struct {
		ArrivalTick   int       `json:"arrival_tick"`
		DurationTicks int       `json:"duration_ticks"`
		Arrives       time.Time `json:"arrives_at_utc"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &actual); err != nil {
		t.Fatal(err)
	}
	if preview.ArrivalTick != actual.ArrivalTick || preview.DurationTicks != actual.DurationTicks || !preview.Arrives.Equal(actual.Arrives) {
		t.Fatalf("preview %+v differs from actual %+v", preview, actual)
	}
}
