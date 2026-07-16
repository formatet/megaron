package world

import (
	"fmt"
	"testing"
)

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
			if err := validateMap(tiles, w, h); err != nil {
				t.Fatalf("%dx%d seed %d: GenerateMap returned invalid map: %v", w, h, seed, err)
			}
		}
		// A spread of large pseudo-random seeds (the live world used one of these).
		for _, seed := range []int64{1781944573308082963, 9223372036854775807, 42424242424242, 7000000000000000001} {
			tiles, eff := GenerateMap(stubID{}, seed, w, h)
			if err := validateMap(tiles, w, h); err != nil {
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
	if err := validateMap(tiles, w, h); err != nil {
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

// Fas 1a — handelskedjan: every generated map must have ≥1 buildable west tile
// with copper in its 6-hex catchment AND ≥1 buildable east tile with tin in its
// catchment. This guarantees the first wanax to settle on such a tile produces
// ore from turn 1, so the cross-sea bronze trade can be demonstrated without
// depending on the oracle (which was broken until Fas 1b).
func TestGenerateMap_OreCatchmentInBothHemispheres(t *testing.T) {
	sizes := [][2]int{{40, 30}, {56, 40}}
	for _, sz := range sizes {
		w, h := sz[0], sz[1]
		for seed := int64(0); seed < 20; seed++ {
			tiles, _ := GenerateMap(stubID{}, seed, w, h)
			if err := checkOreCatchmentInvariants(tiles); err != nil {
				t.Fatalf("%dx%d seed %d: %v", w, h, seed, err)
			}
		}
	}
}

// checkOreCatchmentInvariants is the pure-logic extraction of the validateMap
// ore-catchment check so we can call it in tests independently.
func checkOreCatchmentInvariants(tiles []MapTile) error {
	tileMap := make(map[[2]int]MapTile, len(tiles))
	maxQ := 0
	for _, t := range tiles {
		tileMap[[2]int{t.Q, t.R}] = t
		if t.Q > maxQ {
			maxQ = t.Q
		}
	}
	halfQ := maxQ / 2
	dirs6 := [6][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
	isBuildable := func(t MapTile) bool {
		switch t.Terrain {
		case TerrainCoastalSea, TerrainDeepSea,
			TerrainMountainLimestone, TerrainMountainRed, TerrainSemiDesert:
			return false
		}
		return true
	}

	westCopper, eastTin := false, false
	for _, t := range tiles {
		if !isBuildable(t) {
			continue
		}
		var hasCopper, hasTin bool
		for _, d := range dirs6 {
			nb, ok := tileMap[[2]int{t.Q + d[0], t.R + d[1]}]
			if !ok {
				continue
			}
			if nb.CopperDeposit {
				hasCopper = true
			}
			if nb.TinDeposit {
				hasTin = true
			}
		}
		if hasCopper && t.Q <= halfQ {
			westCopper = true
		}
		if hasTin && t.Q > halfQ {
			eastTin = true
		}
		if westCopper && eastTin {
			break
		}
	}
	if !westCopper {
		return fmt.Errorf("no buildable west tile (q <= %d) has copper in its 6-hex catchment", halfQ)
	}
	if !eastTin {
		return fmt.Errorf("no buildable east tile (q > %d) has tin in its 6-hex catchment", halfQ)
	}
	return nil
}

// TestOracleReveal_EastTileFindsTinCatchment simulates the repaired oracle query
// logic on a generated tile set: given a realistic eastern capital position
// (closest buildable tile to the nearest tin deposit), there must exist a
// buildable tile within oracleRadius=8 whose 6-hex catchment contains tin.
// This test runs on map-tile data only (no DB), confirming that the Fas 1b fix
// (search map_tiles instead of provinces) would find tin where the old
// provinces-join never could (which searched only the provinces table).
func TestOracleReveal_EastTileFindsTinCatchment(t *testing.T) {
	const oracleRadius = 8
	tiles := genTiles(42, 56, 40)

	tileMap := make(map[[2]int]MapTile, len(tiles))
	for _, tile := range tiles {
		tileMap[[2]int{tile.Q, tile.R}] = tile
	}

	dirs6 := [6][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
	isBuildable := func(tile MapTile) bool {
		switch tile.Terrain {
		case TerrainCoastalSea, TerrainDeepSea,
			TerrainMountainLimestone, TerrainMountainRed, TerrainSemiDesert:
			return false
		}
		return true
	}
	axialDist := func(aq, ar, bq, br int) int {
		dq := aq - bq
		dr := ar - br
		d := dq
		if d < 0 {
			d = -d
		}
		s := dq + dr
		if s < 0 {
			s = -s
		}
		e := dr
		if e < 0 {
			e = -e
		}
		return (d + s + e) / 2
	}

	// Find the buildable tile closest to any tin deposit (simulates a realistic
	// eastern capital that spawns near Anatolia). There must be such a tile since
	// the Fas 1a invariant guarantees eastTinCatchment is true for every valid map.
	bestDist := 1<<31 - 1
	var originQ, originR int
	for _, tile := range tiles {
		if !isBuildable(tile) {
			continue
		}
		for _, d := range dirs6 {
			nb, ok := tileMap[[2]int{tile.Q + d[0], tile.R + d[1]}]
			if !ok {
				continue
			}
			if nb.TinDeposit {
				// This tile is a tin-catchment site — distance from it to nearest tin = 1.
				// Pick the one with the highest q (most eastern) to simulate Anatolia wanax.
				dist := axialDist(tile.Q, tile.R, nb.Q, nb.R)
				if dist < bestDist || (dist == bestDist && tile.Q > originQ) {
					bestDist = dist
					originQ, originR = tile.Q, tile.R
				}
			}
		}
	}
	if bestDist == 1<<31-1 {
		t.Fatal("no buildable tile with a tin-deposit neighbour found — Fas 1a invariant may have failed")
	}

	// Simulate oracle search: origin is the eastern capital found above.
	// Confirm that from this origin, the oracle (radius=8) finds ≥1 buildable
	// tile with tin in catchment — specifically, the origin itself qualifies
	// since it already has a tin neighbour.
	foundTinSite := false
	for _, tile := range tiles {
		if !isBuildable(tile) {
			continue
		}
		if axialDist(tile.Q, tile.R, originQ, originR) > oracleRadius {
			continue
		}
		for _, d := range dirs6 {
			nb, ok := tileMap[[2]int{tile.Q + d[0], tile.R + d[1]}]
			if !ok {
				continue
			}
			if nb.TinDeposit {
				foundTinSite = true
				break
			}
		}
		if foundTinSite {
			break
		}
	}

	if !foundTinSite {
		t.Errorf("oracle from east origin (%d,%d): no buildable tile within radius %d has tin in catchment — oracle fix may not work for seed 42, 56x40",
			originQ, originR, oracleRadius)
	}
}

// TestSpawnOreCatchmentScore_BiasSelection verifies that SpawnOreCatchmentScore
// returns 1 for tiles whose 6-hex catchment contains the hemisphere's ore and 0
// for tiles that do not. This mirrors the ORDER BY ore-bias CASE in join.go: when
// both a biased and an unbiased tile are viable, the biased one sorts first.
//
// The test builds a minimal synthetic tile set (no DB) so it is fast and
// deterministic. Each sub-test names the exact scenario it exercises.
func TestSpawnOreCatchmentScore_BiasSelection(t *testing.T) {
	halfQ := 10 // arbitrary midpoint; west = q<=10, east = q>10

	// Helper: build a tileMap from a slice.
	makeTileMap := func(tiles []MapTile) map[[2]int]MapTile {
		m := make(map[[2]int]MapTile, len(tiles))
		for _, t := range tiles {
			m[[2]int{t.Q, t.R}] = t
		}
		return m
	}

	t.Run("west tile with copper neighbour scores 1", func(t *testing.T) {
		// q=5 is west (<=halfQ=10). Its neighbour at q+1=6 has copper.
		candidateW := MapTile{Q: 5, R: 0, Terrain: TerrainPlains}
		copperNeighbour := MapTile{Q: 6, R: 0, Terrain: TerrainHills, CopperDeposit: true}
		tm := makeTileMap([]MapTile{candidateW, copperNeighbour})
		if got := SpawnOreCatchmentScore(candidateW, tm, halfQ); got != 1 {
			t.Errorf("west+copper: SpawnOreCatchmentScore = %d, want 1", got)
		}
	})

	t.Run("west tile without copper neighbour scores 0", func(t *testing.T) {
		candidateW := MapTile{Q: 5, R: 0, Terrain: TerrainPlains}
		tinNeighbour := MapTile{Q: 6, R: 0, Terrain: TerrainMountainLimestone, TinDeposit: true}
		tm := makeTileMap([]MapTile{candidateW, tinNeighbour})
		if got := SpawnOreCatchmentScore(candidateW, tm, halfQ); got != 0 {
			t.Errorf("west+tin(wrong ore): SpawnOreCatchmentScore = %d, want 0", got)
		}
	})

	t.Run("east tile with tin neighbour scores 1", func(t *testing.T) {
		candidateE := MapTile{Q: 15, R: 0, Terrain: TerrainPlains}
		tinNeighbour := MapTile{Q: 16, R: 0, Terrain: TerrainMountainLimestone, TinDeposit: true}
		tm := makeTileMap([]MapTile{candidateE, tinNeighbour})
		if got := SpawnOreCatchmentScore(candidateE, tm, halfQ); got != 1 {
			t.Errorf("east+tin: SpawnOreCatchmentScore = %d, want 1", got)
		}
	})

	t.Run("east tile without tin neighbour scores 0", func(t *testing.T) {
		candidateE := MapTile{Q: 15, R: 0, Terrain: TerrainPlains}
		copperNeighbour := MapTile{Q: 16, R: 0, Terrain: TerrainHills, CopperDeposit: true}
		tm := makeTileMap([]MapTile{candidateE, copperNeighbour})
		if got := SpawnOreCatchmentScore(candidateE, tm, halfQ); got != 0 {
			t.Errorf("east+copper(wrong ore): SpawnOreCatchmentScore = %d, want 0", got)
		}
	})

	t.Run("ore neighbour not adjacent scores 0", func(t *testing.T) {
		// q=5 west candidate, copper 2 hexes away (not a direct neighbour).
		candidateW := MapTile{Q: 5, R: 0, Terrain: TerrainPlains}
		farCopper := MapTile{Q: 7, R: 0, Terrain: TerrainHills, CopperDeposit: true}
		tm := makeTileMap([]MapTile{candidateW, farCopper})
		if got := SpawnOreCatchmentScore(candidateW, tm, halfQ); got != 0 {
			t.Errorf("far copper: SpawnOreCatchmentScore = %d, want 0", got)
		}
	})
}

// TestSpawnOreCatchmentScore_RealMap verifies that on a real generated map (seed 42,
// 56×40) there are tiles in both hemispheres that score 1 — i.e. the bias is
// actually usable when the world map is generated. This links the pure-function
// contract above to real mapgen output.
func TestSpawnOreCatchmentScore_RealMap(t *testing.T) {
	tiles := genTiles(42, 56, 40)
	tileMap := make(map[[2]int]MapTile, len(tiles))
	maxQ := 0
	for _, tile := range tiles {
		tileMap[[2]int{tile.Q, tile.R}] = tile
		if tile.Q > maxQ {
			maxQ = tile.Q
		}
	}
	halfQ := (maxQ) / 2 // mirrors join.go: halfQ = (mapWidth-1)/2, maxQ = mapWidth-1

	isBuildable := func(tile MapTile) bool {
		switch tile.Terrain {
		case TerrainCoastalSea, TerrainDeepSea,
			TerrainMountainLimestone, TerrainMountainRed, TerrainSemiDesert:
			return false
		}
		return true
	}

	westScored, eastScored := false, false
	for _, tile := range tiles {
		if !isBuildable(tile) {
			continue
		}
		score := SpawnOreCatchmentScore(tile, tileMap, halfQ)
		if score == 1 && tile.Q <= halfQ {
			westScored = true
		}
		if score == 1 && tile.Q > halfQ {
			eastScored = true
		}
		if westScored && eastScored {
			break
		}
	}
	if !westScored {
		t.Errorf("seed 42, 56x40: no buildable west tile (q<=%d) has copper in 6-hex catchment — bias would be inert for west hemisphere", halfQ)
	}
	if !eastScored {
		t.Errorf("seed 42, 56x40: no buildable east tile (q>%d) has tin in 6-hex catchment — bias would be inert for east hemisphere", halfQ)
	}
}

// TestGenerateMap_EveryRiverReachesDelta is the black-box counterpart to the
// panic-based invariant addRiver asserts internally (mapgen.go): every
// connected clump of river_valley/river_delta tiles must contain at least one
// river_delta tile. This is the P3 regression for the "Amyklai-class" silent
// failure (temenos_mapgen.md §Kända begränsningar) — a river that reached the
// coast but produced no delta, caught only by reading DB rows by hand. Since
// the old random-walk river is gone (replaced by steepest-descent + pit-fill
// over the height field, which by construction never carves a tile unless it
// already reached the sea), this should never fail — that is exactly the
// point of asserting it as a permanent test, not just an ad-hoc PNG look.
//
// Sizes/seed counts mirror the plan's verification ask (20 seeds at 56×40, a
// few at 120×84 and 230×230 — kept small there so `go test` stays fast).
func TestGenerateMap_EveryRiverReachesDelta(t *testing.T) {
	cases := []struct {
		w, h, seeds int
	}{
		{56, 40, 20},
		{120, 84, 5},
		{230, 230, 3},
	}
	for _, tc := range cases {
		for seed := int64(0); seed < int64(tc.seeds); seed++ {
			tiles, eff := GenerateMap(stubID{}, seed, tc.w, tc.h)

			terrain := make(map[cell]Terrain, len(tiles))
			for _, t := range tiles {
				terrain[cell{t.Q, t.R}] = t.Terrain
			}

			// Connected components over river_valley + river_delta tiles only
			// (hex adjacency, same 6 axial directions as landComponents).
			dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
			isRiver := func(t Terrain) bool { return t == TerrainRiverValley || t == TerrainRiverDelta }
			seen := map[cell]bool{}
			var valleyTiles, deltaTiles int
			riverComponents := 0
			for c, terr := range terrain {
				if !isRiver(terr) || seen[c] {
					continue
				}
				riverComponents++
				hasDelta := false
				queue := []cell{c}
				seen[c] = true
				for len(queue) > 0 {
					cur := queue[0]
					queue = queue[1:]
					if terrain[cur] == TerrainRiverDelta {
						hasDelta = true
					}
					for _, d := range dirs {
						n := cell{cur.q + d[0], cur.r + d[1]}
						nt, ok := terrain[n]
						if !ok || !isRiver(nt) || seen[n] {
							continue
						}
						seen[n] = true
						queue = append(queue, n)
					}
				}
				if !hasDelta {
					t.Fatalf("%dx%d seed %d (eff %d): a river component has no delta tile — Amyklai-class failure regressed",
						tc.w, tc.h, seed, eff)
				}
			}
			for _, terr := range terrain {
				switch terr {
				case TerrainRiverValley:
					valleyTiles++
				case TerrainRiverDelta:
					deltaTiles++
				}
			}
			if riverComponents == 0 {
				t.Fatalf("%dx%d seed %d (eff %d): no river tiles at all", tc.w, tc.h, seed, eff)
			}
			t.Logf("%dx%d seed %d (eff %d): %d rivers, %d river_valley tiles, %d river_delta tiles",
				tc.w, tc.h, seed, eff, riverComponents, valleyTiles, deltaTiles)
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

// TestGenerateMap_TinCapAndSpread is the P4 regression for the deposit-
// clustering rewrite (plan §P4-A/D): tin_sources (connected components of
// tin-deposit tiles) must never exceed the design-invariant tinSourceCap on
// any map size — the pre-P4 per-hex-% roll let tin explode to 48 hexes on
// one seed and monopolise a single cluster on another (plan §P4 empirics).
//
// Once a map has >=2 tin sources AND the underlying candidate terrain
// (mountain_limestone on tin-biased land) actually spans >=2 distinct land
// components, at least 2 sources must land on different components — the
// monopoly case plan §A names explicitly (230×230 seed 303: 6 tin hexes, one
// area). When the candidate terrain itself only exists on a single
// landmass, spread is structurally impossible — plan §A: "Får kandidaterna
// bara ihop en landmassa: acceptera + logga, validateMap avgör" — so that
// case is skipped, not failed.
func TestGenerateMap_TinCapAndSpread(t *testing.T) {
	sizes := [][2]int{{56, 40}, {120, 84}, {230, 230}}
	for _, sz := range sizes {
		w, h := sz[0], sz[1]
		for seed := int64(0); seed < 10; seed++ {
			tiles, eff := GenerateMap(stubID{}, seed, w, h)

			sources := depositSourceCount(tiles, func(t MapTile) bool { return t.TinDeposit })
			if sources > tinSourceCap {
				t.Fatalf("%dx%d seed %d (eff %d): tin_sources = %d, want <= %d",
					w, h, seed, eff, sources, tinSourceCap)
			}
			if sources < 2 {
				continue // spread is only meaningful once >=2 sources exist
			}

			comp := landComponents(tiles)
			_, chanE := seaChannels(w)
			candidateComps := map[int]bool{}
			for _, t := range tiles {
				if t.Terrain == TerrainMountainLimestone && t.Q > chanE {
					candidateComps[comp[[2]int{t.Q, t.R}]] = true
				}
			}
			if len(candidateComps) < 2 {
				continue // candidate terrain itself only spans one landmass
			}

			tinLandmasses := map[int]bool{}
			for _, t := range tiles {
				if t.TinDeposit {
					tinLandmasses[comp[[2]int{t.Q, t.R}]] = true
				}
			}
			if len(tinLandmasses) < 2 {
				t.Fatalf("%dx%d seed %d (eff %d): %d tin sources, %d candidate landmasses, but all sources on a single land component (monopoly)",
					w, h, seed, eff, sources, len(candidateComps))
			}
		}
	}
}
