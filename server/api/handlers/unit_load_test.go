package handlers

// Regression test for the Fas 0 load-bug: main.go registers the wildcard as
// {unitID} on /worlds/{worldID}/units/{unitID}/load, but Load (and Unload)
// used to read chi.URLParam(r, "shipID") — a name that doesn't exist on that
// route, so it always resolved to "", uuid.Parse("") failed, and every load
// request (valid ship UUID or garbage) died with the same 400 "invalid ship
// ID" before the store lookup ever ran.
//
// This is a DB integration test (real Postgres, gated by DATABASE_URL) and
// goes through a real chi.Mux — a direct h.Load(w, r) call without a
// populated chi RouteContext can't reproduce a route-param NAME mismatch at
// all (chi.URLParam would just return "" either way, matching a hand-built
// context). The router below registers Load/Unload alongside their real
// siblings (march/recall/stance) on one mux with the exact prod pattern, so
// this also guards against reintroducing {shipID} on only one sibling route:
// chi panics at registration time if sibling routes disagree on a wildcard's
// name at the same position.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
)

func unitLoadTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestUnitLoad_RouteParamNameMatchesRouter(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()

	// See internal/combat/unit_arrival_colonize_test.go for why leftover
	// active test worlds must be archived first (one_active_world partial
	// unique index).
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	username := "loader-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, 0, 0, 'plains', true) RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Loadport', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, provinceID, playerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, settlement_id)
		 VALUES ($1, $2, 'merchantman', 'naval', 1, 'garrison', $3) RETURNING id`,
		worldID, playerID, settlementID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create ship unit: %v", err)
	}
	var cargoID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'garrison', $3) RETURNING id`,
		worldID, playerID, settlementID,
	).Scan(&cargoID); err != nil {
		t.Fatalf("create cargo unit: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Get("/worlds/{worldID}/units", uh.ListUnits)
	r.Post("/worlds/{worldID}/units/{unitID}/march", uh.March)
	r.Post("/worlds/{worldID}/units/{unitID}/recall", uh.Recall)
	r.Post("/worlds/{worldID}/units/{unitID}/stance", uh.SetStance)
	r.Post("/worlds/{worldID}/units/{unitID}/load", uh.Load)
	r.Post("/worlds/{worldID}/units/{unitID}/unload", uh.Unload)

	body, _ := json.Marshal(map[string]string{"unit_id": cargoID.String()})
	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+worldID.String()+"/units/"+shipID.String()+"/load", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Load(valid garrisoned ship) = %d %q, want 200 (route param name must resolve the real ship ID, not \"\")",
			rec.Code, rec.Body.String())
	}

	var shipCargo uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT cargo_unit_id FROM units WHERE id = $1`, shipID).Scan(&shipCargo); err != nil {
		t.Fatalf("load ship after Load: %v", err)
	}
	if shipCargo != cargoID {
		t.Errorf("ship.cargo_unit_id = %s, want %s", shipCargo, cargoID)
	}
	var cargoStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, cargoID).Scan(&cargoStatus); err != nil {
		t.Fatalf("load cargo unit after Load: %v", err)
	}
	if cargoStatus != "embarked" {
		t.Errorf("cargo unit status = %q, want embarked", cargoStatus)
	}
}
