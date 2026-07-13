package handlers

// K4 API tick-contract regression tests. A march's timing is expressed in world
// ticks (the source of truth under the tick substrate, mig 067) on both the march
// response and unitSummary: arrival_tick + duration_ticks, plus a derived UTC
// convenience arrives_at_utc. These guard that (a) the March handler emits the
// three fields, (b) unitSummary derives them from the units.depart_tick/arrive_tick
// columns for a marching unit, and (c) a non-marching unit carries none of them.
//
// DB integration tests (real Postgres, gated by DATABASE_URL) via unitLoadTestPool.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/tick"
)

func TestMarch_ResponseCarriesTickContract(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
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
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
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
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Capital', 'achaean', $3, 'capital', true)`,
		worldID, capProvID, playerID,
	); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	// A field-positioned colonist that colonizes the hex it stands on: this
	// bypasses FindPath (zero-distance) so the test needs no tile graph, while
	// still exercising the March UPDATE + response with a real travel of 1 tick.
	const hexQ, hexR = 3, -1
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'plains')`,
		worldID, hexQ, hexR,
	); err != nil {
		t.Fatalf("create target tile: %v", err)
	}
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'positioned', $3, $4) RETURNING id`,
		worldID, playerID, hexQ, hexR,
	).Scan(&unitID); err != nil {
		t.Fatalf("create colonist: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/units/{unitID}/march", uh.March)

	body, _ := json.Marshal(map[string]any{"target_q": hexQ, "target_r": hexR, "intent": "colonize"})
	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+worldID.String()+"/units/"+unitID.String()+"/march", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("March = %d %q, want 202", rec.Code, rec.Body.String())
	}

	var resp struct {
		ArrivesAt     time.Time `json:"arrives_at"`
		ArrivalTick   *int      `json:"arrival_tick"`
		DurationTicks *int      `json:"duration_ticks"`
		ArrivesAtUTC  time.Time `json:"arrives_at_utc"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode march response: %v", err)
	}
	// A zero-distance colonize travels the floor of 1 tick.
	if resp.DurationTicks == nil || *resp.DurationTicks != 1 {
		t.Errorf("duration_ticks = %v, want 1", resp.DurationTicks)
	}
	if resp.ArrivalTick == nil || *resp.ArrivalTick != worldTick+1 {
		t.Errorf("arrival_tick = %v, want %d (current_tick %d + 1)", resp.ArrivalTick, worldTick+1, worldTick)
	}
	// arrives_at_utc is arrives_at normalised to UTC — the same instant.
	if !resp.ArrivesAtUTC.Equal(resp.ArrivesAt) {
		t.Errorf("arrives_at_utc %v is a different instant than arrives_at %v", resp.ArrivesAtUTC, resp.ArrivesAt)
	}
	if loc := resp.ArrivesAtUTC.Location(); loc != time.UTC {
		t.Errorf("arrives_at_utc location = %v, want UTC", loc)
	}

	// The tick columns must be persisted on the row so the read path can derive them.
	var departTick, arriveTick *int
	if err := pool.QueryRow(ctx, `SELECT depart_tick, arrive_tick FROM units WHERE id = $1`, unitID).
		Scan(&departTick, &arriveTick); err != nil {
		t.Fatalf("read tick columns: %v", err)
	}
	if departTick == nil || *departTick != worldTick {
		t.Errorf("units.depart_tick = %v, want %d", departTick, worldTick)
	}
	if arriveTick == nil || *arriveTick != worldTick+1 {
		t.Errorf("units.arrive_tick = %v, want %d", arriveTick, worldTick+1)
	}
}

func TestUnitSummary_TickContractDerivation(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	const worldTick = 20
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-world-"+uuid.New().String(), worldTick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	username := "reader-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, _ := authSvc.ValidateAccessToken(accessToken)
	playerID := claims.PlayerID

	// A marching unit with tick columns set (target left NULL so the summary's
	// path attach is skipped — irrelevant to the tick derivation under test).
	const departTick, arriveTick = 10, 25
	var marchingID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r, depart_tick, arrive_tick)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'marching', 4, -2, $3, $4) RETURNING id`,
		worldID, playerID, departTick, arriveTick,
	).Scan(&marchingID); err != nil {
		t.Fatalf("create marching unit: %v", err)
	}
	// A garrison unit with no tick columns.
	var garrisonID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'garrison') RETURNING id`,
		worldID, playerID,
	).Scan(&garrisonID); err != nil {
		t.Fatalf("create garrison unit: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Get("/worlds/{worldID}/units", uh.ListUnits)

	req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/units", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListUnits = %d %q, want 200", rec.Code, rec.Body.String())
	}

	var resp struct {
		Units []struct {
			ID            string     `json:"id"`
			Status        string     `json:"status"`
			ArrivalTick   *int       `json:"arrival_tick"`
			DurationTicks *int       `json:"duration_ticks"`
			ArrivesAtUTC  *time.Time `json:"arrives_at_utc"`
		} `json:"units"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode units response: %v", err)
	}

	var marching, garrison *struct {
		ID            string     `json:"id"`
		Status        string     `json:"status"`
		ArrivalTick   *int       `json:"arrival_tick"`
		DurationTicks *int       `json:"duration_ticks"`
		ArrivesAtUTC  *time.Time `json:"arrives_at_utc"`
	}
	for i := range resp.Units {
		switch resp.Units[i].ID {
		case marchingID.String():
			marching = &resp.Units[i]
		case garrisonID.String():
			garrison = &resp.Units[i]
		}
	}
	if marching == nil || garrison == nil {
		t.Fatalf("expected both units in response, got %d units", len(resp.Units))
	}

	// Marching unit: derived from the row's tick columns.
	if marching.ArrivalTick == nil || *marching.ArrivalTick != arriveTick {
		t.Errorf("marching arrival_tick = %v, want %d", marching.ArrivalTick, arriveTick)
	}
	if marching.DurationTicks == nil || *marching.DurationTicks != arriveTick-departTick {
		t.Errorf("marching duration_ticks = %v, want %d", marching.DurationTicks, arriveTick-departTick)
	}
	// arrives_at_utc = now + (arrive_tick − current_tick) real ticks, in UTC.
	wantUTC := clk.Now().Add(time.Duration(arriveTick-worldTick) * time.Duration(tick.TickSeconds) * time.Second).UTC()
	if marching.ArrivesAtUTC == nil || !marching.ArrivesAtUTC.Equal(wantUTC) {
		t.Errorf("marching arrives_at_utc = %v, want %v", marching.ArrivesAtUTC, wantUTC)
	}

	// Garrison unit: no march timing at all.
	if garrison.ArrivalTick != nil || garrison.DurationTicks != nil || garrison.ArrivesAtUTC != nil {
		t.Errorf("garrison unit leaked march timing: arrival_tick=%v duration_ticks=%v arrives_at_utc=%v",
			garrison.ArrivalTick, garrison.DurationTicks, garrison.ArrivesAtUTC)
	}
}
