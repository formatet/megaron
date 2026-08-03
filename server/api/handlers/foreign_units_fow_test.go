package handlers

// FOW tests for GET /worlds/{worldID}/foreign-units (fow/frammande-enheter,
// 2026-08-03) — enemy/neutral units had no visibility path at all before this
// slice; ListUnits (unit.go) only ever returns the caller's OWN units. These
// tests pin the new surface: live tier reveals a foreign unit in full (type,
// size, stance), remembered tier reveals none, a march is fully disclosed but
// never turns its target/route hexes into map knowledge, and the same live-only
// rule now gates a foreign city's garrison in /provinces.
//
// Same rig as provinces_fow_test.go: real Postgres (DATABASE_URL-gated),
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

// foreignUnitView mirrors foreign_units.go's foreignUnit JSON shape for test
// decoding.
type foreignUnitView struct {
	ID       string `json:"id"`
	Owner    string `json:"owner"`
	OwnerID  string `json:"owner_id"`
	Type     string `json:"type"`
	Category string `json:"category"`
	Size     int    `json:"size"`
	Status   string `json:"status"`
	Stance   string `json:"stance,omitempty"`
	Q        int    `json:"q"`
	R        int    `json:"r"`

	TargetQ   *int       `json:"target_q,omitempty"`
	TargetR   *int       `json:"target_r,omitempty"`
	DepartsAt *time.Time `json:"departs_at,omitempty"`
	ArrivesAt *time.Time `json:"arrives_at,omitempty"`
	Path      [][2]int   `json:"path,omitempty"`
}

// registerViewer creates a fresh player via a real auth.Service and returns
// (playerID, bearer token) — same pattern as provinces_fow_test.go.
func registerViewer(t *testing.T, ctx context.Context, authSvc *auth.Service, prefix string) (uuid.UUID, string) {
	t.Helper()
	username := prefix + "-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register %s: %v", prefix, err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	return claims.PlayerID, accessToken
}

