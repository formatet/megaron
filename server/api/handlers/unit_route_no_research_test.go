package handlers

// Acceptance 3 (megaron_plan_rorelse_sparad_vag.md §6): once a march has a
// saved route, changing the terrain afterwards must never change what the
// owner's own map or keryx shows — the path came from the ORDER, not from a
// fresh search. This is the direct test of R1's "vägen ... söks aldrig fram
// igen vid läsning" and R7's "gäller vägen returneras route.Hexes som path,
// och ingen FindPath körs."

import (
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
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestAcceptance3_PathNeverResearched_AfterTerrainChange(t *testing.T) {
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
	accessToken, _, err := authSvc.Register(ctx, "no-research-"+uuid.New().String(), "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	// A mountain wall forces a detour via (0,1)/(1,1): (0,0) -> (0,1) -> (1,1)
	// -> (2,0) (or similar), never straight through (1,0).
	for _, tl := range []struct {
		q, r    int
		terrain string
	}{
		{0, 0, "plains"}, {1, 0, "mountain_limestone"}, {2, 0, "plains"},
		{0, 1, "plains"}, {1, 1, "plains"}, {2, 1, "plains"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, tl.q, tl.r, tl.terrain,
		); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}

	clk := clock.NewTestClock(time.Now())
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

	if _, err := combat.StartMarch(ctx, pool, scheduler, eventStore, clk, combat.MarchOrder{
		WorldID: worldID, PlayerID: playerID, UnitID: unitID,
		TargetQ: 2, TargetR: 0,
	}, nil); err != nil {
		t.Fatalf("StartMarch: %v", err)
	}

	uh := NewUnitHandler(pool, scheduler, eventStore, clk)
	router := chi.NewRouter()
	router.Use(auth.Middleware(authSvc))
	router.Get("/worlds/{worldID}/units", uh.ListUnits)

	fetchPath := func() [][2]int {
		req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/units", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("ListUnits = %d %q, want 200", rec.Code, rec.Body.String())
		}
		var resp struct {
			Units []struct {
				ID   uuid.UUID `json:"id"`
				Path [][2]int  `json:"path"`
			} `json:"units"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		for _, u := range resp.Units {
			if u.ID == unitID {
				return u.Path
			}
		}
		t.Fatalf("unit %v not found in ListUnits response", unitID)
		return nil
	}

	before := fetchPath()
	if len(before) < 4 {
		t.Fatalf("test fixture is too weak: saved path %v has no real detour to prove against a straight line", before)
	}
	for _, hex := range before {
		if hex == [2]int{1, 0} {
			t.Fatalf("test fixture is broken: saved path %v crosses the mountain at (1,0)", before)
		}
	}

	// Remove the mountain — a fresh A* search would now find a shorter,
	// straight path right through (1,0).
	if _, err := pool.Exec(ctx,
		`UPDATE map_tiles SET terrain = 'plains' WHERE world_id = $1 AND q = 1 AND r = 0`, worldID,
	); err != nil {
		t.Fatalf("open the mountain: %v", err)
	}

	after := fetchPath()
	if len(after) != len(before) {
		t.Fatalf("path length changed after the terrain edit: before=%v after=%v — the path was re-searched", before, after)
	}
	for i := range before {
		if after[i] != before[i] {
			t.Fatalf("path changed after the terrain edit at index %d: before=%v after=%v — the path was re-searched", i, before, after)
		}
	}
}
