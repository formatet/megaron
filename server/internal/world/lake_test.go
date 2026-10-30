package world

import "testing"

// MarkLakes: every sea tile cut off from the edge's sea becomes a lake, and
// no lake touches the edge's sea. Rivers do not connect.
func TestMarkLakes_EnclosedSeaOnly(t *testing.T) {
	t.Parallel()
	w, h := 64, 64
	lakes := 0
	for _, seed := range []int64{1, 2, 3, 4} {
		tiles := genTiles(seed, w, h)
		grid := make(map[cell]Terrain, len(tiles))
		for _, tl := range tiles {
			grid[cell{tl.Q, tl.R}] = tl.Terrain
		}
		main := mainSeaComponent(grid, w, h)
		for _, tl := range tiles {
			c := cell{tl.Q, tl.R}
			switch {
			case tl.Terrain == TerrainLake:
				lakes++
				if main[c] {
					t.Errorf("seed %d: lake at %v is part of the edge sea", seed, c)
				}
			case tl.Terrain == TerrainCoastalSea || tl.Terrain == TerrainDeepSea:
				if !main[c] {
					t.Errorf("seed %d: enclosed %s at %v was not marked lake", seed, tl.Terrain, c)
				}
			}
		}
	}
	if lakes == 0 {
		t.Fatal("no lake on four 64x64 maps — the test proves nothing")
	}
}
