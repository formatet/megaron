package world

import (
	"fmt"
	"log/slog"
	"math/rand"
	"sort"
	"strings"
)

type cell struct{ q, r int }

// Deposit bias of a landmass. Copper-biased landmasses live in the western
// hemisphere, tin-biased in the eastern — so the strategic-metal halves can
// never connect overland (bronze always demands sea trade). Neutral landmasses
// (Crete, the Cyclades) sit in the central channel and carry neither metal.
const (
	biasNeutral = iota
	biasCopper
	biasTin
)

// landmass IDs are assigned sequentially as masses are placed; 0 is reserved
// for open sea. The per-ID bias lives in the `bias` map built during generation.
const lmSea = 0

// GenerateMap procedurally generates a hex grid for a world using a seeded RNG.
//
// v3 — Mycenaean archipelago. Instead of two big blobs it lays down a
// recognisable east-Mediterranean spread that scales with map area:
//   - Mainland (west, large)        — copper hills.
//   - Anatolia (east, large)        — tin mountains + cedar forests.
//   - Crete    (south-centre, med)  — neutral trade hub.
//   - Cyclades (centre, scaled N)   — a string of small neutral stepping-stones.
//   - One remote copper island and one remote tin island — isolated productive
//     sources that force overseas trade.
//
// Guarantees (verified by mapgen_test.go):
//   - Copper deposits sit only on `hills`, tin only on `mountain_limestone`,
//     i.e. on terrain that actually has a production rule (no dead deposits).
//   - Copper and tin live in disjoint land components — bronze is unreachable
//     without crossing the sea.
//   - At least 2 productive copper and 2 productive tin tiles, and ≥3 cedar
//     forests on the eastern landmass.
//   - Multiple distinct landmasses separated by sea (a real archipelago).
//
// The unit-test guarantees are now *enforced at generation time*: GenerateMap
// validates each candidate map and reseeds until one passes (rejection sampling),
// so a map that lacks a tin pole can never reach a live world. It returns the
// effective seed that produced the returned map — callers MUST persist it (it
// may differ from the requested seed when an early candidate was rejected).
func GenerateMap(worldID interface{ String() string }, seed int64, width, height int) ([]MapTile, int64) {
	for attempt := int64(0); attempt < maxMapAttempts; attempt++ {
		eff := seed + attempt
		tiles := generateMapOnce(worldID, eff, width, height)
		if err := validateMap(tiles); err != nil {
			slog.Warn("mapgen: invalid map, reseeding",
				"world", worldID.String(), "seed", eff, "width", width, "height", height, "err", err)
			continue
		}
		return tiles, eff
	}
	// A broken map must never host a world — fail loud rather than serve a
	// world whose MVP loop (cross-sea bronze trade) is structurally impossible.
	panic(fmt.Sprintf("mapgen: no valid map in %d attempts from seed %d (%dx%d)",
		maxMapAttempts, seed, width, height))
}

// maxMapAttempts bounds the rejection-sampling loop. Valid maps are common
// (seeds 0–19 already pass every invariant), so this ceiling is only a guard.
const maxMapAttempts = 100

// Minimum guarantees a generated map must satisfy before it can host a world.
// Mirror the thresholds asserted by TestGenerateMap_DepositsOnProductiveTerrain
// and the validation checklist in temenos_mapgen.md.
const (
	minProductiveCopper = 2
	minProductiveTin    = 2
	minCedar            = 2
	minLandmasses       = 4 // a real archipelago, not one merged blob
)

