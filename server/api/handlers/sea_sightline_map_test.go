package handlers

// DB/handler tests for the open-water sightline (Timothy 2026-09-25) and the
// disbanded-units-are-not-eyes fix, through the real GET /map. Same rig as
// foreign_units_fow_test.go: real Postgres (DATABASE_URL-gated), citiesTestPool,
// a real auth.Service-minted token, a chi.Mux with the production middleware.

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
	"github.com/jackc/pgx/v5/pgxpool"
)

// mapTiersFor seeds a world with tiles, lets setup place the viewer's units, and
// returns the viewer's /map tier per hex.
func mapTiersFor(t *testing.T, tiles map[[2]int]string, setup func(ctx context.Context, pool *pgxpool.Pool, worldID, viewerID uuid.UUID)) map[[2]int]string {
	t.Helper()
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
	viewerID, accessToken := registerViewer(t, ctx, authSvc, "sightline-viewer")

	for c, terrain := range tiles {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, c[0], c[1], terrain,
		); err != nil {
			t.Fatalf("create map_tiles(%d,%d): %v", c[0], c[1], err)
		}
	}
	setup(ctx, pool, worldID, viewerID)

	wh := NewWorldHandler(pool, authSvc, clock.NewTestClock(time.Now()))
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Get("/worlds/{worldID}/map", wh.Map)

	req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/map", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /map = %d %q, want 200", rec.Code, rec.Body.String())
	}
	var got []struct {
		Q    int    `json:"q"`
		R    int    `json:"r"`
		Tier string `json:"tier"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode /map: %v (body: %s)", err, rec.Body.String())
	}
	tiers := make(map[[2]int]string, len(got))
	for _, tl := range got {
		tiers[[2]int{tl.Q, tl.R}] = tl.Tier
	}
	return tiers
}

// TestMap_SeaHorizonFollowsOpenWaterOnly is the acceptance-world case end to end:
// a spearman on the shore at (28,18), open sea to the east, forest/hills/mountain
// to the west and an enclosed lake at (24,18)/(24,19) behind them. /map must
// report the lake as fog (it was live under the full-disk rule) while the open sea
// 4 hexes east stays live.
func TestMap_SeaHorizonFollowsOpenWaterOnly(t *testing.T) {
	tiles := map[[2]int]string{
		{24, 18}: "coastal_sea", {24, 19}: "coastal_sea", // the enclosed lake
		{25, 18}: "mountain_limestone", {26, 18}: "hills", {27, 18}: "forest_olive_grove",
		{25, 19}: "hills", {26, 19}: "forest_olive_grove", {27, 19}: "plains",
		{28, 18}: "plains", // the spearman's hex, on the shore
	}
	for q := 29; q <= 32; q++ {
		tiles[[2]int{q, 18}] = "deep_sea"
		tiles[[2]int{q, 17}] = "deep_sea"
	}
	tiers := mapTiersFor(t, tiles, func(ctx context.Context, pool *pgxpool.Pool, worldID, viewerID uuid.UUID) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
			 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'positioned', 28, 18)`,
			worldID, viewerID,
		); err != nil {
			t.Fatalf("create spearman: %v", err)
		}
	})

	for _, lake := range [][2]int{{24, 18}, {24, 19}} {
		if tiers[lake] != "fog" {
			t.Errorf("enclosed lake %v tier = %q, want \"fog\" — the line from (28,18) crosses land", lake, tiers[lake])
		}
	}
	if tiers[[2]int{32, 18}] != "live" {
		t.Errorf("open sea (32,18) tier = %q, want \"live\" — 4 out over open water", tiers[[2]int{32, 18}])
	}
	if tiers[[2]int{26, 18}] != "live" {
		t.Errorf("hills (26,18) at distance 2 tier = %q, want \"live\" — the land radius is unchanged", tiers[[2]int{26, 18}])
	}
}

// TestMap_DisbandedUnitIsNotAnEye: most disband paths leave q/r on the row, and
// LoadLiveEyes used to filter only 'embarked' — so a disbanded unit kept seeing.
// A positioned unit is the control: same shape, but alive.
func TestMap_DisbandedUnitIsNotAnEye(t *testing.T) {
	tiles := map[[2]int]string{{0, 0}: "plains", {1, 0}: "plains", {40, 0}: "plains", {41, 0}: "plains"}
	tiers := mapTiersFor(t, tiles, func(ctx context.Context, pool *pgxpool.Pool, worldID, viewerID uuid.UUID) {
		for _, u := range []struct {
			status string
			q      int
		}{{"disbanded", 0}, {"positioned", 40}} {
			if _, err := pool.Exec(ctx,
				`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
				 VALUES ($1, $2, 'spearman', 'land', 100, 0, $3, $4, 0)`,
				worldID, viewerID, u.status, u.q,
			); err != nil {
				t.Fatalf("create %s unit: %v", u.status, err)
			}
		}
	})

	for _, c := range [][2]int{{0, 0}, {1, 0}} {
		if tiers[c] != "fog" {
			t.Errorf("hex %v beside a DISBANDED unit tier = %q, want \"fog\" — a disbanded unit does not see", c, tiers[c])
		}
	}
	for _, c := range [][2]int{{40, 0}, {41, 0}} {
		if tiers[c] != "live" {
			t.Errorf("hex %v beside a positioned unit tier = %q, want \"live\" (control)", c, tiers[c])
		}
	}
}
