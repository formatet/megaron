package world

import (
	"testing"

	"formatet/megaron/server/internal/hexgrid"
)

// The mapgen ore guarantee and the spawn-score mirror must read the same ring
// production and join.go read. A radius change that misses world/ left both on
// radius 1 for months after P1 (2026-08-07 → 2026-11-22).
func TestSpawnCatchmentRadiusMatchesHexgrid(t *testing.T) {
	if spawnCatchmentRadius != hexgrid.CatchmentRadius {
		t.Fatalf("spawnCatchmentRadius = %d, hexgrid.CatchmentRadius = %d", spawnCatchmentRadius, hexgrid.CatchmentRadius)
	}
	want := 3 * hexgrid.CatchmentRadius * (hexgrid.CatchmentRadius + 1) // hexes in the ring, centre excluded
	if len(catchmentOffsets) != want {
		t.Fatalf("catchmentOffsets has %d hexes, want %d", len(catchmentOffsets), want)
	}
}

func TestSpawnOreCatchmentScore_RingEdge(t *testing.T) {
	halfQ := 10
	cand := MapTile{Q: 5, R: 0, Terrain: TerrainPlains}
	for _, tc := range []struct {
		name string
		ore  MapTile
		want int
	}{
		{"copper two hexes away scores 1", MapTile{Q: 7, R: 0, Terrain: TerrainHills, CopperDeposit: true}, 1},
		{"copper two hexes away on the diagonal scores 1", MapTile{Q: 6, R: -2, Terrain: TerrainHills, CopperDeposit: true}, 1},
		{"copper three hexes away scores 0", MapTile{Q: 8, R: 0, Terrain: TerrainHills, CopperDeposit: true}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tm := map[[2]int]MapTile{{cand.Q, cand.R}: cand, {tc.ore.Q, tc.ore.R}: tc.ore}
			if got := SpawnOreCatchmentScore(cand, tm, halfQ); got != tc.want {
				t.Errorf("SpawnOreCatchmentScore = %d, want %d", got, tc.want)
			}
		})
	}
}
