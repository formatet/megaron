package world

import "testing"

// stubID satisfies the worldID parameter of GenerateMap.
type stubID struct{}

func (stubID) String() string { return "test-world" }

func genTiles(seed int64, w, h int) []MapTile {
	return GenerateMap(stubID{}, seed, w, h)
}

func tileIsLand(t Terrain) bool {
	return t != TerrainDeepSea && t != TerrainCoastalSea
}

// landComponents groups contiguous land tiles into connected components and
// returns, for each tile coordinate, the component ID it belongs to.
func landComponents(tiles []MapTile) map[[2]int]int {
	terrain := map[[2]int]Terrain{}
	for _, t := range tiles {
		terrain[[2]int{t.Q, t.R}] = t.Terrain
	}
	comp := map[[2]int]int{}
	next := 0
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
	for _, t := range tiles {
		key := [2]int{t.Q, t.R}
		if !tileIsLand(t.Terrain) {
			continue
		}
		if _, seen := comp[key]; seen {
			continue
		}
		id := next
		next++
		queue := [][2]int{key}
		comp[key] = id
		for len(queue) > 0 {
			c := queue[0]
			queue = queue[1:]
			for _, d := range dirs {
				n := [2]int{c[0] + d[0], c[1] + d[1]}
				tt, ok := terrain[n]
				if !ok || !tileIsLand(tt) {
					continue
				}
				if _, seen := comp[n]; seen {
					continue
				}
				comp[n] = id
				queue = append(queue, n)
			}
		}
	}
	return comp
}

func TestGenerateMap_TileCountAndBounds(t *testing.T) {
	w, h := 40, 30
	tiles := genTiles(12345, w, h)
	if len(tiles) != w*h {
		t.Fatalf("tile count = %d, want %d", len(tiles), w*h)
	}
	seen := map[[2]int]bool{}
	for _, tile := range tiles {
		if tile.Q < 0 || tile.Q >= w || tile.R < 0 || tile.R >= h {
			t.Fatalf("tile out of bounds: (%d,%d)", tile.Q, tile.R)
		}
		k := [2]int{tile.Q, tile.R}
		if seen[k] {
			t.Fatalf("duplicate tile (%d,%d)", tile.Q, tile.R)
		}
		seen[k] = true
	}
}

func TestGenerateMap_Deterministic(t *testing.T) {
	a := genTiles(999, 40, 30)
	b := genTiles(999, 40, 30)
	if len(a) != len(b) {
		t.Fatalf("len mismatch %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("tile %d differs: %+v vs %+v", i, a[i], b[i])
		}
	}
}

// Deposits must sit on terrain that has a matching production rule, otherwise
// they are dead (copper rule = hills, tin rule = mountain_limestone).
func TestGenerateMap_DepositsOnProductiveTerrain(t *testing.T) {
	for seed := int64(0); seed < 20; seed++ {
		tiles := genTiles(seed, 40, 30)
		copper, tin, cedar := 0, 0, 0
		for _, tile := range tiles {
			if tile.CopperDeposit {
				copper++
				if tile.Terrain != TerrainHills {
					t.Fatalf("seed %d: copper deposit on %s (want hills)", seed, tile.Terrain)
				}
			}
			if tile.TinDeposit {
				tin++
				if tile.Terrain != TerrainMountainLimestone {
					t.Fatalf("seed %d: tin deposit on %s (want mountain_limestone)", seed, tile.Terrain)
				}
			}
			if tile.CedarDeposit {
				cedar++
				if tile.Terrain != TerrainForestOliveGrove {
					t.Fatalf("seed %d: cedar deposit on %s (want forest_olive_grove)", seed, tile.Terrain)
				}
			}
			if tile.SilverDeposit && tile.Terrain != TerrainHills && tile.Terrain != TerrainMountainLimestone {
				t.Fatalf("seed %d: silver deposit on %s", seed, tile.Terrain)
			}
		}
		if copper < 2 {
			t.Fatalf("seed %d: only %d copper deposits, want >=2", seed, copper)
		}
		if tin < 2 {
			t.Fatalf("seed %d: only %d tin deposits, want >=2", seed, tin)
		}
		if cedar < 2 {
			t.Fatalf("seed %d: only %d cedar deposits, want >=2", seed, cedar)
		}
	}
}

// Bronze must require sea trade: no land component may hold both copper and tin.
func TestGenerateMap_CopperTinSeaSeparated(t *testing.T) {
	for seed := int64(0); seed < 20; seed++ {
		tiles := genTiles(seed, 40, 30)
		comp := landComponents(tiles)
		copperComps := map[int]bool{}
		tinComps := map[int]bool{}
		for _, tile := range tiles {
			k := [2]int{tile.Q, tile.R}
			if tile.CopperDeposit {
				copperComps[comp[k]] = true
			}
			if tile.TinDeposit {
				tinComps[comp[k]] = true
			}
		}
		for c := range copperComps {
			if tinComps[c] {
				t.Fatalf("seed %d: copper and tin share land component %d", seed, c)
			}
		}
	}
}

// It is an archipelago: many distinct landmasses separated by sea.
func TestGenerateMap_IsArchipelago(t *testing.T) {
	for seed := int64(0); seed < 20; seed++ {
		tiles := genTiles(seed, 40, 30)
		comp := landComponents(tiles)
		ids := map[int]bool{}
		sea := 0
		for _, tile := range tiles {
			if !tileIsLand(tile.Terrain) {
				sea++
				continue
			}
			ids[comp[[2]int{tile.Q, tile.R}]] = true
		}
		if len(ids) < 4 {
			t.Fatalf("seed %d: only %d landmasses, want >=4 (archipelago)", seed, len(ids))
		}
		if sea == 0 {
			t.Fatalf("seed %d: no sea tiles", seed)
		}
	}
}