// validateMap returns a non-nil error naming every invariant the tile set
// violates. The tin check is the one the live 0620 world silently failed:
// 0 mountain_limestone tiles → 0 productive tin → no tin pole → dead MVP loop.
func validateMap(tiles []MapTile) error {
	copperProd, tinProd, cedar := 0, 0, 0
	comp := landComponents(tiles)
	copperComps := map[int]bool{}
	tinComps := map[int]bool{}
	landmasses := map[int]bool{}
	for _, t := range tiles {
		k := [2]int{t.Q, t.R}
		if tileIsLand(t.Terrain) {
			landmasses[comp[k]] = true
		}
		if t.CopperDeposit && t.Terrain == TerrainHills {
			copperProd++
			copperComps[comp[k]] = true
		}
		if t.TinDeposit && t.Terrain == TerrainMountainLimestone {
			tinProd++
			tinComps[comp[k]] = true
		}
		if t.CedarDeposit {
			cedar++
		}
	}

	var fails []string
	if copperProd < minProductiveCopper {
		fails = append(fails, fmt.Sprintf("productive copper = %d (want >= %d)", copperProd, minProductiveCopper))
	}
	if tinProd < minProductiveTin {
		fails = append(fails, fmt.Sprintf("productive tin = %d (want >= %d)", tinProd, minProductiveTin))
	}
	if cedar < minCedar {
		fails = append(fails, fmt.Sprintf("cedar = %d (want >= %d)", cedar, minCedar))
	}
	if len(landmasses) < minLandmasses {
		fails = append(fails, fmt.Sprintf("landmasses = %d (want >= %d)", len(landmasses), minLandmasses))
	}
	for c := range copperComps {
		if tinComps[c] {
			fails = append(fails, fmt.Sprintf("copper and tin share land component %d", c))
		}
	}
	if len(fails) > 0 {
		return fmt.Errorf("invalid map: %s", strings.Join(fails, "; "))
	}
	return nil
}

