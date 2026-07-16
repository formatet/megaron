package world

import (
	"fmt"
	"log/slog"
	"math/rand"
	"sort"
	"strings"

	"github.com/aquilax/go-perlin"
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

// ── P1 height-field calibration numbers (bor i koden, itereras via PNG) ────
// temenos_mapgen_arkipelag_plan.md §P1.
const (
	// landFraction is the top-elevation share of the height field that
	// becomes land. A percentile threshold makes land share IDENTICAL on
	// every seed and every map size — fixes the old fixed-radius collapse
	// (0.22 → 0.07 → 0.03 across 56×40 / 120×84 / 230×230).
	landFraction = 0.25

	// lowFreqDivisor sets the low-frequency wavelength (width/lowFreqDivisor):
	// a handful of large Earthsea-scale landmasses per hemisphere.
	lowFreqDivisor = 3.0
	// highFreqWavelength is the high-frequency wavelength in hexes:
	// Cycladic/Ionian island-scatter grain, independent of map size.
	highFreqWavelength = 8.0

	// Blend weights (single source of truth — flip composition here only).
	// PRIMARY mode (Timothy's eyeball round 2026-07-16): a uniform
	// Earthsea-style blend — belt weights equal the hemisphere weights, so
	// low-frequency dominates everywhere and the scatter is even seasoning.
	// The losing alternative ("östra Medelhavet": a dense central scatter
	// belt) is the two-line change beltLow/HighWeight = 0.3/0.7 — keep the
	// mechanism, it may return once real play data exists.
	hemisphereLowWeight  = 0.7
	hemisphereHighWeight = 0.3
	beltLowWeight        = 0.7
	beltHighWeight       = 0.3

	// Channel-depression band (Timothy 2026-07-16: "kanalerna kanske kan
	// vara lite mer oregelbundna?"): the hard all-sea columns stay (they are
	// THE adjacency blocker for copper/tin separation), but the height field
	// is depressed in a band around each column whose half-width wobbles
	// noisily along the channel — so the channel reads as an irregular sea
	// corridor with ragged coasts instead of a ruler-straight canal.
	channelBandMin        = 2.0 // narrowest half-width of the depressed band, in columns
	channelBandMax        = 6.0 // widest half-width — also the early-out distance
	channelBandWavelength = 8.0 // hexes along r between band-width wobbles
	// Raw 1D Perlin amplitude is well under ±1, which left the half-width
	// pinned near its midpoint (≈ a straight coast 3 columns off the column,
	// the seam Timothy flagged). The gain stretches the wobble across the
	// full min..max range; clamping handles the overshoot.
	channelBandNoiseGain = 3.0
	// Depression at the column itself; fades linearly to 0 at the band edge.
	// The fBm blend spans roughly ±1.2, so 1.6 pushes near-column cells far
	// below any land percentile — land almost never touches the straight edge.
	channelDepressionDepth = 1.6

	// remoteIsleMaxTiles: a land component smaller than this (and not the
	// hemisphere's mainland/anatolia) is eligible as the "remote isle" that
	// forceMetal makes productive to force overseas trade.
	remoteIsleMaxTiles = 15

	// Height-percentile bands within the land range [cutoff, max]: below
	// lowBandMax → plains/scrub, below midBandMax → hills, at/above →
	// mountains. coastalFringeMax caps how high a coastal cell can be and
	// still get the forest/scrub fringe instead of its band terrain.
	lowBandMax       = 0.35
	midBandMax       = 0.7
	coastalFringeMax = 0.6
)

// GenerateMap procedurally generates a hex grid for a world using a seeded RNG.
//
// v4 (P1) — height-field archipelago. A two-scale fBm height field (see
// heightField) replaces the old fixed-radius blob placement: land is simply
// the top landFraction of the field by elevation, so every seed and every map
// size gets the same land share and organic (non-hexagonal) coastlines. The
// old six-region layout (Mainland/Anatolia/Crete/Cyclades/remote isles) is
// gone — per plan §Beslut 2026-07-16 #3 the region model is replaced by
// hemisphere guarantees derived from where each land COMPONENT ends up after
// the sea channels are carved:
//   - Entirely west of the western channel  → copper bias.
//   - Entirely east of the eastern channel  → tin bias.
//   - In the central belt between them      → neutral (the scatter/"Ionian" belt).
//
// The largest copper component stands in for "mainland", the largest tin
// component for "anatolia" (rivers + the remote-isle fallback target them);
// a small (<remoteIsleMaxTiles) component of the matching bias, if one
// exists, is forced productive as the remote overseas source.
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
// Minimum guarantees for WP3+ (river delta) and WP5 (mineral calibration).
const (
	minDeltaTiles       = 1 // ≥1 river_delta hex per map (WP3)
	minStraits          = 3 // ≥3 sea-strait hexes (narrow passages between landmasses)
	minTinCopperSeaDist = 8 // tenn↔koppar must require real sea crossing (WP5)
	// maxTinCopperSeaDist not enforced at generation time — on small maps the BFS
	// finds no path (MaxInt) since the channels block a direct route; the
	// rejection loop would exhaust 100 attempts. The placement guarantees copper and
	// tin are always in opposite hemispheres, so they ARE reachable via sea — the BFS
	// just can't prove it within the tile set boundary on small maps.
)

func validateMap(tiles []MapTile) error {
	copperProd, tinProd, cedar, deltaCount := 0, 0, 0, 0
	comp := landComponents(tiles)
	copperComps := map[int]bool{}
	tinComps := map[int]bool{}
	landmasses := map[int]bool{}

	// Build a fast lookup for the catchment check below.
	tileMap := make(map[[2]int]MapTile, len(tiles))
	maxQ := 0
	for _, t := range tiles {
		k := [2]int{t.Q, t.R}
		tileMap[k] = t
		if t.Q > maxQ {
			maxQ = t.Q
		}
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
		if t.Terrain == TerrainRiverDelta {
			deltaCount++
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
	if deltaCount < minDeltaTiles {
		fails = append(fails, fmt.Sprintf("river_delta tiles = %d (want >= %d)", deltaCount, minDeltaTiles))
	}
	for c := range copperComps {
		if tinComps[c] {
			fails = append(fails, fmt.Sprintf("copper and tin share land component %d", c))
		}
	}

	// WP5: tin↔copper minimum sea distance ≥ 8 hexes (ensures real crossing, not trivial adjacency)
	dist := tinCopperSeaDistance(tiles)
	if dist < minTinCopperSeaDist {
		fails = append(fails, fmt.Sprintf("tin↔copper sea distance = %d (want >= %d)", dist, minTinCopperSeaDist))
	}

	// WP5: ≥3 strait hexes (narrow sea passages between landmasses)
	straits := countStraits(tiles)
	if straits < minStraits {
		fails = append(fails, fmt.Sprintf("strait hexes = %d (want >= %d)", straits, minStraits))
	}

	// Fas 1a (handelskedjan): guarantee that at least one start-eligible tile in each
	// hemisphere has its malm within catchment — so the first wanax to settle there
	// produces ore from turn 1 without needing an oracle or extra colonisation.
	//
	// "Buildable" mirrors the terrain exclusion list in join.go capital placement:
	//   NOT IN (coastal_sea, deep_sea, mountain_limestone, mountain_red, semi_desert)
	// "Catchment" = the 6 axial neighbours RecomputeProduction reads (same as production logic).
	// "West" = q <= maxQ/2; "East" = q > maxQ/2 (east hemisphere, where tin is placed).
	//
	// A tile with a deposit that has ≥1 buildable neighbour is sufficient: that neighbour is
	// a valid colony site and the deposit tile is in its 6-hex catchment.
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

	westCopperCatchment := false // ≥1 buildable west tile whose catchment has copper
	eastTinCatchment := false    // ≥1 buildable east tile whose catchment has tin

	for _, t := range tiles {
		if !isBuildable(t) {
			continue
		}
		var hasCopperNeighbour, hasTinNeighbour bool
		for _, d := range dirs6 {
			nb, ok := tileMap[[2]int{t.Q + d[0], t.R + d[1]}]
			if !ok {
				continue
			}
			if nb.CopperDeposit {
				hasCopperNeighbour = true
			}
			if nb.TinDeposit {
				hasTinNeighbour = true
			}
		}
		if hasCopperNeighbour && t.Q <= halfQ {
			westCopperCatchment = true
		}
		if hasTinNeighbour && t.Q > halfQ {
			eastTinCatchment = true
		}
		if westCopperCatchment && eastTinCatchment {
			break
		}
	}

	if !westCopperCatchment {
		fails = append(fails, "no buildable west tile (q <= maxQ/2) has copper in its 6-hex catchment")
	}
	if !eastTinCatchment {
		fails = append(fails, "no buildable east tile (q > maxQ/2) has tin in its 6-hex catchment")
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

	chanW, chanE := seaChannels(width)

	// ── 1. Height field + percentile land threshold ────────────────────
	field := heightField(rng, width, height)
	cutoff, maxHeight := landCutoff(field, landFraction)
	landSet := make(map[cell]bool, width*height)
	for c, v := range field {
		if v >= cutoff {
			landSet[c] = true
		}
	}

	// ── 2. Carve the two permanent sea channels ─────────────────────────
	// A single all-sea column fully blocks horizontal hex-adjacency, so land
	// can never span a channel — every component ends up entirely west of
	// chanW, entirely east of chanE, or entirely in the central belt. That
	// makes the old per-blob "drown any tendril that sprawled into the
	// centre" rule redundant: bias is read off each component's side AFTER
	// carving (step 3), so there is nothing left to drown.
	for q := 0; q < width; q++ {
		if q != chanW && q != chanE {
			continue
		}
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			delete(landSet, cell{q, r})
		}
	}

	// ── 3. Land components + position-derived bias ──────────────────────
	// Build placeholder tiles (real terrain isn't decided until step 4) just
	// so landComponents — the same connectivity rule validateMap uses — can
	// group land into components.
	placeholder := make([]MapTile, 0, width*height)
	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			c := cell{q, r}
			terrain := TerrainDeepSea
			if landSet[c] {
				terrain = TerrainPlains
			}
			placeholder = append(placeholder, MapTile{Q: q, R: r, Terrain: terrain})
		}
	}
	rawComp := landComponents(placeholder)

	// landmap/compBias/compSize use the file's existing ID space: 0 is always
	// sea (lmSea); real components start at 1 (landComponents itself starts
	// IDs at 0, so we offset by one to keep that convention intact — forceMetal
	// below relies on 0 meaning "no component").
	landmap := make(map[cell]int, width*height)
	compSize := map[int]int{}
	compBias := map[int]int{}
	for _, t := range placeholder {
		c := cell{t.Q, t.R}
		if !tileIsLand(t.Terrain) {
			landmap[c] = lmSea
			continue
		}
		id := rawComp[[2]int{t.Q, t.R}] + 1
		landmap[c] = id
		compSize[id]++
		if _, seen := compBias[id]; !seen {
			switch {
			case t.Q < chanW:
				compBias[id] = biasCopper
			case t.Q > chanE:
				compBias[id] = biasTin
			default:
				compBias[id] = biasNeutral
			}
		}
	}

	// Deterministic id order (Go map range order is not) for the "largest
	// component" and "small isle" picks below.
	ids := make([]int, 0, len(compSize))
	for id := range compSize {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	// Largest copper component = "mainland", largest tin component =
	// "anatolia" — they stand in for the old named landmasses as the
	// river/remote-isle-fallback targets.
	mainland, anatolia := 0, 0
	maxCopper, maxTin := 0, 0
	for _, id := range ids {
		switch compBias[id] {
		case biasCopper:
			if compSize[id] > maxCopper {
				maxCopper, mainland = compSize[id], id
			}
		case biasTin:
			if compSize[id] > maxTin {
				maxTin, anatolia = compSize[id], id
			}
		}
	}
	// A small (<remoteIsleMaxTiles) component of the matching bias becomes
	// the "remote isle" forceMetal makes productive below (step 10).
	copperIsle, tinIsle := 0, 0
	for _, id := range ids {
		if id == mainland || id == anatolia || compSize[id] >= remoteIsleMaxTiles {
			continue
		}
		switch compBias[id] {
		case biasCopper:
			if copperIsle == 0 {
				copperIsle = id
			}
		case biasTin:
			if tinIsle == 0 {
				tinIsle = id
			}
		}
	}

	// ── 4. Provisional terrain: height band × hemisphere bias ───────────
	// Real terrain semantics (height × moisture lookup) land in P2 — this is
	// only enough texture for every terrain the deposit steps below need to
	// actually occur: hills (copper), mountain_limestone (tin, silver),
	// forest_olive_grove on tin-biased land (cedar).
	grid := make(map[cell]Terrain, width*height)
	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			c := cell{q, r}
			if !landSet[c] {
				grid[c] = TerrainDeepSea
				continue
			}
			norm := 0.0
			if maxHeight > cutoff {
				norm = (field[c] - cutoff) / (maxHeight - cutoff)
			}
			coastal := false
			for _, n := range hexNeighbours(c, width, height) {
				if !landSet[n] {
					coastal = true
					break
				}
			}
			grid[c] = provisionalTerrain(norm, compBias[landmap[c]], coastal, rng)
		}
	}

	// ── 5. Coastlines ─────────────────────────────────────────────────
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

	// ── 6. Rivers + deltas on the two hemisphere-largest landmasses ────
	// Each gets 1–2 rivers flowing from inland toward the coast. The river
	// mouth becomes a river_delta tile (highest grain, coastal).
	if mainland != 0 {
		addRiver(grid, landmap, rng, mainland, width, height)
	}
	if anatolia != 0 {
		addRiver(grid, landmap, rng, anatolia, width, height)
	}

	// ── 7. Build tiles + collect deposit candidates by bias & terrain ──
	tiles := make([]MapTile, 0, width*height)
	index := map[cell]int{}

	var (
		copperCand []int // hills on a copper-biased landmass
		tinCand    []int // mountain_limestone on a tin-biased landmass
		silverCand []int // any productive metal terrain, no copper/tin
		cedarCand  []int // forest_olive_grove on a tin-biased landmass
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
				if compBias[lm] == biasCopper {
					copperCand = append(copperCand, idx)
				}
				silverCand = append(silverCand, idx)
			case TerrainMountainLimestone:
				if compBias[lm] == biasTin {
					tinCand = append(tinCand, idx)
				}
				silverCand = append(silverCand, idx)
			case TerrainForestOliveGrove:
				if compBias[lm] == biasTin {
					cedarCand = append(cedarCand, idx)
				}
			}
		}
	}

	// ── 8. Assign deposits ────────────────────────────────────────────
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

	// ── 9. Guarantee minimums (productive terrain only) ────────────────
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

	// ── 10. Make the remote metal isles productive ──────────────────────
	// Force one hills+copper / mountain+tin tile on each, converting terrain
	// if the small island didn't roll any — so a "remote copper/tin island"
	// is always a real source.
	forceMetal := func(lm, fallback int, terrain Terrain, set func(*MapTile)) {
		if lm == 0 {
			// No small isle of the right bias exists — force the metal on the
			// hemisphere's mainland instead, so the pole always exists.
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

// seaChannels returns the two permanent sea-channel columns (33 %/67 % of
// width) that split the map into copper (west), neutral (centre) and tin
// (east) zones. Single source of truth for both heightField's belt weighting
// and generateMapOnce's channel carving — they must never drift apart.
func seaChannels(width int) (chanW, chanE int) {
	return width * 33 / 100, width * 67 / 100
}

// heightField computes a per-cell elevation via two-scale fractional Brownian
// motion (fBm): a low-frequency field (wavelength ≈ width/lowFreqDivisor)
// that produces a handful of large Earthsea-scale landmasses, and a
// high-frequency field (wavelength ≈ highFreqWavelength hexes) that adds
// Cycladic island scatter as even seasoning (plan §P1 MÅLBILD-UTSEENDE).
// The blend weight is position-aware — hemispheres vs. the central belt
// between the sea channels — though in the primary mode both use the same
// weights (see the blend-weight consts for the alternative).
//
// Around each channel column the field is depressed in a band whose
// half-width wobbles noisily along r (channelDepressionAt), so the coast
// facing a channel is ragged instead of tracing the ruler-straight column.
//
// Domain: the same sheared-rectangle generation domain as the rest of the
// file (rowOrigin/inMap), but noise is sampled on axis-aligned (q, row)
// coordinates — row = r - rowOrigin(q, width) — so the field itself isn't
// sheared along with the hex grid.
func heightField(rng *rand.Rand, width, height int) map[cell]float64 {
	// Independent Perlin permutation tables, seeded from the map's own rng
	// so the whole field stays deterministic per seed.
	low := perlin.NewPerlin(2, 2, 3, rng.Int63())   // 3 octaves: a little fractal roughness at continent scale
	high := perlin.NewPerlin(2, 2, 2, rng.Int63())  // 2 octaves: cheap, high-frequency scatter
	bandW := perlin.NewPerlin(2, 2, 1, rng.Int63()) // 1D band-width wobble, western channel
	bandE := perlin.NewPerlin(2, 2, 1, rng.Int63()) // 1D band-width wobble, eastern channel

	chanW, chanE := seaChannels(width)
	lowWavelength := float64(width) / lowFreqDivisor

	field := make(map[cell]float64, width*height)
	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		wLow, wHigh := hemisphereLowWeight, hemisphereHighWeight
		if q > chanW && q < chanE {
			wLow, wHigh = beltLowWeight, beltHighWeight
		}
		for r := base; r < base+height; r++ {
			row := float64(r - base)
			lowVal := low.Noise2D(float64(q)/lowWavelength, row/lowWavelength)
			highVal := high.Noise2D(float64(q)/highFreqWavelength, row/highFreqWavelength)
			v := wLow*lowVal + wHigh*highVal
			v -= channelDepressionAt(q, row, chanW, bandW)
			v -= channelDepressionAt(q, row, chanE, bandE)
			field[cell{q, r}] = v
		}
	}
	return field
}

// channelDepressionAt returns how much the height field is lowered at column
// q by the sea channel at chanQ. The depression is channelDepressionDepth at
// the column itself and fades linearly to zero at a half-width that wobbles
// between channelBandMin and channelBandMax columns, driven by 1D noise
// along the channel — so the channel-facing coastline lands at a different
// distance on every row and never traces the straight column. The percentile
// land threshold is applied AFTER this, so total land share is untouched;
// the land simply migrates away from the channels.
func channelDepressionAt(q int, row float64, chanQ int, band *perlin.Perlin) float64 {
	dist := float64(iAbs(q - chanQ))
	if dist >= channelBandMax {
		return 0
	}
	// Amplify the (small-amplitude) 1D Perlin so the half-width actually
	// sweeps the full min..max range, then clamp before mapping onto it.
	n := channelBandNoiseGain * band.Noise1D(row/channelBandWavelength)
	if n > 1 {
		n = 1
	} else if n < -1 {
		n = -1
	}
	halfWidth := channelBandMin + (channelBandMax-channelBandMin)*(n+1)/2
	if dist >= halfWidth {
		return 0
	}
	return channelDepressionDepth * (1 - dist/halfWidth)
}

// landCutoff sorts every height-field value and returns the elevation at the
// landFraction percentile (cells at/above it become land) plus the field's
// maximum, so callers can normalise land elevation into [0,1]. Land share is
// therefore identical across every seed and map size — no more area-dependent
// collapse (baseline: 0.22 → 0.07 → 0.03 across 56×40/120×84/230×230).
func landCutoff(field map[cell]float64, fraction float64) (cutoff, maxHeight float64) {
	vals := make([]float64, 0, len(field))
	for _, v := range field {
		vals = append(vals, v)
	}
	sort.Float64s(vals)
	idx := int(float64(len(vals)) * (1 - fraction))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(vals) {
		idx = len(vals) - 1
	}
	return vals[idx], vals[len(vals)-1]
}

// provisionalTerrain is P1's placeholder terrain rule: height band ×
// hemisphere bias × a coastal fringe, with just enough rng mix to avoid flat
// bands. It exists to unblock P1 (the real height×moisture terrain lookup is
// P2, temenos_mapgen_arkipelag_plan.md §P2) while still guaranteeing every
// terrain the deposit steps need actually occurs: hills everywhere (copper
// on copper-biased land), mountain_limestone in the high band (tin-biased
// high band keeps a limestone minority so tin ore — which only sits on
// limestone — actually exists on the east), and forest_olive_grove on the
// coastal fringe (cedar candidates on tin-biased land).
func provisionalTerrain(heightNorm float64, bias int, coastal bool, rng *rand.Rand) Terrain {
	switch {
	case coastal && heightNorm < coastalFringeMax:
		// Coastal fringe: forest/scrub mix. Tin-biased coast leans forest —
		// that's where cedar deposits are allowed to land.
		forestChance := 0.45
		if bias == biasTin {
			forestChance = 0.55
		}
		if rng.Float64() < forestChance {
			return TerrainForestOliveGrove
		}
		return TerrainScrubMaquis
	case heightNorm < lowBandMax:
		// Low band: plains with a scrub fringe.
		if rng.Float64() < 0.15 {
			return TerrainScrubMaquis
		}
		return TerrainPlains
	case heightNorm < midBandMax:
		// Mid band: hills — the copper terrain. Tin-biased land gets an
		// occasional semi_desert patch instead (Anatolia texture).
		if bias == biasTin && rng.Float64() < 0.25 {
			return TerrainSemiDesert
		}
		return TerrainHills
	default:
		// High band: mountain_red dominates the tin hemisphere,
		// mountain_limestone elsewhere — with a limestone minority kept on
		// tin land so tin ore has somewhere to exist.
		if bias == biasTin {
			if rng.Float64() < 0.3 {
				return TerrainMountainLimestone
			}
			return TerrainMountainRed
		}
		return TerrainMountainLimestone
	}
}

// addRiver creates a connected river corridor from an inland tile toward the coast,
// converting land tiles to river_valley. Where the river meets the sea it places
// a river_delta (2–4 adjacent coastal tiles with the highest grain in the game).
// This replaces the old cosmetic addRiverValley with a real geographical feature.
func addRiver(grid map[cell]Terrain, landmap map[cell]int, rng *rand.Rand, targetLM, width, height int) {
	// Find inland plains/hills tiles on the target landmass (not adjacent to sea).
	var inland []cell
	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			c := cell{q, r}
			if landmap[c] == targetLM &&
				(grid[c] == TerrainPlains || grid[c] == TerrainHills) &&
				!hasDeepSeaNeighbour(grid, c, width, height) {
				inland = append(inland, c)
			}
		}
	}
	if len(inland) == 0 {
		return
	}

	// Walk from a random inland start toward the coast.
	// Prefer moving in a direction with more land neighbours (hugs the landmass).
	start := inland[rng.Intn(len(inland))]
	length := 5 + rng.Intn(5) // 5–9 tiles before we stop or hit coast

	// Choose a general direction toward the nearest coast quadrant.
	// dr = ±1 based on which half of the map the start is in.
	dr := 1
	row := start.r - rowOrigin(start.q, width)
	if row > height/2 {
		dr = -1
	}
	dq := 0
	if start.q < width/2 {
		dq = -1
	} else {
		dq = 1
	}

	c := start
	var riverCells []cell
	for i := 0; i < length; i++ {
		if landmap[c] != targetLM {
			break
		}
		grid[c] = TerrainRiverValley
		riverCells = append(riverCells, c)

		// Try to step toward coast; jitter slightly left/right to look organic.
		jq := dq + rng.Intn(3) - 1 // dq-1, dq, or dq+1
		if jq < -1 {
			jq = -1
		} else if jq > 1 {
			jq = 1
		}
		candidates := []cell{
			{c.q + jq, c.r + dr},
			{c.q, c.r + dr},
			{c.q + dq, c.r},
		}
		moved := false
		for _, n := range candidates {
			if !inMap(n.q, n.r, width, height) {
				continue
			}
			if isSea(grid[n]) {
				// River reached coast — place delta here and stop.
				placeDelta(grid, landmap, rng, n, targetLM, width, height)
				return
			}
			if landmap[n] == targetLM {
				c = n
				moved = true
				break
			}
		}
		if !moved {
			break
		}
	}

	// If we ran out of river tiles without hitting sea, place a delta at the last
	// river cell if it neighbours coastal sea.
	if len(riverCells) > 0 {
		last := riverCells[len(riverCells)-1]
		for _, n := range hexNeighbours(last, width, height) {
			if isSea(grid[n]) {
				placeDelta(grid, landmap, rng, n, targetLM, width, height)
				return
			}
		}
	}
}

