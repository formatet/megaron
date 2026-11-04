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
// land piece on the map edge, a strait between mainland and islands, a cape,
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
	// The mainland and both islands lie within W hexes of each other: one connected strait
	// (the strip along the mainland coast) that contains the one-hex gap at (5,10).
	if len(straits) != 1 {
		t.Fatalf("want one strait between mainland and islands: %+v", straits)
	}
	has := false
	for _, h := range straits[0].Hexes {
		has = has || h == [2]int{5, 10}
	}
	if !has {
		t.Fatalf("the gap hex (5,10) is part of the strait: %+v", straits[0])
	}
	capes := placesOfKind(tiles, "cape")
	if len(capes) != 1 || capes[0].Anchor != [2]int{5, 16} {
		t.Fatalf("want one cape: %+v", capes)
	}
	for _, k := range []string{"island", "strait", "bay", "cape", "plain", "hills", "delta"} {
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

// landBlock paints a q x r rectangle of land onto g.
func landBlock(g map[[2]int]Terrain, q0, q1, r0, r1 int) {
	for q := q0; q <= q1; q++ {
		for r := r0; r <= r1; r++ {
			g[[2]int{q, r}] = TerrainScrubMaquis
		}
	}
}

func kindCount(tiles []MapTile, kind string) int { return len(placesOfKind(tiles, kind)) }

// Two islands with a 2-hex gap: ONE strait (the whole strip), and no bay.
func TestPlacesStraitIsTwoLandmassesWithNarrowGap(t *testing.T) {
	t.Parallel()
	g := seaGrid(12)
	landBlock(g, 0, 3, 0, 11)
	landBlock(g, 6, 9, 0, 11)
	tiles := gridTiles(g)
	if n := kindCount(tiles, "strait"); n != 1 {
		t.Fatalf("want one strait, got %d", n)
	}
	if n := kindCount(tiles, "bay"); n != 0 {
		t.Fatalf("a strait is not a bay, got %d", n)
	}
}

// The width limit W: a gap of W hexes is a strait, W+1 is open sea.
func TestPlacesStraitWidthLimit(t *testing.T) {
	t.Parallel()
	for gap := 1; gap <= StraitMaxWidth+2; gap++ {
		g := seaGrid(16)
		landBlock(g, 0, 3, 0, 15)
		landBlock(g, 4+gap, 12, 0, 15)
		want := 1
		if gap > StraitMaxWidth {
			want = 0
		}
		if n := kindCount(gridTiles(g), "strait"); n != want {
			t.Fatalf("gap %d: want %d straits, got %d", gap, want, n)
		}
	}
}

// One hex with land on four+ sides is a bay and never a strait; contiguous bay hexes are one bay.
func TestPlacesBayIsCoastalSeaWithLandOnFourSides(t *testing.T) {
	t.Parallel()
	g := seaGrid(12)
	landBlock(g, 2, 8, 2, 8)
	g[[2]int{5, 5}] = TerrainCoastalSea
	g[[2]int{6, 5}] = TerrainCoastalSea
	tiles := gridTiles(g)
	bays := placesOfKind(tiles, "bay")
	if len(bays) != 1 || len(bays[0].Hexes) != 2 || bays[0].Anchor != [2]int{5, 5} {
		t.Fatalf("want one two-hex bay anchored at its first hex: %+v", bays)
	}
	if n := kindCount(tiles, "strait"); n != 0 {
		t.Fatalf("a bay of a single landmass is no strait: %d", n)
	}
}

// A deep inlet cut into ONE landmass is bay water, never a strait: the "two different landmasses" rule.
func TestPlacesInletOfOneLandmassIsNotStrait(t *testing.T) {
	t.Parallel()
	g := seaGrid(12)
	landBlock(g, 2, 8, 2, 8)
	for q := 2; q <= 5; q++ {
		g[[2]int{q, 5}] = TerrainCoastalSea
	}
	tiles := gridTiles(g)
	if n := kindCount(tiles, "strait"); n != 0 {
		t.Fatalf("an inlet of one landmass is no strait: %d", n)
	}
	if n := kindCount(tiles, "bay"); n == 0 {
		t.Fatal("the inlet should hold a bay")
	}
}

// Lake water is neither bay nor strait, and deep sea is not a bay (the fishing rule says coastal_sea).
func TestPlacesLakeAndDeepSeaAreNotBays(t *testing.T) {
	t.Parallel()
	g := seaGrid(12)
	landBlock(g, 2, 8, 2, 8)
	g[[2]int{5, 5}] = TerrainLake
	g[[2]int{7, 4}] = TerrainDeepSea
	tiles := gridTiles(g)
	if n := kindCount(tiles, "bay") + kindCount(tiles, "strait"); n != 0 {
		t.Fatalf("lake/deep water inside land is neither bay nor strait: %d", n)
	}
}

// The map edge is not land: an edge hex with three land neighbours is no bay,
// and a lone landmass beside the edge makes no strait.
func TestPlacesMapEdgeIsNotLand(t *testing.T) {
	t.Parallel()
	g := seaGrid(8)
	g[[2]int{1, 4}] = TerrainScrubMaquis
	g[[2]int{0, 3}] = TerrainScrubMaquis
	g[[2]int{0, 5}] = TerrainScrubMaquis // (0,4) has 3 land neighbours, three more beyond the edge
	tiles := gridTiles(g)
	if n := kindCount(tiles, "bay"); n != 0 {
		t.Fatalf("edge counted as land: %d bays", n)
	}
	g2 := seaGrid(10)
	landBlock(g2, 4, 9, 0, 9)
	if n := kindCount(gridTiles(g2), "strait"); n != 0 {
		t.Fatalf("one landmass and the edge is no strait: %d", n)
	}
}

// A lake between two landmasses is lake water: no strait, no bay.
func TestPlacesLakeBetweenLandmassesIsNotStrait(t *testing.T) {
	t.Parallel()
	g := seaGrid(12)
	landBlock(g, 0, 3, 0, 11)
	landBlock(g, 6, 9, 0, 11)
	for r := 0; r < 12; r++ {
		g[[2]int{4, r}] = TerrainLake
		g[[2]int{5, r}] = TerrainLake
	}
	tiles := gridTiles(g)
	if n := kindCount(tiles, "strait") + kindCount(tiles, "bay"); n != 0 {
		t.Fatalf("lake water is neither strait nor bay: %d", n)
	}
}
