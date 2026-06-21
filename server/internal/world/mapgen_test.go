package world

import "testing"

// stubID satisfies the worldID parameter of GenerateMap.
type stubID struct{}

func (stubID) String() string { return "test-world" }

// genTiles returns the validated tile set for a seed (effective seed discarded).
// tileIsLand and landComponents now live in mapgen.go (non-test) so validateMap
// can share them.
func genTiles(seed int64, w, h int) []MapTile {
	tiles, _ := GenerateMap(stubID{}, seed, w, h)
	return tiles
}

func TestGenerateMap_TileCountAndBounds(t *testing.T) {
	w, h := 40, 30
	tiles := genTiles(12345, w, h)
	if len(tiles) != w*h {
		t.Fatalf("tile count = %d, want %d", len(tiles), w*h)
	}
	seen := map[[2]int]bool{}
	for _, tile := range tiles {
		// Tiles live in the rectangular offset domain: q in [0,w), and the
		// per-column row (r - rowOrigin(q)) in [0,h). See mapgen.go rowOrigin.
		row := tile.R - rowOrigin(tile.Q, w)
		if tile.Q < 0 || tile.Q >= w || row < 0 || row >= h {
			t.Fatalf("tile out of rectangular domain: (%d,%d) row=%d", tile.Q, tile.R, row)
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

// The invariant is now enforced at generation time: GenerateMap must return a
// map that passes validateMap for every seed AND every map size — including the
// production 56×40 that the unit suite never covered before the 0620 incident.
func TestGenerateMap_InvariantsEnforcedAcrossSizesAndSeeds(t *testing.T) {
	sizes := [][2]int{{40, 30}, {56, 40}, {30, 20}}
	for _, sz := range sizes {
		w, h := sz[0], sz[1]
		for seed := int64(0); seed < 200; seed++ {
			tiles, _ := GenerateMap(stubID{}, seed, w, h)
			if err := validateMap(tiles); err != nil {
				t.Fatalf("%dx%d seed %d: GenerateMap returned invalid map: %v", w, h, seed, err)
			}
		}
		// A spread of large pseudo-random seeds (the live world used one of these).
		for _, seed := range []int64{1781944573308082963, 9223372036854775807, 42424242424242, 7000000000000000001} {
			tiles, eff := GenerateMap(stubID{}, seed, w, h)
			if err := validateMap(tiles); err != nil {
				t.Fatalf("%dx%d seed %d (eff %d): invalid map: %v", w, h, seed, eff, err)
			}
		}
	}
}

// Regression for the 0620 incident: the live world (seed 1781944573308082963 at
// 56×40) shipped with ZERO productive tin — no tin pole, dead MVP loop. Whatever
// path produced that, the guarantee now holds: this exact seed/size must yield a
// map with a real tin pole (>= 2 productive tin) reachable from the wrapper.
func TestGenerateMap_RegressionTinPole_0620(t *testing.T) {
	const seed, w, h = int64(1781944573308082963), 56, 40

	tiles, _ := GenerateMap(stubID{}, seed, w, h)
	if err := validateMap(tiles); err != nil {
		t.Fatalf("0620 seed still produces an invalid map: %v", err)
	}
	tin := 0
	for _, t := range tiles {
		if t.TinDeposit && t.Terrain == TerrainMountainLimestone {
			tin++
		}
	}
	if tin < minProductiveTin {
		t.Fatalf("0620 seed: productive tin = %d, want >= %d (the bug that killed the world)", tin, minProductiveTin)
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
