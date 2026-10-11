package world

import (
	"reflect"
	"testing"
)

func TestPlacesKindsMinimumAndCanonicalAnchor(t *testing.T) {
	tiles := nameFixture()
	for i := 0; i < 5; i++ {
		terrain := TerrainMountainLimestone
		if i%2 == 1 {
			terrain = TerrainMountainRed
		}
		tiles = append(tiles, MapTile{Q: 40 + i, Terrain: terrain}, MapTile{Q: 50 + i, Terrain: TerrainSemiDesert}, MapTile{Q: 60 + i, Terrain: TerrainForestCedar}, MapTile{Q: 70 + i, Terrain: TerrainForestOliveGrove})
	}
	for i := 0; i < 4; i++ {
		tiles = append(tiles, MapTile{Q: 80 + i, Terrain: TerrainForestCedar}, MapTile{Q: 90 + i, Terrain: TerrainSemiDesert}, MapTile{Q: 100 + i, Terrain: TerrainMountainRed})
	}
	want := Places(tiles)
	if len(want) != 5 {
		t.Fatalf("want two rivers and one each massif/desert/cedar forest, got %+v", want)
	}
	for _, p := range want {
		if p.Kind != "river" && len(p.Hexes) != 5 {
			t.Fatalf("minimum differs: %+v", p)
		}
		if p.Anchor != p.Hexes[0] {
			t.Fatalf("noncanonical anchor: %+v", p)
		}
	}
	for i, j := 0, len(tiles)-1; i < j; i, j = i+1, j-1 {
		tiles[i], tiles[j] = tiles[j], tiles[i]
	}
	if !reflect.DeepEqual(want, Places(tiles)) {
		t.Fatal("SQL order changes place identities")
	}
}
