package handlers

import (
	"context"
	"math"
	"net/http"
	"testing"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/hexgrid"
)

// A city with no forest must expose and accept an actual timber placement.
// Every remaining production hex is maquis or hills; the valley subcase adds
// only its target valley. This exercises the authenticated HTTP verb and the
// persisted rate, not a duplicated terrain/rate formula in the test.
func TestSparseTimber_HTTPPlacementAndLumbermill(t *testing.T) {
	for _, terrain := range []string{"scrub_maquis", "hills", "river_valley"} {
		t.Run(terrain, func(t *testing.T) {
			tiles := map[[2]int]string{}
			for i, h := range hexgrid.Ring(hexgrid.Coord{}, hexgrid.CatchmentRadius) {
				tiles[[2]int{h.Q, h.R}] = []string{"scrub_maquis", "hills"}[i%2]
			}
			tiles[[2]int{1, 0}] = terrain
			f := setupPlacementFixture(t, tiles)
			pool := p10TestPool(t)
			ctx := context.Background()
			if _, err := pool.Exec(ctx, `UPDATE map_tiles SET terrain='scrub_maquis' WHERE world_id=$1 AND q=0 AND r=0`, f.worldID); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE settlements SET population=1000 WHERE id=$1`, f.settlementID); err != nil {
				t.Fatal(err)
			}
			options := func(wantCap, wantPlaced int, wantRate float64) {
				t.Helper()
				code, resp := f.do(t, http.MethodGet, f.placementOptionsPath(), nil)
				if code != http.StatusOK {
					t.Fatalf("placement-options %d: %v", code, resp)
				}
				for _, item := range resp["hexes"].([]any) {
					h := item.(map[string]any)
					if h["hex_q"] != float64(1) || h["hex_r"] != float64(0) {
						continue
					}
					for _, item := range h["goods"].([]any) {
						g := item.(map[string]any)
						if g["good_key"] != "timber" {
							continue
						}
						if g["cap"] != float64(wantCap) || g["placed"] != float64(wantPlaced) || math.Abs(g["marginal_yield"].(float64)-wantRate) > 1e-9 || math.Abs(g["rate_per_tick"].(float64)-.33) > 1e-9 {
							t.Fatalf("timber option = %v, want cap%d placed%d rate%.3f", g, wantCap, wantPlaced, wantRate)
						}
						return
					}
					t.Fatalf("%s target offers no timber: %v", terrain, h)
				}
				t.Fatal("target hex absent")
			}
			place := func(want int) {
				t.Helper()
				code, resp := f.do(t, http.MethodPost, f.placementsPath(), map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "timber"})
				if code != want {
					t.Fatalf("place timber on %s = %d: %v, want %d", terrain, code, resp, want)
				}
			}
			rate := func(want float64) {
				t.Helper()
				var got float64
				if err := pool.QueryRow(ctx, `SELECT rate FROM settlement_goods WHERE settlement_id=$1 AND good_key='timber'`, f.settlementID).Scan(&got); err != nil {
					t.Fatal(err)
				}
				if math.Abs(got-want) > 1e-9 {
					t.Fatalf("timber persisted rate %.9f, want %.9f", got, want)
				}
			}
			options(1, 0, .33)
			place(http.StatusCreated)
			options(1, 1, .33)
			rate(.33)
			place(http.StatusConflict) // capacity, not workforce: nine citizens remain free
			if _, err := pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level,hex_q,hex_r) VALUES($1,'lumbermill',1,1,0)`, f.settlementID); err != nil {
				t.Fatal(err)
			}
			options(5, 1, .561)
			for i := 0; i < 4; i++ {
				place(http.StatusCreated)
			}
			options(5, 5, .561)
			rate(2.805)
			place(http.StatusConflict)
			if _, err := pool.Exec(ctx, `UPDATE buildings SET level=3 WHERE settlement_id=$1 AND building_type='lumbermill'`, f.settlementID); err != nil {
				t.Fatal(err)
			}
			// The migration adds possibilities, never automatic timber production.
			for i := 0; i < 2; i++ {
				if err := economy.RecomputeProduction(ctx, pool, f.settlementID); err != nil {
					t.Fatal(err)
				}
			}
			options(5, 5, 1.023)
			rate(5.115)
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM settlement_placement WHERE settlement_id=$1 AND good_key='timber'`, f.settlementID).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 5 {
				t.Fatalf("recompute changed placements: %d", count)
			}
		})
	}
}

func TestSparseTimber_HTTPBuildOnNewTerrains(t *testing.T) {
	for _, terrain := range []string{"scrub_maquis", "hills", "river_valley"} {
		t.Run(terrain, func(t *testing.T) {
			f := buildHexFixture(t)
			if _, err := f.pool.Exec(context.Background(), `UPDATE map_tiles SET terrain=$2 WHERE world_id=$1 AND q=1 AND r=0`, f.worldID, terrain); err != nil {
				t.Fatal(err)
			}
			code, resp := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "lumbermill", "hex_q": 1, "hex_r": 0})
			if code != http.StatusCreated {
				t.Fatalf("build lumbermill on %s = %d: %v, want 201", terrain, code, resp)
			}
		})
	}
}
