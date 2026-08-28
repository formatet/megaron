package handlers

// Cargo tests for GET /worlds/{worldID}/foreign-units (handel/frammande-last,
// 2026-08-16) — megaron_plan_frammande_last.md. A visible foreign ship's
// embarked cohort (units.cargo_unit_id, mig 047) is a market signal ("tenn
// rör sig från Knossos"): a wanax who already sees the ship also sees what it
// carries. This adds a COLUMN to a row that already passed the FOW gate, not
// a new row — TestForeignUnits_CargoDoesNotLeakHiddenShip pins that a ship
// outside FOW still does not appear at all, cargo or no cargo.
//
// Same rig as foreign_units_fow_test.go: real Postgres (DATABASE_URL-gated),
// citiesTestPool, a real auth.Service-minted token, and a chi.Mux running the
// production middleware.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// foreignUnitCargoView mirrors foreign_units.go's foreignUnit JSON shape
// (including the new Cargo field) for test decoding.
type foreignUnitCargoView struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Category string `json:"category"`
	Size     int    `json:"size"`
	Q        int    `json:"q"`
	R        int    `json:"r"`
	Cargo    *struct {
		Type string `json:"type"`
		Size int    `json:"size"`
	} `json:"cargo,omitempty"`
}

// TestForeignUnits_CargoSurfacesEmbarkedCohort is the slice's red-before: a
// visible foreign ship carrying an embarked land cohort must report that
// cohort's type+size under "cargo" in the /foreign-units payload. Before this
// slice the payload had no cargo field at all.
func TestForeignUnits_CargoSurfacesEmbarkedCohort(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	viewerID, accessToken := registerViewer(t, ctx, authSvc, "fu-cargo-viewer")

	var enemyOwnerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"fu-cargo-enemy-"+uuid.New().String(),
	).Scan(&enemyOwnerID); err != nil {
		t.Fatalf("create enemy owner: %v", err)
	}

	var capitalProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&capitalProvinceID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Viewerton', 'achaean', $3, 'capital', true)`,
		worldID, capitalProvinceID, viewerID,
	); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	// Visible enemy ship at hex distance 2 (inside the settlement eye's live
	// radius 3 over plains) carrying an embarked spearman cohort.
	for _, tile := range [][2]int{{2, 0}, {5, 0}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'plains')`,
			worldID, tile[0], tile[1],
		); err != nil {
			t.Fatalf("create map_tiles(%d,%d): %v", tile[0], tile[1], err)
		}
	}

	var cargoID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status)
		 VALUES ($1, $2, 'spearman', 'land', 57, 'embarked') RETURNING id`,
		worldID, enemyOwnerID,
	).Scan(&cargoID); err != nil {
		t.Fatalf("create embarked cohort: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, cargo_unit_id)
		 VALUES ($1, $2, 'merchantman', 'naval', 1, 12, 'positioned', 2, 0, $3)`,
		worldID, enemyOwnerID, cargoID,
	); err != nil {
		t.Fatalf("create visible enemy ship: %v", err)
	}

	// Hidden enemy ship (distance 5, outside FOW), also carrying cargo — this
	// is the regression guard: cargo must never be the reason a unit that
	// wouldn't otherwise be visible becomes visible.
	var hiddenCargoID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status)
		 VALUES ($1, $2, 'spearman', 'land', 33, 'embarked') RETURNING id`,
		worldID, enemyOwnerID,
	).Scan(&hiddenCargoID); err != nil {
		t.Fatalf("create hidden embarked cohort: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, cargo_unit_id)
		 VALUES ($1, $2, 'merchantman', 'naval', 1, 12, 'positioned', 5, 0, $3)`,
		worldID, enemyOwnerID, hiddenCargoID,
	); err != nil {
		t.Fatalf("create hidden enemy ship: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	wh := NewWorldHandler(pool, authSvc, clk)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Get("/worlds/{worldID}/foreign-units", wh.ForeignUnits)

	req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/foreign-units", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /foreign-units = %d %q, want 200", rec.Code, rec.Body.String())
	}
	var units []foreignUnitCargoView
	if err := json.Unmarshal(rec.Body.Bytes(), &units); err != nil {
		t.Fatalf("decode /foreign-units response: %v (body: %s)", err, rec.Body.String())
	}

	if len(units) != 1 {
		t.Fatalf("expected exactly 1 visible foreign ship (the hidden one must not leak), got %d: %+v", len(units), units)
	}
	got := units[0]
	if got.Type != "merchantman" {
		t.Fatalf("visible unit = %+v, want the merchantman at (2,0)", got)
	}
	if got.Cargo == nil {
		t.Fatalf("visible ship carrying an embarked cohort has no cargo field in the payload — regression against the slice's whole point")
	}
	if got.Cargo.Type != "spearman" || got.Cargo.Size != 57 {
		t.Errorf("cargo = %+v, want type=spearman size=57", *got.Cargo)
	}
}

// TestForeignUnits_CargoOmittedWhenEmpty is the counterpart: a visible
// foreign ship with no cargo aboard must not grow a spurious cargo field —
// "empty/no last → ingen extra rad" (plan §2).
func TestForeignUnits_CargoOmittedWhenEmpty(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	viewerID, accessToken := registerViewer(t, ctx, authSvc, "fu-nocargo-viewer")

	var enemyOwnerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"fu-nocargo-enemy-"+uuid.New().String(),
	).Scan(&enemyOwnerID); err != nil {
		t.Fatalf("create enemy owner: %v", err)
	}

	var capitalProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&capitalProvinceID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Viewerton', 'achaean', $3, 'capital', true)`,
		worldID, capitalProvinceID, viewerID,
	); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 2, 0, 'plains')`,
		worldID,
	); err != nil {
		t.Fatalf("create map_tiles(2,0): %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'merchantman', 'naval', 1, 12, 'positioned', 2, 0)`,
		worldID, enemyOwnerID,
	); err != nil {
		t.Fatalf("create visible empty enemy ship: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	wh := NewWorldHandler(pool, authSvc, clk)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Get("/worlds/{worldID}/foreign-units", wh.ForeignUnits)

	req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/foreign-units", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /foreign-units = %d %q, want 200", rec.Code, rec.Body.String())
	}
	var units []foreignUnitCargoView
	if err := json.Unmarshal(rec.Body.Bytes(), &units); err != nil {
		t.Fatalf("decode /foreign-units response: %v (body: %s)", err, rec.Body.String())
	}
	if len(units) != 1 {
		t.Fatalf("expected exactly 1 visible foreign ship, got %d: %+v", len(units), units)
	}
	if units[0].Cargo != nil {
		t.Errorf("empty ship reported cargo = %+v, want nil", *units[0].Cargo)
	}
}
