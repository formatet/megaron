package world

import "testing"

// seaGrid is an n x n axial block of coastal sea; callers paint single hexes.
func seaGrid(n int) map[[2]int]Terrain {
	g := map[[2]int]Terrain{}
	for q := 0; q < n; q++ {
		for r := 0; r < n; r++ {
			g[[2]int{q, r}] = TerrainCoastalSea
		}
	}
	return g
}

func gridTiles(g map[[2]int]Terrain) []MapTile {
	var out []MapTile
	for k, v := range g {
		out = append(out, MapTile{Q: k[0], R: k[1], Terrain: v})
	}
	return out
}

func placesOfKind(tiles []MapTile, kind string) []Place {
	var out []Place
	for _, p := range Places(tiles) {
		if p.Kind == kind {
			out = append(out, p)
		}
	}
	return out
}

// platstyper P1: one mainland (touching the map edge), two one-hex islands, one
// land piece on the map edge, a strait between mainland and island, a cape,
// plains/hills at and just under their minimum, and a one-hex delta.
func platstyperFixture() []MapTile {
	g := seaGrid(20)
	for q := 0; q <= 4; q++ {
		for r := 0; r < 20; r++ {
			g[[2]int{q, r}] = TerrainScrubMaquis
		}
	}
	g[[2]int{8, 4}] = TerrainScrubMaquis   // island
	g[[2]int{6, 10}] = TerrainScrubMaquis  // island; the sea between it and the mainland is a strait
	g[[2]int{19, 10}] = TerrainScrubMaquis // touches the map edge: not an island
	g[[2]int{5, 16}] = TerrainScrubMaquis  // cape
	for i := 0; i < PlainMin; i++ {
		g[[2]int{1, 2 + i}] = TerrainPlains
	}
	for i := 0; i < PlainMin-1; i++ {
		g[[2]int{3, 2 + i}] = TerrainPlains
	}
	for i := 0; i < HillsMin; i++ {
		g[[2]int{1, 12 + i}] = TerrainHills
	}
	for i := 0; i < HillsMin-1; i++ {
		g[[2]int{3, 12 + i}] = TerrainHills
	}
	g[[2]int{2, 17}] = TerrainRiverDelta
	return gridTiles(g)
}

func TestPlacesIslandStraitCape(t *testing.T) {
	t.Parallel()
	tiles := platstyperFixture()
	islands := placesOfKind(tiles, "island")
	if len(islands) != 2 || islands[0].Anchor != [2]int{6, 10} || islands[1].Anchor != [2]int{8, 4} {
		t.Fatalf("want the two inner one-hex islands, not the mainland nor the edge piece: %+v", islands)
	}
	straits := placesOfKind(tiles, "strait")
	if len(straits) != 1 || len(straits[0].Hexes) != 2 || straits[0].Anchor != [2]int{5, 10} {
		t.Fatalf("want one two-hex strait between mainland and island: %+v", straits)
	}
	capes := placesOfKind(tiles, "cape")
	if len(capes) != 1 || capes[0].Anchor != [2]int{5, 16} {
		t.Fatalf("want one cape: %+v", capes)
	}
	for _, k := range []string{"island", "strait", "cape", "plain", "hills", "delta"} {
		if !IsPlaceKind(k) {
			t.Fatalf("IsPlaceKind(%s) false", k)
		}
	}
}

// The biggest landmass is never an island, even when it floats wholly inside the map.
func TestPlacesLargestLandmassIsNotAnIsland(t *testing.T) {
	t.Parallel()
	g := seaGrid(14)
	for q := 3; q <= 7; q++ {
		for r := 3; r <= 7; r++ {
			g[[2]int{q, r}] = TerrainScrubMaquis
		}
	}
	g[[2]int{11, 11}] = TerrainScrubMaquis
	islands := placesOfKind(gridTiles(g), "island")
	if len(islands) != 1 || islands[0].Anchor != [2]int{11, 11} {
		t.Fatalf("only the small landmass is an island: %+v", islands)
	}
}

func TestPlacesPlainHillsDeltaMinimums(t *testing.T) {
	t.Parallel()
	tiles := platstyperFixture()
	for kind, size := range map[string]int{"plain": PlainMin, "hills": HillsMin} {
		ps := placesOfKind(tiles, kind)
		if len(ps) != 1 || len(ps[0].Hexes) != size {
			t.Fatalf("%s: want one component of exactly the minimum %d, got %+v", kind, size, ps)
		}
	}
	if d := placesOfKind(tiles, "delta"); len(d) != 1 || d[0].Anchor != [2]int{2, 17} {
		t.Fatalf("a one-hex delta is a place: %+v", d)
	}
}
