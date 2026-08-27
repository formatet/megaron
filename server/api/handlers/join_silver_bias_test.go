package handlers

// join.go's tier-2 tiebreak used to know only the hemisphere's own ore
// (copper west, tin east). Nothing ever steered a settler towards SILVER, and
// the drift world showed exactly what that costs: 8 copper hexes put copper in
// 4 of 8 cities' catchments, while 4 silver hexes reached 1 — same terrain
// class, same map, the only difference being this ORDER BY
// (megaron_silvergeografin.md, 2026-08-27). Density alone cannot close that
// gap: a 60x60 world holds ~60 possible city sites for ~10 settlers, so an
// unbiased settler meets a scarce metal at roughly sources/sites.
//
// The rank is now hemisphere ore = 2, silver = 1, nothing = 0. This test pins
// both halves of that: silver beats bare ground, and ore still beats silver.
//
// Fixture (one landmass, so tier-1's landmass load can never decide; map_width
// 12 -> half_q = 5, every candidate is WEST so copper is the hemisphere ore):
//   (0,   0) plains — copper deposit at (1,0), rank 2
//   (0, 100) plains — silver deposit at (1,100), rank 1
//   (0, 200) plains — nothing, rank 0
// Candidates sit 100 hexes apart, so the 4-hex clearance filter never lets one
// join remove another, and each deposit is only ever within catchment radius
// of its own candidate. Deposit tiles are mountain_red: real deposit ground
// that join's terrain filter excludes from candidacy, so they cannot be picked
// themselves.
//
// DB integration test (real Postgres, gated by DATABASE_URL).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func seedMetalBiasWorld(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, map_width, map_height) VALUES ($1, 12, 250) RETURNING id`,
		"test-metal-bias-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	type tile struct {
		q, r           int
		terrain        string
		copper, silver bool
	}
	one := 1
	for _, tl := range []tile{
		{0, 0, "plains", false, false},
		{1, 0, "mountain_red", true, false},
		{0, 100, "plains", false, false},
		{1, 100, "mountain_red", false, true},
		{0, 200, "plains", false, false},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain, copper_deposit, silver_deposit, landmass_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			worldID, tl.q, tl.r, tl.terrain, tl.copper, tl.silver, one,
		); err != nil {
			t.Fatalf("seed tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}
	return worldID
}

func TestJoin_SilverOutranksBareGroundButNotOre(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	worldID := seedMetalBiasWorld(t, pool)

	authSvc := auth.NewService(pool, "test-secret")
	router := joinCultureRouter(pool, authSvc, clock.NewTestClock(time.Now()))

	joinOnce := func(prefix string) (int, int) {
		_, token := registerViewer(t, ctx, authSvc, prefix)
		req := httptest.NewRequest(http.MethodPost, "/worlds/"+worldID.String()+"/join", strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST /join (%s) = %d %q, want 201", prefix, rec.Code, rec.Body.String())
		}
		var resp struct {
			Tile struct {
				Q int `json:"Q"`
				R int `json:"R"`
			} `json:"tile"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode /join response (%s): %v (body: %s)", prefix, err, rec.Body.String())
		}
		return resp.Tile.Q, resp.Tile.R
	}

	q1, r1 := joinOnce("metal-bias-1")
	if q1 != 0 || r1 != 0 {
		t.Fatalf("join 1 landed on (%d,%d), want the copper site (0,0) — the hemisphere's own ore must still outrank silver", q1, r1)
	}
	q2, r2 := joinOnce("metal-bias-2")
	if q2 != 0 || r2 != 100 {
		t.Fatalf("join 2 landed on (%d,%d), want the silver site (0,100) — silver in catchment must outrank ground with no metal at all", q2, r2)
	}
	q3, r3 := joinOnce("metal-bias-3")
	if q3 != 0 || r3 != 200 {
		t.Fatalf("join 3 landed on (%d,%d), want the last remaining site (0,200)", q3, r3)
	}
}