// generateMapOnce produces a single candidate map for a seed. It is wrapped by
// GenerateMap, which validates and reseeds. Deterministic per seed.
func generateMapOnce(worldID interface{ String() string }, seed int64, width, height int) []MapTile {
	rng := rand.New(rand.NewSource(seed))

	grid    := make(map[cell]Terrain)
	landmap := make(map[cell]int) // which landmass each cell belongs to
	bias    := map[int]int{}      // landmass ID → deposit bias

	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			grid[cell{q, r}]    = TerrainDeepSea
			landmap[cell{q, r}] = lmSea
		}
	}

	nextLM := 1
	place := func(qMin, qMax, rMin, rMax, radMin, radSpan, b int) int {
		if qMax <= qMin {
			qMax = qMin + 1
		}
		if rMax <= rMin {
			rMax = rMin + 1
		}
		// qMin..qMax are columns; rMin..rMax are screen-rows (height fractions).
		// Convert the row to axial r for the sheared rectangular domain.
		seedCol := qMin + rng.Intn(qMax - qMin)
		seedRow := rMin + rng.Intn(rMax - rMin)
		seedC := cell{seedCol, rowOrigin(seedCol, width) + seedRow}
		// Keep a 2-hex moat around existing land so landmasses stay distinct
		// (a real spread of islands, not one merged blob).
		for dq := -2; dq <= 2; dq++ {
			for dr := -2; dr <= 2; dr++ {
				n := cell{seedC.q + dq, seedC.r + dr}
				if hexDist(seedC, n) <= 2 && landmap[n] != lmSea {
					return 0
				}
			}
		}
		lm := nextLM
		nextLM++
		bias[lm] = b
		expandLandmass(grid, landmap, rng, seedC, width, height, radMin+rng.Intn(radSpan), lm)
		return lm
	}

	// ── 1. Mainland — western copper hills ────────────────────────────
	mainland := place(4, width*30/100, height/4, height*3/4, 6, 3, biasCopper)

	// ── 2. Anatolia — eastern tin mountains + cedar forests ───────────
	anatolia := place(width*72/100, width*92/100, height/4, height*3/4, 6, 3, biasTin)

	// ── 3. Crete — neutral hub, southern centre ───────────────────────
	place(width*40/100, width*58/100, height*60/100, height*85/100, 3, 3, biasNeutral)

	// ── 4. Cyclades — string of small neutral islands, scaled to area ─
	numCyclades := 3 + (width*height)/400
	for i := 0; i < numCyclades; i++ {
		place(width*36/100, width*64/100, height*15/100, height*85/100, 1, 2, biasNeutral)
	}

	// ── 5. Remote metal islands — isolated productive sources ─────────
	// Kept inside their hemisphere so the copper/tin separation is robust
	// even if a small island brushes a mainland.
	copperIsle := place(width*8/100, width*30/100, height*8/100, height*40/100, 1, 2, biasCopper)
	tinIsle    := place(width*70/100, width*92/100, height*55/100, height*88/100, 1, 2, biasTin)

	// ── 5b. Carve two permanent sea channels ──────────────────────────
	// A single all-sea hex column fully blocks horizontal hex-adjacency, so
	// the copper hemisphere (west of chanW), the neutral centre and the tin
	// hemisphere (east of chanE) become three sea-separated zones. We also
	// drown any copper/tin tendril that sprawled into the central strip, so
	// the centre carries neutral land only — bronze always demands sea trade.
	chanW := width * 33 / 100
	chanE := width * 67 / 100
	drown := func(c cell) {
		grid[c]    = TerrainDeepSea
		landmap[c] = lmSea
	}
	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			c := cell{q, r}
			switch {
			case q == chanW || q == chanE:
				drown(c)
			case q > chanW && q < chanE:
				if b := bias[landmap[c]]; b == biasCopper || b == biasTin {
					drown(c)
				}
			}
		}
	}

	// ── 6. Coastlines ─────────────────────────────────────────────────
	// Deep-sea tiles adjacent to land become coastal_sea (shallow water).
	// Land terrain is NOT changed — "coast" is a property (coastal flag), not a terrain type.
	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			c := cell{q, r}
			if grid[c] == TerrainDeepSea && hasLandNeighbour(grid, c, width, height) {
				grid[c] = TerrainCoastalSea
			}
		}
	}

	// ── 7. River valleys on the two big landmasses ────────────────────
	if mainland != 0 {
		addRiverValley(grid, landmap, rng, mainland, width, height)
	}
	if anatolia != 0 {
		addRiverValley(grid, landmap, rng, anatolia, width, height)
	}

	// ── 8. Build tiles + collect deposit candidates by bias & terrain ──
	tiles := make([]MapTile, 0, width*height)
	index := map[cell]int{}

	var (
		copperCand []int // hills on a copper-biased landmass
		tinCand    []int // mountain_limestone on a tin-biased landmass
		silverCand []int // any productive metal terrain, no copper/tin
		cedarCand  []int // eastern forest
	)

	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			c := cell{q, r}
			terrain := grid[c]
			lm := landmap[c]

			idx := len(tiles)
			index[c] = idx
			tiles = append(tiles, MapTile{
				Q: q, R: r,
				Terrain:   terrain,
				Coastal:   !isSea(terrain) && hasCoastalSeaNeighbour(grid, c, width, height),
				Fertility: 0.2 + rng.Float64()*0.8,
				Mineral:   0.1 + rng.Float64()*0.7,
			})

			switch terrain {
			case TerrainHills:
				if bias[lm] == biasCopper {
					copperCand = append(copperCand, idx)
				}
				silverCand = append(silverCand, idx)
			case TerrainMountainLimestone:
				if bias[lm] == biasTin {
					tinCand = append(tinCand, idx)
				}
				silverCand = append(silverCand, idx)
			case TerrainForestOliveGrove:
				if bias[lm] == biasTin {
					cedarCand = append(cedarCand, idx)
				}
			}
		}
	}

	// ── 9. Assign deposits ────────────────────────────────────────────
	for _, idx := range copperCand {
		if rng.Float64() < 0.35 {
			tiles[idx].CopperDeposit = true
		}
	}
	for _, idx := range tinCand {
		if rng.Float64() < 0.35 {
			tiles[idx].TinDeposit = true
		}
	}
	// Silver: rare, on metal terrain that didn't draw copper/tin.
	for _, idx := range silverCand {
		if !tiles[idx].CopperDeposit && !tiles[idx].TinDeposit && rng.Float64() < 0.08 {
			tiles[idx].SilverDeposit = true
		}
	}
	// Cedar: 3–5 eastern forests.
	rng.Shuffle(len(cedarCand), func(i, j int) { cedarCand[i], cedarCand[j] = cedarCand[j], cedarCand[i] })
	cedarTarget := 3 + rng.Intn(3)
	for i, idx := range cedarCand {
		if i >= cedarTarget {
			break
		}
		tiles[idx].CedarDeposit = true
	}

	// ── 10. Guarantee minimums (productive terrain only) ──────────────
	ensure := func(cand []int, count int, set func(*MapTile), has func(MapTile) bool) {
		have := 0
		for _, t := range tiles {
			if has(t) {
				have++
			}
		}
		for _, idx := range cand {
			if have >= count {
				return
			}
			if !has(tiles[idx]) {
				set(&tiles[idx])
				have++
			}
		}
	}
	ensure(copperCand, 2, func(t *MapTile) { t.CopperDeposit = true }, func(t MapTile) bool { return t.CopperDeposit })
	ensure(tinCand, 2, func(t *MapTile) { t.TinDeposit = true }, func(t MapTile) bool { return t.TinDeposit })

	// ── 11. Make the remote metal isles productive ────────────────────
	// Force one hills+copper / mountain+tin tile on each, converting terrain
	// if the small island didn't roll any — so a "remote copper/tin island"
	// is always a real source.
	forceMetal := func(lm, fallback int, terrain Terrain, set func(*MapTile)) {
		if lm == 0 {
			// Remote isle failed to place (moat collision) — force the metal on
			// its hemisphere's mainland instead, so the pole always exists.
			lm = fallback
		}
		if lm == 0 {
			return
		}
		var landTiles []int
		for c, l := range landmap {
			if l == lm && !isSea(grid[c]) {
				landTiles = append(landTiles, index[c])
			}
		}
		if len(landTiles) == 0 {
			return
		}
		// landmap iterates in random order — sort so tile selection (and thus the
		// generated map) stays deterministic for a given seed.
		sort.Ints(landTiles)
		// Prefer a tile already of the right terrain; else convert the first.
		target := -1
		for _, idx := range landTiles {
			if tiles[idx].Terrain == terrain {
				target = idx
				break
			}
		}
		if target == -1 {
			target = landTiles[0]
			// Converting terrain invalidates any deposit the tile already held
			// (e.g. a cedar forest becoming mountain) — clear before re-flagging.
			tiles[target].Terrain = terrain
			tiles[target].CedarDeposit = false
			tiles[target].SilverDeposit = false
		}
		set(&tiles[target])
	}
	forceMetal(copperIsle, mainland, TerrainHills, func(t *MapTile) { t.CopperDeposit = true })
	forceMetal(tinIsle, anatolia, TerrainMountainLimestone, func(t *MapTile) { t.TinDeposit = true })

	return tiles
}

