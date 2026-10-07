package handlers

// Acceptance 2 (megaron_plan_rorelse_sparad_vag.md §6), read site (c): the
// Recall handler's precheck (unit.go ~586) must resolve the marching unit's
// CURRENT position through the saved route, exactly like combat.ExecuteRecall
// does (proven by combat's TestAcceptance2_*_CatchesWhereTimeSays). The precheck's currentPos itself
// isn't returned in the HTTP response, so this test makes it OBSERVABLE a
// different way: two settlements are placed so that "nearest to the OLD
// (origin) position" and "nearest to the CORE's (ford) position" pick
// DIFFERENT settlements — resolveOrderOrigin's own nearest-settlement choice
// then reveals which currentPos the precheck actually used, via the
// dispatched order messenger's origin_id.

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
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/tick"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestAcceptance2_RecallPrecheck_CatchesWhereTimeSays(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	accessToken, _, err := authSvc.Register(ctx, "ford-recall-precheck-"+uuid.New().String(), "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	// The ford fixture: (0,0) plains (origin) -> (1,0) river_ford (dominates
	// the cost) -> (2,0) plains (target). Plus (-1,0), adjacent ONLY to the
	// origin, and (1,1), adjacent ONLY to the ford (hexgrid axial offsets:
	// (1,0)'s neighbours are (2,0),(0,0),(1,1),(1,-1),(2,-1),(0,1); (0,0)'s
	// are (1,0),(-1,0),(0,1),(0,-1),(1,-1),(-1,1) — (1,1) and (-1,0) each
	// appear in exactly one of the two lists).
	for _, tl := range []struct {
		q, r    int
		terrain string
	}{
		{0, 0, "plains"}, {1, 0, "river_ford"}, {2, 0, "plains"},
		{-1, 0, "plains"}, {1, 1, "plains"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, tl.q, tl.r, tl.terrain,
		); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}

	settlementAt := func(q, r int, name string) uuid.UUID {
		var provinceID uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, 'plains') RETURNING id`,
			worldID, q, r,
		).Scan(&provinceID); err != nil {
			t.Fatalf("create province (%d,%d): %v", q, r, err)
		}
		var settlementID uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', false) RETURNING id`,
			worldID, provinceID, name, playerID,
		).Scan(&settlementID); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return settlementID
	}
	nearOriginID := settlementAt(-1, 0, "NearOrigin")
	nearFordID := settlementAt(1, 1, "NearFord")

	clk := clock.NewTestClock(time.Now())
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET current_tick = 0, last_tick_at = $1 WHERE id = $2`, clk.Now(), worldID,
	); err != nil {
		t.Fatalf("align world tick anchor: %v", err)
	}
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'positioned', 0, 0) RETURNING id`,
		worldID, playerID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create positioned land unit: %v", err)
	}

	res, err := combat.StartMarch(ctx, pool, scheduler, eventStore, clk, combat.MarchOrder{
		WorldID: worldID, PlayerID: playerID, UnitID: unitID,
		TargetQ: 2, TargetR: 0,
	}, nil)
	if err != nil {
		t.Fatalf("StartMarch: %v", err)
	}
	if res.DurationTicks != 3 {
		t.Fatalf("test fixture assumption broken: march took %d ticks, want 3", res.DurationTicks)
	}
	clk.Advance(time.Duration(0.3*3*float64(tick.TickSeconds)) * time.Second) // 30% through

	uh := NewUnitHandler(pool, scheduler, eventStore, clk)
	router := chi.NewRouter()
	router.Use(auth.Middleware(authSvc))
	router.Post("/worlds/{worldID}/units/{unitID}/recall", uh.Recall)

	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+worldID.String()+"/units/"+unitID.String()+"/recall", bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("Recall = %d %q, want 202 (a courier dispatch)", rec.Code, rec.Body.String())
	}
	var resp struct {
		MessengerID uuid.UUID `json:"messenger_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	var originID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT origin_id FROM messengers WHERE id = $1`, resp.MessengerID).Scan(&originID); err != nil {
		t.Fatalf("load dispatched messenger: %v", err)
	}
	if originID != nearFordID {
		wantName := "NearFord"
		gotName := "NearOrigin (or another settlement)"
		if originID == nearOriginID {
			gotName = "NearOrigin"
		}
		t.Errorf("Recall's courier dispatched from settlement %v (%s), want %v (%s) — "+
			"the precheck must resolve current position via the saved route (the ford), not the old re-walk (the origin)",
			originID, gotName, nearFordID, wantName)
	}
}

// TestAcceptance2_UnitJSON_CurrentPosMatches is the fourth surface: at the
// same 30%-through instant as the other three TestAcceptance2_* tests,
// GET /worlds/{worldID}/units' current_q/current_r (R7) must report the ford
// (1,0), not the old floor-based origin.
func TestAcceptance2_UnitJSON_CurrentPosMatches(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	accessToken, _, err := authSvc.Register(ctx, "ford-unit-json-"+uuid.New().String(), "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	for _, tl := range []struct {
		q, r    int
		terrain string
	}{
		{0, 0, "plains"}, {1, 0, "river_ford"}, {2, 0, "plains"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, tl.q, tl.r, tl.terrain,
		); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}

	clk := clock.NewTestClock(time.Now())
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET current_tick = 0, last_tick_at = $1 WHERE id = $2`, clk.Now(), worldID,
	); err != nil {
		t.Fatalf("align world tick anchor: %v", err)
	}
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'positioned', 0, 0) RETURNING id`,
		worldID, playerID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create positioned land unit: %v", err)
	}

	res, err := combat.StartMarch(ctx, pool, scheduler, eventStore, clk, combat.MarchOrder{
		WorldID: worldID, PlayerID: playerID, UnitID: unitID,
		TargetQ: 2, TargetR: 0,
	}, nil)
	if err != nil {
		t.Fatalf("StartMarch: %v", err)
	}
	if res.DurationTicks != 3 {
		t.Fatalf("test fixture assumption broken: march took %d ticks, want 3", res.DurationTicks)
	}
	clk.Advance(time.Duration(0.3*3*float64(tick.TickSeconds)) * time.Second) // 30% through

	uh := NewUnitHandler(pool, scheduler, eventStore, clk)
	router := chi.NewRouter()
	router.Use(auth.Middleware(authSvc))
	router.Get("/worlds/{worldID}/units", uh.ListUnits)

	req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/units", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ListUnits = %d %q, want 200", rec.Code, rec.Body.String())
	}
	var resp struct {
		Units []struct {
			ID       uuid.UUID `json:"id"`
			CurrentQ *int      `json:"current_q"`
			CurrentR *int      `json:"current_r"`
		} `json:"units"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var found bool
	for _, u := range resp.Units {
		if u.ID != unitID {
			continue
		}
		found = true
		if u.CurrentQ == nil || u.CurrentR == nil {
			t.Fatalf("unit %v has no current_q/current_r — want (1,0) (the ford)", unitID)
		}
		if *u.CurrentQ != 1 || *u.CurrentR != 0 {
			t.Errorf("unit current_q/r = (%d,%d), want (1,0) (the ford — val A, already entered at departure)", *u.CurrentQ, *u.CurrentR)
		}
	}
	if !found {
		t.Fatalf("unit %v not found in ListUnits response", unitID)
	}
}
