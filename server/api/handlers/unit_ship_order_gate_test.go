package handlers

// R3 (megaron_plan_skeppsuppdrag_landsatt.md): "ships at sea take no orders" —
// no order, immediate or Runner-borne, reaches a naval unit that isn't
// docked at its own port (status='garrison'). Exception: an owner with no
// active settlement at all (R6's stranded-ship case) may still order such a
// ship — otherwise it would be unreachable forever.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func registerShipGateTestPlayer(t *testing.T, ctx context.Context, authSvc *auth.Service, prefix string) (accessToken string, playerID uuid.UUID) {
	t.Helper()
	accessToken, _, err := authSvc.Register(ctx, prefix+"-"+uuid.New().String(), "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	return accessToken, claims.PlayerID
}

func TestUnitMarch_PositionedShipAtSeaRejected(t *testing.T) {
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
	accessToken, playerID := registerShipGateTestPlayer(t, ctx, authSvc, "sea-order-tester")

	// The player HAS a settlement (elsewhere) — so the stranded exception must
	// NOT apply here.
	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 50, 50, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create home province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Home', 'achaean', $3, 'capital', true)`,
		worldID, provinceID, playerID,
	); err != nil {
		t.Fatalf("create home settlement: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 0, 0, 'coastal_sea'), ($1, 1, 0, 'coastal_sea')`,
		worldID,
	); err != nil {
		t.Fatalf("create map tiles: %v", err)
	}

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'positioned', 0, 0) RETURNING id`,
		worldID, playerID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create field-positioned ship: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/units/{unitID}/march", uh.March)

	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+worldID.String()+"/units/"+shipID.String()+"/march",
		strings.NewReader(`{"target_q":1,"target_r":0}`))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("March(positioned ship at sea, owner has a settlement) = %d %q, want 422 — R3",
			rec.Code, rec.Body.String())
	}
}

func TestUnitMarch_StrandedShipWithNoSettlementsStillTakesOrders(t *testing.T) {
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
	// R6's stranded case: the owner holds NO active settlement at all — the
	// only exception RequireShipInPort carves out (a ship with nowhere to be
	// sent home to must remain reachable, or it is lost forever).
	accessToken, playerID := registerShipGateTestPlayer(t, ctx, authSvc, "castaway")

	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 0, 0, 'coastal_sea'), ($1, 1, 0, 'coastal_sea')`,
		worldID,
	); err != nil {
		t.Fatalf("create map tiles: %v", err)
	}

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'positioned', 0, 0) RETURNING id`,
		worldID, playerID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create field-positioned ship: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/units/{unitID}/march", uh.March)

	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+worldID.String()+"/units/"+shipID.String()+"/march",
		strings.NewReader(`{"target_q":1,"target_r":0,"intent":"explore"}`))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// explore still needs a home settlement of its own to return to (P7's own
	// rule, unchanged) — with none at all, StartMarch's explore branch itself
	// refuses. What THIS test proves is narrower and upstream of that: the
	// order reaches StartMarch at all instead of being turned away at
	// RequireShipInPort's gate with the generic "ships at sea take no orders"
	// message — a distinct rejection reason from explore's own "requires at
	// least one settlement".
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("March(stranded ship, explore) = %d %q, want 422", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); contains(got, "ships at sea take no orders") {
		t.Errorf("rejection = %q — the stranded-owner exception should have let this order past RequireShipInPort; got its generic refusal instead", got)
	}
}
