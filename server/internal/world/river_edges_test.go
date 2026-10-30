package world

import (
	"reflect"
	"testing"
)

func TestRiverEdgesUseSavedWaterGeometryAndWebDirectionOrder(t *testing.T) {
	t.Parallel()
	tiles := []MapTile{{Q: 0, R: 0, Terrain: TerrainRiver}}
	// Six distinct water neighbours exercise the protocol's full index order.
	offsets := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
	for i, xy := range offsets {
		terrain := TerrainRiver
		if i%2 == 0 {
			terrain = TerrainRiverFord
		}
		tiles = append(tiles, MapTile{Q: xy[0], R: xy[1], Terrain: terrain})
	}
	if got := RiverEdges(tiles)[[2]int{0, 0}]; !reflect.DeepEqual(got, []int{0, 1, 2, 3, 4, 5}) {
		t.Fatalf("edge order=%v", got)
	}
	// River delta and valley are land, never water edges. Source has no
	// synthetic branch; a mouth connects to actual sea without naming its type.
	tiles = []MapTile{{Q: 0, R: 0, Terrain: TerrainRiver}, {Q: 1, R: 0, Terrain: TerrainRiverDelta}, {Q: 0, R: 1, Terrain: TerrainRiverValley}, {Q: 10, R: 0, Terrain: TerrainRiverFord}, {Q: 11, R: 0, Terrain: TerrainCoastalSea}, {Q: 20, R: 0, Terrain: TerrainRiver}, {Q: 19, R: 0, Terrain: TerrainDeepSea}}
	got := RiverEdges(tiles)
	for xy, want := range map[[2]int][]int{{0, 0}: {}, {10, 0}: {0}, {20, 0}: {1}} {
		if !reflect.DeepEqual(got[xy], want) {
			t.Errorf("river%v edges=%v want%v", xy, got[xy], want)
		}
	}
	for _, xy := range [][2]int{{1, 0}, {0, 1}, {11, 0}, {19, 0}} {
		if _, ok := got[xy]; ok {
			t.Errorf("nonriver%v gained metadata", xy)
		}
	}
	for i, j := 0, len(tiles)-1; i < j; i, j = i+1, j-1 {
		tiles[i], tiles[j] = tiles[j], tiles[i]
	}
	if !reflect.DeepEqual(got, RiverEdges(tiles)) {
		t.Fatal("tile row order changed river edges")
	}
}
