package handlers

// megaron_plan_byggkostnader steg 4 / test 7: join must not place a settler on a
// landmass without a timber hex — every building costs timber and a new city
// starts with none. Fixture: landmass A has only plains, landmass B has plains
// and an olive grove. Both A tiles are better by every tie-breaker (A is first
// in line: equal load, and the RANDOM tail is the only thing separating them),
// so the test joins several settlers and requires ALL of them on B.
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
)

func TestJoin_OnlyLandmassesWithTimber(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active worlds: %v", err)
	}
	var worldID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, map_width, map_height) VALUES ($1, 12, 250) RETURNING id`,
		"test-viable-landmass-"+t.Name(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	// A = landmass 1 (plains only), B = landmass 2 (plains + olive grove).
	// All far apart so the 4-hex clearance never excludes one by another.
	tiles := []struct {
		q, r     int
		terrain  string
		landmass int
	}{
		{0, 0, "plains", 1}, {0, 40, "plains", 1}, {0, 80, "plains", 1},
		{0, 120, "plains", 2}, {0, 160, "forest_olive_grove", 2}, {0, 200, "plains", 2},
	}
	for _, tl := range tiles {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain, landmass_id) VALUES ($1, $2, $3, $4, $5)`,
			worldID, tl.q, tl.r, tl.terrain, tl.landmass,
		); err != nil {
			t.Fatalf("seed tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}

	authSvc := auth.NewService(pool, "test-secret")
	r := joinCultureRouter(pool, authSvc, clock.NewTestClock(time.Now()))
	for i := 0; i < 3; i++ {
		_, token := registerViewer(t, ctx, authSvc, "viable-landmass")
		req := httptest.NewRequest(http.MethodPost, "/worlds/"+worldID+"/join", strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("join %d = %d %q, want 201", i, rec.Code, rec.Body.String())
		}
		var resp struct {
			Tile struct{ Q, R int } `json:"tile"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode join %d: %v", i, err)
		}
		var lm int
		if err := pool.QueryRow(ctx, `SELECT landmass_id FROM map_tiles WHERE world_id=$1 AND q=$2 AND r=$3`,
			worldID, resp.Tile.Q, resp.Tile.R).Scan(&lm); err != nil {
			t.Fatalf("landmass of join %d: %v", i, err)
		}
		if lm != 2 {
			t.Fatalf("join %d landed on landmass %d at (%d,%d); landmass 1 has no timber hex — want 2", i, lm, resp.Tile.Q, resp.Tile.R)
		}
	}
}