// tileIsLand reports whether a terrain is land (not sea).
func tileIsLand(t Terrain) bool {
	return !isSea(t)
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

// expandLandmass flood-fills terrain from a seed cell, marking each cell with the given landmass ID.
func expandLandmass(grid map[cell]Terrain, landmap map[cell]int, rng *rand.Rand, seed cell, width, height, radius, lm int) {
	queue   := []cell{seed}
	visited := map[cell]bool{seed: true}

	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]

		dist := hexDist(c, seed)
		var terrain Terrain
		switch {
		case dist == 0:
			terrain = TerrainPlains
		case dist <= radius/4:
			terrain = TerrainPlains
		case dist <= radius/2:
			if rng.Float64() > 0.4 {
				terrain = TerrainHills
			} else {
				terrain = TerrainPlains
			}
		case dist <= radius*3/4:
			switch {
			case rng.Float64() < 0.35:
				terrain = TerrainMountainLimestone
			case rng.Float64() < 0.55:
				terrain = TerrainHills
			default:
				terrain = TerrainForestOliveGrove
			}
		default:
			if rng.Float64() < 0.5 {
				terrain = TerrainForestOliveGrove
			} else {
				terrain = TerrainHills
			}
		}

		grid[c]    = terrain
		landmap[c] = lm

		if dist >= radius {
			continue
		}
		for _, n := range hexNeighbours(c, width, height) {
			if !visited[n] && rng.Float64() > 0.25 {
				visited[n] = true
				queue = append(queue, n)
			}
		}
	}
}