// TestForeignUnits_LiveTierRevealsNearbyPositionedUnit is T1+T2: a positioned
// foreign unit 2 hexes from the viewer's capital (inside the settlement eye's
// live radius 3 over plains) must appear in full; the same kind of unit 5 hexes
// out (outside that radius, no other eye) must not appear at all.
func TestForeignUnits_LiveTierRevealsNearbyPositionedUnit(t *testing.T) {
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
	viewerID, accessToken := registerViewer(t, ctx, authSvc, "fu-viewer")

	var enemyOwnerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"fu-enemy-"+uuid.New().String(), "fu-enemy-"+uuid.New().String()+"@test.invalid",
	).Scan(&enemyOwnerID); err != nil {
		t.Fatalf("create enemy owner: %v", err)
	}

	// Viewer's capital at (0,0) — settlement eye sees 3 hexes over plains.
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

	// Visible enemy: positioned at hex distance 2 (inside radius 3).
	for _, tile := range [][2]int{{2, 0}, {5, 0}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'plains')`,
			worldID, tile[0], tile[1],
		); err != nil {
			t.Fatalf("create map_tiles(%d,%d): %v", tile[0], tile[1], err)
		}
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, stance, q, r)
		 VALUES ($1, $2, 'war_chariot', 'land', 40, 0, 'positioned', 'fortify', 2, 0)`,
		worldID, enemyOwnerID,
	); err != nil {
		t.Fatalf("create visible enemy unit: %v", err)
	}
	// Hidden enemy: positioned at hex distance 5 (outside radius 3, no other eye).
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'positioned', 5, 0)`,
		worldID, enemyOwnerID,
	); err != nil {
		t.Fatalf("create hidden enemy unit: %v", err)
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
	var units []foreignUnitView
	if err := json.Unmarshal(rec.Body.Bytes(), &units); err != nil {
		t.Fatalf("decode /foreign-units response: %v (body: %s)", err, rec.Body.String())
	}

	if len(units) != 1 {
		t.Fatalf("expected exactly 1 visible foreign unit, got %d: %+v", len(units), units)
	}
	got := units[0]
	if got.Type != "war_chariot" || got.Size != 40 || got.Stance != "fortify" {
		t.Errorf("visible unit = %+v, want type=war_chariot size=40 stance=fortify", got)
	}
	if got.Q != 2 || got.R != 0 {
		t.Errorf("visible unit position = (%d,%d), want (2,0)", got.Q, got.R)
	}
}

// TestForeignUnits_RememberedTileDoesNotRevealUnit is T3: an enemy unit standing
// on a hex the viewer has scouted before (player_scouted_tiles, tier 2) but does
// not currently see must NOT appear — remembered tiles carry no activity.
func TestForeignUnits_RememberedTileDoesNotRevealUnit(t *testing.T) {
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
	viewerID, accessToken := registerViewer(t, ctx, authSvc, "fu-rememberer")

	var enemyOwnerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"fu-enemy-"+uuid.New().String(), "fu-enemy-"+uuid.New().String()+"@test.invalid",
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

	// Hex (6,0): distance 6 from capital, outside the settlement eye's radius 3,
	// but previously scouted — tier 2 memory.
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 6, 0, 'plains')`,
		worldID,
	); err != nil {
		t.Fatalf("create map_tiles(6,0): %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO player_scouted_tiles (world_id, player_id, q, r) VALUES ($1, $2, 6, 0)`,
		worldID, viewerID,
	); err != nil {
		t.Fatalf("mark (6,0) remembered: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'positioned', 6, 0)`,
		worldID, enemyOwnerID,
	); err != nil {
		t.Fatalf("create enemy unit on remembered tile: %v", err)
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
	var units []foreignUnitView
	if err := json.Unmarshal(rec.Body.Bytes(), &units); err != nil {
		t.Fatalf("decode /foreign-units response: %v (body: %s)", err, rec.Body.String())
	}
	if len(units) != 0 {
		t.Errorf("expected no units revealed from a remembered-only tile, got %+v", units)
	}
}

// TestForeignUnits_MarchingUnitRevealedByInterpolatedPosition is T4: a foreign
// march whose origin AND target both sit far outside the viewer's sight must
// still appear when its INTERPOLATED CURRENT position falls inside it — this is
// the whole difference from the /marches endpoint gate (which gates on origin OR
// target, never a moving point).
func TestForeignUnits_MarchingUnitRevealedByInterpolatedPosition(t *testing.T) {
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
	viewerID, accessToken := registerViewer(t, ctx, authSvc, "fu-march-viewer")

	var enemyOwnerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"fu-march-enemy-"+uuid.New().String(), "fu-march-enemy-"+uuid.New().String()+"@test.invalid",
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

	// Straight line q=-8..8, r=0 (17 tiles) — both endpoints are hex-distance 8
	// from the capital, outside its live radius 3. departsAt/arrivesAt give
	// elapsed/total = 4h/10h = 0.4; interpolatedEyePos picks
	// idx = round(0.4*(17-1)) = round(6.4) = 6, i.e. q = -8+6 = -2, r=0 — hex
	// distance 2 from the capital, INSIDE its radius-3 vision. Origin and target
	// both invisible, only the interpolated current position visible.
	for q := -8; q <= 8; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			worldID, q,
		); err != nil {
			t.Fatalf("create map_tiles(%d,0): %v", q, err)
		}
	}

	now := time.Now().UTC().Truncate(time.Second)
	departsAt := now.Add(-4 * time.Hour)
	arrivesAt := now.Add(6 * time.Hour) // total 10h, 4h elapsed -> progress 0.4

	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, target_q, target_r, departs_at, arrives_at)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'marching', -8, 0, 8, 0, $3, $4) RETURNING id`,
		worldID, enemyOwnerID, departsAt, arrivesAt,
	).Scan(&unitID); err != nil {
		t.Fatalf("create marching enemy unit: %v", err)
	}

	clk := clock.NewTestClock(now)
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
	var units []foreignUnitView
	if err := json.Unmarshal(rec.Body.Bytes(), &units); err != nil {
		t.Fatalf("decode /foreign-units response: %v (body: %s)", err, rec.Body.String())
	}
	if len(units) != 1 {
		t.Fatalf("expected the marching unit to be revealed via its interpolated position, got %d: %+v", len(units), units)
	}
	got := units[0]
	if got.Q != -2 || got.R != 0 {
		t.Errorf("interpolated position = (%d,%d), want (-2,0) — 0.4 progress along the 17-hex path", got.Q, got.R)
	}
	if got.TargetQ == nil || *got.TargetQ != 8 || got.TargetR == nil || *got.TargetR != 0 {
		t.Errorf("target = (%v,%v), want (8,0) — march data must be disclosed in full", got.TargetQ, got.TargetR)
	}
	if len(got.Path) < 2 || got.Path[0] != [2]int{-8, 0} || got.Path[len(got.Path)-1] != [2]int{8, 0} {
		t.Errorf("path = %v, want to start at (-8,0) and end at (8,0)", got.Path)
	}
}

// TestForeignUnits_UnauthenticatedReturnsEmptyList is T5: an unauthenticated
// caller must get an empty list, never unit data — unlike /map, which shows the
// whole world to a logged-out caller.
func TestForeignUnits_UnauthenticatedReturnsEmptyList(t *testing.T) {
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

	var enemyOwnerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"fu-anon-enemy-"+uuid.New().String(), "fu-anon-enemy-"+uuid.New().String()+"@test.invalid",
	).Scan(&enemyOwnerID); err != nil {
		t.Fatalf("create enemy owner: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 0, 0, 'plains')`,
		worldID,
	); err != nil {
		t.Fatalf("create map_tiles(0,0): %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'positioned', 0, 0)`,
		worldID, enemyOwnerID,
	); err != nil {
		t.Fatalf("create enemy unit: %v", err)
	}

	authSvc := auth.NewService(pool, "test-secret")
	wh := NewWorldHandler(pool, authSvc, clock.NewTestClock(time.Now()))
	r := chi.NewRouter()
	r.Use(auth.OptionalMiddleware(authSvc))
	r.Get("/worlds/{worldID}/foreign-units", wh.ForeignUnits)

	// No Authorization header at all.
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/foreign-units", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /foreign-units (unauthenticated) = %d %q, want 200", rec.Code, rec.Body.String())
	}
	var units []foreignUnitView
	if err := json.Unmarshal(rec.Body.Bytes(), &units); err != nil {
		t.Fatalf("decode /foreign-units response: %v (body: %s)", err, rec.Body.String())
	}
	if len(units) != 0 {
		t.Errorf("unauthenticated /foreign-units must return [], got %+v", units)
	}
}

