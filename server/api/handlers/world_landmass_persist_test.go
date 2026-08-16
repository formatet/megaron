package handlers

// Slice 1 of megaron_plan_spawn_landmassa.md: map_tiles.landmass_id (migration
// 124) must be persisted for every LAND tile at generation time via the real
// production insert path (WorldHandler.storeTiles), NULL for sea tiles, and
// the ids must actually be hex-connected components — not just "some number".
//
// DB integration test (real Postgres, gated by DATABASE_URL). Red before
// migration 124 + the storeTiles column-list change (landmass_id column does
// not exist), green after.

import (
	"context"
	"os"
	"testing"

	"formatet/megaron/server/internal/world"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func landmassTestPool(t *testing.T) *pgxpool.Pool {
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

func TestStoreTiles_PersistsLandmassID(t *testing.T) {
	pool := landmassTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, map_width, map_height) VALUES ($1, 'active', 40, 30) RETURNING id`,
		"test-landmass-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	// internal/world.TestGenerateMap_CopperTinSeaSeparated proves copper and
	// tin never share a land component, so any generated map holding both
	// has >= 2 landmasses (both deposits appear reliably at this size — see
	// TestGenerateMap_DepositsOnProductiveTerrain). Loop a few seeds and use
	// the first that actually yields >= 2 components, so this test does not
	// depend on one seed's incidental geometry forever.
	var tiles []world.MapTile
	found := false
	for seed := int64(0); seed < 20; seed++ {
		candidate, _ := world.GenerateMap(worldID, seed, 40, 30)
		comp := world.LandComponents(candidate)
		ids := map[int]bool{}
		for _, id := range comp {
			ids[id] = true
		}
		if len(ids) >= 2 {
			tiles = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no seed in [0,20) produced a multi-landmass map — fixture assumption broken")
	}

	h := &WorldHandler{pool: pool}
	if err := h.storeTiles(ctx, worldID, tiles); err != nil {
		t.Fatalf("storeTiles: %v", err)
	}

	// Independent expectation computed straight from world.LandComponents —
	// storeTiles must have written exactly this, tile for tile.
	wantComp := world.LandComponents(tiles)

	rows, err := pool.Query(ctx,
		`SELECT q, r, terrain, landmass_id FROM map_tiles WHERE world_id = $1`,
		worldID,
	)
	if err != nil {
		t.Fatalf("query map_tiles: %v", err)
	}
	defer rows.Close()

	type persisted struct {
		q, r     int
		landmass *int
		terrain  string
	}
	var got []persisted
	for rows.Next() {
		var p persisted
		if err := rows.Scan(&p.q, &p.r, &p.terrain, &p.landmass); err != nil {
			t.Fatalf("scan row: %v", err)
		}
		got = append(got, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("row iteration: %v", err)
	}
	if len(got) != len(tiles) {
		t.Fatalf("persisted %d rows, want %d", len(got), len(tiles))
	}

	distinctIDs := map[int]bool{}
	byID := map[int][][2]int{}
	for _, p := range got {
		key := [2]int{p.q, p.r}
		wantID, isLand := wantComp[key]
		if p.terrain == "coastal_sea" || p.terrain == "deep_sea" {
			if isLand {
				t.Fatalf("(%d,%d): terrain %s is sea per fixture but LandComponents says land", p.q, p.r, p.terrain)
			}
			if p.landmass != nil {
				t.Fatalf("(%d,%d): sea tile has non-NULL landmass_id=%d, want NULL", p.q, p.r, *p.landmass)
			}
			continue
		}
		if !isLand {
			// Land per terrain exclusion above but absent from the component
			// map is not expected to occur (landComponents covers every
			// non-sea tile) — surface it loudly if it ever does.
			t.Fatalf("(%d,%d): terrain %s not sea but missing from LandComponents", p.q, p.r, p.terrain)
		}
		if p.landmass == nil {
			t.Fatalf("(%d,%d): land tile (terrain %s) has NULL landmass_id", p.q, p.r, p.terrain)
		}
		if *p.landmass != wantID {
			t.Fatalf("(%d,%d): landmass_id=%d, want %d (from world.LandComponents)", p.q, p.r, *p.landmass, wantID)
		}
		distinctIDs[*p.landmass] = true
		byID[*p.landmass] = append(byID[*p.landmass], key)
	}
	if len(distinctIDs) < 2 {
		t.Fatalf("only %d distinct landmass_id in a fixture chosen for >= 2 components", len(distinctIDs))
	}

	// Hex-connectivity, checked independently against the DB-read rows (not
	// by re-trusting the helper that produced them): every id's tile set must
	// form a single connected component under the 6-neighbour axial rule.
	dirs6 := [6][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
	for id, coords := range byID {
		present := map[[2]int]bool{}
		for _, c := range coords {
			present[c] = true
		}
		seen := map[[2]int]bool{coords[0]: true}
		queue := [][2]int{coords[0]}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, d := range dirs6 {
				n := [2]int{cur[0] + d[0], cur[1] + d[1]}
				if present[n] && !seen[n] {
					seen[n] = true
					queue = append(queue, n)
				}
			}
		}
		if len(seen) != len(coords) {
			t.Fatalf("landmass_id %d: %d tiles persisted but only %d are hex-reachable from each other — not one connected component",
				id, len(coords), len(seen))
		}
	}
}