// placeDelta converts coastal land tiles adjacent to a river mouth into river_delta terrain.
// Delta tiles are coastal, fertile, and strategically exposed — the geographic "honey trap".
// We look for land tiles on the targetLM that border any sea tile (coastal_sea counts).
func placeDelta(grid map[cell]Terrain, landmap map[cell]int, rng *rand.Rand, mouth cell, targetLM, width, height int) {
	deltaSize := 1 + rng.Intn(3) // 1–3 delta tiles
	placed := 0

	// Walk outward from the mouth: prefer land tiles that border sea.
	candidates := hexNeighbours(mouth, width, height)
	// Also include the mouth's own neighbours' neighbours for larger deltas.
	for _, n := range hexNeighbours(mouth, width, height) {
		for _, nn := range hexNeighbours(n, width, height) {
			candidates = append(candidates, nn)
		}
	}
	for _, c := range candidates {
		if placed >= deltaSize {
			break
		}
		if !inMap(c.q, c.r, width, height) {
			continue
		}
		t := grid[c]
		// Convert a land tile on our landmass that borders any sea tile.
		if !isSea(t) && landmap[c] == targetLM && hasAnySeaNeighbour(grid, c, width, height) {
			grid[c] = TerrainRiverDelta
			placed++
		}
	}
}

