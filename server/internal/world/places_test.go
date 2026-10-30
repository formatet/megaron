package world

import (
	"reflect"
	"testing"
)

func TestPlacesKindsMinimumAndCanonicalAnchor(t *testing.T) {
	t.Parallel()
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

// Lakes are nameable at every size, like rivers (insjöar slice 3, Timothy 2026-10-30).
func TestPlacesLakeEverySize(t *testing.T) {
	t.Parallel()
	tiles := []MapTile{
		{Q: 0, Terrain: TerrainLake},
		{Q: 5, Terrain: TerrainLake}, {Q: 6, Terrain: TerrainLake}, {Q: 7, Terrain: TerrainLake},
		{Q: 10, Terrain: TerrainCoastalSea},
	}
	var lakes []Place
	for _, p := range Places(tiles) {
		if p.Kind == "lake" {
			lakes = append(lakes, p)
		}
	}
	if len(lakes) != 2 || len(lakes[0].Hexes)+len(lakes[1].Hexes) != 4 {
		t.Fatalf("want a one-hex and a three-hex lake, coastal water excluded; got %+v", lakes)
	}
	if !IsPlaceKind("lake") || IsPlaceKind("sea") {
		t.Fatal("place kind owner disagrees with Places")
	}
}