// TestForeignUnits_MarchDoesNotLeakMapKnowledge is T6: after a foreign march is
// revealed via its interpolated position (same fixture as the interpolation
// test above), the viewer's OWN map knowledge must be completely unaffected —
// the target hex (8,0), far outside the viewer's real vision, must still read
// as fog under GET /map, and no player_scouted_tiles row may exist for the
// target or the far (invisible) legs of the enemy's route. Rows DO appear for
// hexes the viewer's OWN capital eye reaches regardless of the enemy march
// (q in [-3,3] at r=0) — that is /map's pre-existing own-vision memory write and
// not something this feature should suppress or is responsible for.
func TestForeignUnits_MarchDoesNotLeakMapKnowledge(t *testing.T) {
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
	viewerID, accessToken := registerViewer(t, ctx, authSvc, "fu-leak-viewer")

	var enemyOwnerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"fu-leak-enemy-"+uuid.New().String(), "fu-leak-enemy-"+uuid.New().String()+"@test.invalid",
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

	for q := -8; q <= 8; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			worldID, q,
		); err != nil {
			t.Fatalf("create map_tiles(%d,0): %v", q, err)
		}
	}

	now := time.Now().UTC().Truncate(time.Second)
	departsAt := now.Add(-4 * time.Hour)
	arrivesAt := now.Add(6 * time.Hour)
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, target_q, target_r, departs_at, arrives_at)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'marching', -8, 0, 8, 0, $3, $4)`,
		worldID, enemyOwnerID, departsAt, arrivesAt,
	); err != nil {
		t.Fatalf("create marching enemy unit: %v", err)
	}

	clk := clock.NewTestClock(now)
	wh := NewWorldHandler(pool, authSvc, clk)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Get("/worlds/{worldID}/foreign-units", wh.ForeignUnits)
	r.Get("/worlds/{worldID}/map", wh.Map)

	// First: confirm the unit IS revealed (same shape as the interpolation test).
	fuReq := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/foreign-units", nil)
	fuReq.Header.Set("Authorization", "Bearer "+accessToken)
	fuRec := httptest.NewRecorder()
	r.ServeHTTP(fuRec, fuReq)
	var units []foreignUnitView
	if err := json.Unmarshal(fuRec.Body.Bytes(), &units); err != nil {
		t.Fatalf("decode /foreign-units response: %v (body: %s)", err, fuRec.Body.String())
	}
	if len(units) != 1 {
		t.Fatalf("expected the march to be revealed before checking map leakage, got %d units", len(units))
	}

	// Now: GET /map for the same viewer and check the target hex (8,0).
	mapReq := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/map", nil)
	mapReq.Header.Set("Authorization", "Bearer "+accessToken)
	mapRec := httptest.NewRecorder()
	r.ServeHTTP(mapRec, mapReq)
	if mapRec.Code != http.StatusOK {
		t.Fatalf("GET /map = %d %q, want 200", mapRec.Code, mapRec.Body.String())
	}
	type tileView struct {
		Q    int    `json:"q"`
		R    int    `json:"r"`
		Tier string `json:"tier"`
	}
	var tiles []tileView
	if err := json.Unmarshal(mapRec.Body.Bytes(), &tiles); err != nil {
		t.Fatalf("decode /map response: %v (body: %s)", err, mapRec.Body.String())
	}
	var targetTier string
	found := false
	for _, tl := range tiles {
		if tl.Q == 8 && tl.R == 0 {
			targetTier = tl.Tier
			found = true
		}
	}
	if !found {
		t.Fatalf("target hex (8,0) missing from /map response entirely")
	}
	if targetTier != "fog" {
		t.Errorf("target hex (8,0) tier = %q, want \"fog\" — the march must not turn its target into map knowledge", targetTier)
	}

	// No player_scouted_tiles row for any hex outside the viewer's own capital
	// vision (radius 3, i.e. q outside [-3,3]) — those hexes are known ONLY
	// through the enemy's march, and this feature must never write map memory.
	rows, err := pool.Query(ctx,
		`SELECT q, r FROM player_scouted_tiles WHERE world_id = $1 AND player_id = $2`,
		worldID, viewerID,
	)
	if err != nil {
		t.Fatalf("query player_scouted_tiles: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var q, rr int
		if err := rows.Scan(&q, &rr); err != nil {
			t.Fatalf("scan player_scouted_tiles row: %v", err)
		}
		if rr == 0 && (q < -3 || q > 3) {
			t.Errorf("unexpected player_scouted_tiles row (%d,%d) — outside the viewer's own vision, "+
				"could only have come from the enemy march's route/target", q, rr)
		}
	}
}

// TestForeignUnits_GarrisonRevealedLiveHiddenRemembered is T7: the world.go
// garrison-exposure fix. A foreign city's garrison (army_total on the
// /provinces marker) must be non-zero when the city is in the viewer's live
// tier, and exactly zero when the same city is known only through tier-2
// memory (player_scouted_tiles) — closing the sibling hole where a remembered
// city still reported a live garrison count.
func TestForeignUnits_GarrisonRevealedLiveHiddenRemembered(t *testing.T) {
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
	viewerID, accessToken := registerViewer(t, ctx, authSvc, "fu-garrison-viewer")

	var enemyOwnerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"fu-garrison-enemy-"+uuid.New().String(), "fu-garrison-enemy-"+uuid.New().String()+"@test.invalid",
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

	// Live city: distance 2, inside the settlement eye's radius 3.
	var liveProvinceID, liveSettlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 2, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&liveProvinceID); err != nil {
		t.Fatalf("create live-city province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Livewatch', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, liveProvinceID, enemyOwnerID,
	).Scan(&liveSettlementID); err != nil {
		t.Fatalf("create live-city settlement: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 180, 0, 'garrison', $3)`,
		worldID, enemyOwnerID, liveSettlementID,
	); err != nil {
		t.Fatalf("create live-city garrison: %v", err)
	}

	// Remembered-only city: distance 6, outside radius 3, but scouted before.
	var rememberedProvinceID, rememberedSettlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 6, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&rememberedProvinceID); err != nil {
		t.Fatalf("create remembered-city province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Fadingkeep', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, rememberedProvinceID, enemyOwnerID,
	).Scan(&rememberedSettlementID); err != nil {
		t.Fatalf("create remembered-city settlement: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 50, 0, 'garrison', $3)`,
		worldID, enemyOwnerID, rememberedSettlementID,
	); err != nil {
		t.Fatalf("create remembered-city garrison: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO player_scouted_tiles (world_id, player_id, q, r) VALUES ($1, $2, 6, 0)`,
		worldID, viewerID,
	); err != nil {
		t.Fatalf("mark (6,0) remembered: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	wh := NewWorldHandler(pool, authSvc, clk)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Get("/worlds/{worldID}/provinces", wh.Provinces)

	req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/provinces", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /provinces = %d %q, want 200", rec.Code, rec.Body.String())
	}

	type provinceGarrisonView struct {
		Name      string `json:"name"`
		Visible   bool   `json:"visible"`
		ArmyTotal int    `json:"army_total,omitempty"`
	}
	var markers []provinceGarrisonView
	if err := json.Unmarshal(rec.Body.Bytes(), &markers); err != nil {
		t.Fatalf("decode /provinces response: %v (body: %s)", err, rec.Body.String())
	}

	var live, remembered *provinceGarrisonView
	for i := range markers {
		switch markers[i].Name {
		case "Livewatch":
			live = &markers[i]
		case "Fadingkeep":
			remembered = &markers[i]
		}
	}
	if live == nil {
		t.Fatalf("Livewatch (live-tier city) missing from /provinces")
	}
	if !live.Visible || live.ArmyTotal <= 0 {
		t.Errorf("Livewatch = %+v, want visible=true army_total>0 (live tier reveals the garrison)", *live)
	}
	if remembered == nil {
		t.Fatalf("Fadingkeep (remembered-tier city) missing from /provinces")
	}
	if !remembered.Visible {
		t.Errorf("Fadingkeep should still be Visible via tier-2 memory")
	}
	if remembered.ArmyTotal != 0 {
		t.Errorf("Fadingkeep army_total = %d, want 0 — remembered tier must not leak a live garrison count", remembered.ArmyTotal)
	}
}