// hasAnySeaNeighbour reports whether a land tile borders any sea tile (deep or coastal).
func hasAnySeaNeighbour(grid map[cell]Terrain, c cell, w, h int) bool {
	for _, n := range hexNeighbours(c, w, h) {
		if isSea(grid[n]) {
			return true
		}
	}
	return false
}

// tinCopperSeaDistance returns the minimum sea-path distance between any tin-deposit
// tile and any copper-deposit tile, measured through sea tiles only. This ensures
// the cross-sea bronze trade route exists and is non-trivial.
// Returns a large sentinel if no sea path exists (shouldn't happen on a valid map).
func tinCopperSeaDistance(tiles []MapTile) int {
	// Build lookup maps.
	terrain := make(map[cell]Terrain, len(tiles))
	for _, t := range tiles {
		terrain[cell{t.Q, t.R}] = t.Terrain
	}
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}

	// Collect land tiles holding deposits.
	var tinTiles, copperTiles []cell
	for _, t := range tiles {
		if t.TinDeposit {
			tinTiles = append(tinTiles, cell{t.Q, t.R})
		}
		if t.CopperDeposit {
			copperTiles = append(copperTiles, cell{t.Q, t.R})
		}
	}
	if len(tinTiles) == 0 || len(copperTiles) == 0 {
		return 1<<31 - 1
	}

	copperSet := make(map[cell]bool, len(copperTiles))
	for _, c := range copperTiles {
		copperSet[c] = true
	}

	// Multi-source BFS from all tin tiles simultaneously (walking through sea OR land,
	// counting ALL hexes traversed). We measure land-to-land distance as the Wanax
	// must send a ship: start on tin land, cross sea, reach copper land.
	// Simpler: use hex distance in the tile graph (any tile reachable) capped at sea.
	// Actually the game measures sea crossing, so BFS only through sea + the endpoints.
	type item struct {
		c cell
		d int
	}
	visited := make(map[cell]bool)
	queue := make([]item, 0, len(tinTiles))
	for _, c := range tinTiles {
		if !visited[c] {
			visited[c] = true
			queue = append(queue, item{c, 0})
		}
	}

	best := 1<<31 - 1
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.d >= best {
			continue
		}
		for _, d := range dirs {
			n := cell{cur.c.q + d[0], cur.c.r + d[1]}
			if visited[n] {
				continue
			}
			t, ok := terrain[n]
			if !ok {
				continue // outside map
			}
			visited[n] = true
			nd := cur.d + 1
			if copperSet[n] {
				if nd < best {
					best = nd
				}
				continue
			}
			// Only traverse sea tiles (not land) in the BFS interior
			// (tin/copper tiles are the endpoints, sea is the path).
			if !isSea(t) {
				continue
			}
			queue = append(queue, item{n, nd})
		}
	}
	return best
}

