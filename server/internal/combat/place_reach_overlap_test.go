package combat

import (
	"testing"

	"formatet/megaron/server/internal/world"
)

// A unit entering a hex that belongs to two places is a target for both
// (platstyper P1): a three-hex hills island is an island and hills at once.
func TestPlaceReachTargetsTwoPlacesOnOneHex(t *testing.T) {
	var tiles []world.MapTile
	for q := 0; q < 8; q++ {
		for r := 0; r < 8; r++ {
			terrain := world.TerrainCoastalSea
			if r == 4 && q >= 3 && q <= 5 {
				terrain = world.TerrainHills
			}
			tiles = append(tiles, world.MapTile{Q: q, R: r, Terrain: terrain})
		}
	}
	// A bigger landmass at the edge keeps the hills piece from being the largest.
	for r := 0; r < 8; r++ {
		for q := 0; q < 2; q++ {
			for i := range tiles {
				if tiles[i].Q == q && tiles[i].R == r {
					tiles[i].Terrain = world.TerrainScrubMaquis
				}
			}
		}
	}
	got := placeReachTargets(tiles)[[2]int{4, 4}]
	kinds := map[string]bool{}
	for _, tg := range got {
		kinds[tg.place.Kind] = true
	}
	if len(got) != 2 || !kinds["island"] || !kinds["hills"] {
		t.Fatalf("hex 4,4 must reach both the island and the hills, got %+v", got)
	}
}