// addRiverValley creates a short river-valley corridor from an inland tile toward the coast.
// Converts 3–6 plains or hills tiles in a line to river_valley terrain.
func addRiverValley(grid map[cell]Terrain, landmap map[cell]int, rng *rand.Rand, targetLM, width, height int) {
	// Find inland plains/hills tiles on the target landmass (not coastal)
	var candidates []cell
	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			c := cell{q, r}
			if landmap[c] == targetLM &&
				(grid[c] == TerrainPlains || grid[c] == TerrainHills) &&
				!hasDeepSeaNeighbour(grid, c, width, height) {
				candidates = append(candidates, c)
			}
		}
	}
	if len(candidates) == 0 {
		return
	}

	// Pick a random starting point
	start := candidates[rng.Intn(len(candidates))]
	length := 3 + rng.Intn(4) // 3–6 tiles

	// Walk roughly toward the coast (toward lower row or higher row, picking the nearest coast direction)
	dr := 1
	if start.r-rowOrigin(start.q, width) > height/2 {
		dr = -1
	}
	c := start
	for i := 0; i < length; i++ {
		if landmap[c] == targetLM {
			grid[c] = TerrainRiverValley
		}
		// Move toward coast
		next := cell{c.q + rng.Intn(3) - 1, c.r + dr}
		if !inMap(next.q, next.r, width, height) {
			break
		}
		if isSea(grid[next]) {
			break
		}
		c = next
	}
}

func hexDist(a, b cell) int {
	dq := a.q - b.q
	dr := a.r - b.r
	return (iAbs(dq) + iAbs(dq+dr) + iAbs(dr)) / 2
}

// rowOrigin is the per-column r-origin that turns the axial generation domain
// into a rectangle. The renderer positions a tile at y = √3·S·(r + q/2); laying
// each column's r over [rowOrigin(q), rowOrigin(q)+height) with
// rowOrigin(q) = (width-1)/2 − ⌊q/2⌋ cancels that +q/2 shear, so the world reads
// as an offset ("brick") rectangle instead of a sheared parallelogram — while all
// neighbour/distance math stays axial. (width-1)/2 keeps r ≥ 0 for every column.
// See temenos_mapgen_v4.md §A.
func rowOrigin(q, width int) int { return (width-1)/2 - q/2 }

// inMap reports whether axial (q,r) is inside the rectangular generation domain.
func inMap(q, r, width, height int) bool {
	if q < 0 || q >= width {
		return false
	}
	row := r - rowOrigin(q, width)
	return row >= 0 && row < height
}

func hexNeighbours(c cell, w, h int) []cell {
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
	var out []cell
	for _, d := range dirs {
		nq, nr := c.q+d[0], c.r+d[1]
		if inMap(nq, nr, w, h) {
			out = append(out, cell{nq, nr})
		}
	}
	return out
}

func isSea(t Terrain) bool {
	return t == TerrainDeepSea || t == TerrainCoastalSea
}

// hasDeepSeaNeighbour reports whether a land tile borders deep sea.
func hasDeepSeaNeighbour(grid map[cell]Terrain, c cell, w, h int) bool {
	return countDeepSeaNeighbours(grid, c, w, h) > 0
}

// countDeepSeaNeighbours returns how many of the 6 hex neighbours are deep sea.
func countDeepSeaNeighbours(grid map[cell]Terrain, c cell, w, h int) int {
	n := 0
	for _, nb := range hexNeighbours(c, w, h) {
		if grid[nb] == TerrainDeepSea {
			n++
		}
	}
	return n
}

// hasLandNeighbour reports whether a sea tile borders any land tile.
func hasLandNeighbour(grid map[cell]Terrain, c cell, w, h int) bool {
	for _, n := range hexNeighbours(c, w, h) {
		if !isSea(grid[n]) {
			return true
		}
	}
	return false
}

// hasCoastalSeaNeighbour reports whether a land tile borders any coastal_sea tile.
func hasCoastalSeaNeighbour(grid map[cell]Terrain, c cell, w, h int) bool {
	for _, n := range hexNeighbours(c, w, h) {
		if grid[n] == TerrainCoastalSea {
			return true
		}
	}
	return false
}

func iAbs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