// countStraits counts sea hexes that are flanked by land on at least one opposing
// axis pair. A strait hex is a narrow water passage — vital for controlling trade routes.
func countStraits(tiles []MapTile) int {
	terrain := make(map[cell]Terrain, len(tiles))
	for _, t := range tiles {
		terrain[cell{t.Q, t.R}] = t.Terrain
	}
	// Opposing axis pairs in axial hex coordinates.
	opposites := [][2][2]int{
		{{1, 0}, {-1, 0}},
		{{0, 1}, {0, -1}},
		{{1, -1}, {-1, 1}},
	}
	straits := 0
	for _, t := range tiles {
		if !isSea(t.Terrain) {
			continue
		}
		c := cell{t.Q, t.R}
		for _, pair := range opposites {
			a := cell{c.q + pair[0][0], c.r + pair[0][1]}
			b := cell{c.q + pair[1][0], c.r + pair[1][1]}
			at := terrain[a]
			bt := terrain[b]
			if tileIsLand(at) && tileIsLand(bt) {
				straits++
				break // count this tile once even if multiple axes qualify
			}
		}
	}
	return straits
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

// SpawnOreCatchmentScore returns 1 when a candidate spawn tile has the
// hemisphere's strategic ore in its 6-hex catchment, 0 otherwise. This
// mirrors the ORDER BY ore-bias CASE expression in join.go: west tiles
// (q <= halfQ) score 1 if a copper-deposit neighbour exists; east tiles
// (q > halfQ) score 1 if a tin-deposit neighbour exists. A score of 1
// sorts ahead of 0 so the first joiners prefer ore-catchment tiles.
//
// The function is deliberately side-effect-free and DB-free — it exists so
// the spawn-bias contract can be unit-tested without a real database.
func SpawnOreCatchmentScore(candidate MapTile, tileMap map[[2]int]MapTile, halfQ int) int {
	dirs6 := [6][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
	for _, d := range dirs6 {
		nb, ok := tileMap[[2]int{candidate.Q + d[0], candidate.R + d[1]}]
		if !ok {
			continue
		}
		if candidate.Q <= halfQ && nb.CopperDeposit {
			return 1
		}
		if candidate.Q > halfQ && nb.TinDeposit {
			return 1
		}
	}
	return 0
}
