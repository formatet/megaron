package world

import (
	"fmt"
	"log/slog"
	"math"
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
)

// ── P2 terrain-lookup calibration numbers (bor i koden, itereras via PNG) ──
// temenos_mapgen_arkipelag_plan.md §P2. terrainFor (below) is the single
// height×moisture→terrain table; these consts are its thresholds only — the
// SHAPE of the table (which zone maps to which terrain) is the invariant,
// not these numbers.
const (
	// moistureWavelength is the moisture fBm wavelength in hexes: regional
	// wet/dry streaks, bigger than the height field's Cycladic high-frequency
	// scatter (highFreqWavelength) but well inside a hemisphere — plan §P2's
	// 8–20 hex window.
	moistureWavelength = 14.0

	// Height-percentile bands within the land range [cutoff, max] that
	// terrainFor reads: below lowBandMax → low band (food land / scrub),
	// below midBandMax → mid band (the full moisture spread), at/above →
	// high band (bare rock).
	lowBandMax = 0.35
	midBandMax = 0.7

	// Moisture zones (0..1 after normalisation, hemisphere shift, and the
	// coastal bonus below) that terrainFor reads for the mid band's 4-way
	// spread: below moistureAridMax → arid, below moistureMid → dry, below
	// moistureLushMin → moist, at/above → wet. The low and high bands only
	// need the wet/dry line, so they split at moistureMid directly.
	moistureAridMax = 0.15
	moistureMid     = 0.3
	moistureLushMin = 0.65

	// hemisphereMoistureShift nudges the moisture reading before bucketing —
	// west (copper) land reads wetter, east (tin) land reads drier. This IS
	// the entire replacement for the old per-region terrain bias (plan §P2
	// invariant: the lookup itself never changes, only the fields feeding it).
	// Kept modest on purpose: the old provisional terrain deliberately kept a
	// wet MINORITY on the tin hemisphere (a limestone/forest minority amid
	// mountain_red majority) because tin ore only sits on mountain_limestone
	// and cedar only on forest_olive_grove — shift too far and the east loses
	// its own strategic terrain.
	hemisphereMoistureShift = 0.07

	// coastalMoistureBonus keeps the shoreline from reading as bare desert or
	// bare rock: land bordering the sea always reads moister, so a
	// forest/scrub/plains presence survives along every coastline instead of
	// needing a special-cased branch in terrainFor (plan §P2: "coastal fringe
	// keeps a forest/scrub presence").
	coastalMoistureBonus = 0.2
)

// ── P3 river calibration numbers (bor i koden, itereras via PNG) ───────────
// temenos_mapgen_arkipelag_plan.md §P3.
const (
	// riverDensityDivisor sets river count = max(minRivers, landTiles /
	// riverDensityDivisor). The plan's starting number (~1 per 150 land hexes)
	// produces ~88 rivers on 230×230 — that dilutes the delta honey-trap:
	// deltas are the HIGHEST-grain tile in the game, so delta inflation is
	// food inflation, the same scarcity logic already applied to tin. Landed
	// on 500 after eyeballing the PNG suite (temenos_mapgen_arkipelag_plan.md
	// §P3 explicitly names the 300–600 window): ~2 rivers at 56×40, ~5 at
	// 120×84, high-20s at 230×230 — visibly gradient-fed without turning the
	// coastline into a lattice of cyan. "Scarcity beats abundance" per plan.
	riverDensityDivisor = 500
	// minRivers is the map-wide floor regardless of land area — even a small
	// map gets at least two rivers (plan §P3).
	minRivers = 2

	// riverMinComponentTiles: a land component smaller than this never gets a
	// river source — rivers on specks read as noise, not geography (plan §P3
	// "high-elevation tiles ... preferring LARGE components"). Comfortably
	// above remoteIsleMaxTiles (15) so a forced-metal remote isle never
	// doubles as a river source too.
	riverMinComponentTiles = 25

	// riverSourceSpacing is the minimum hex distance between two river
	// sources — plan §P3 "no two sources adjacent or near-adjacent".
	riverSourceSpacing = 6
)

// ── P4 deposit-cluster + scaled-validation calibration numbers ─────────────
// temenos_mapgen_arkipelag_plan.md §P4. Replaces the old per-hex-% deposit
// roll (35 % copper/tin, 8 % silver on candidate terrain) — that made metal
// quantity a function of how much candidate terrain the height/moisture
// fields happened to roll, with wild variance (empirically: 120×84 seed 201
// rolled 48 tin hexes; 230×230 seed 303 rolled 6 in a single monopoly
// cluster). Deposits are now target-counted source CLUSTERS, sized off
// playersFor, so quantity tracks intended population instead of noise.
const (
	// playersAreaDivisor derives target players from map area: 529 = 23²,
	// chosen so it round-trips BOTH plan calibration anchors exactly —
	// 230×230 (52 900 = 230²) divides to exactly 100 (the plan's
	// "hundraspelarmål"), while 56×40 (2240/529 ≈ 4) floors to the 10-player
	// minimum below.
	playersAreaDivisor = 529.0
	// playersFloor is the driftvärld minimum (today's 56×40 world takes 10
	// players) — playersFor never returns less than this regardless of area.
	playersFloor = 10

	// Copper is the deliberately generous metal (plan §A "väst-hemisfären
	// ska vara kopparrik") — no source-cluster cap, so both its cluster size
	// and its source-count target are allowed to scale with players.
	copperClusterMin    = 2
	copperClusterMax    = 4
	copperSourceFloor   = 4  // plan §A literal: "max(4, players/6)"
	copperSourceDivisor = 6

	// Silver sits between copper and tin ("mellanting").
	silverClusterMin    = 1
	silverClusterMax    = 3
	silverSourceFloor   = 3 // plan §A literal: "max(3, players/10)"
	silverSourceDivisor = 10

	// Tin is the opposite of copper: capped, not scaled. tinSourceCap is a
	// DESIGN INVARIANT (Timothy 2026-07-16, plan §A) — tin must get SCARCER
	// relative to player count as the map grows ("scarcity ska bli MER
	// kännbar med 100 spelare, inte mindre"), so the source-cluster count
	// never exceeds 4 no matter how big the map or how many players it's
	// sized for.
	tinClusterMin    = 1
	tinClusterMax    = 3
	tinSourceFloor   = 2  // matches the pre-P4 minProductiveTin floor
	tinSourceDivisor = 25 // gentle ramp: floor holds until ~50 players, caps at 100
	tinSourceCap     = 4

	// landmassAreaDivisor / straitSqrtDivisor calibrate validateMap's scaled
	// floors (below) against the P1 Earthsea blend's empirically observed
	// landComponents/countStraits at the plan's three sizes — see
	// minLandmassesFor/minStraitsFor's doc comments for the anchor numbers.
	landmassAreaDivisor = 900
	straitSqrtDivisor   = 15.0

	// maxLargestComponentFraction is a DESIGN INVARIANT (Timothy 2026-07-16,
	// plan §C), not a tunable: without it the height-field noise melts into
	// one super-continent on large maps. Largest single land component may
	// never exceed 15 % of total map tiles.
	maxLargestComponentFraction = 0.15
)

// playersFor derives a world's target player count from its map area — the
// dimension every P4 scarcity/validation number below is scaled from (plan
// §P4-B). See playersAreaDivisor's comment for why 529 round-trips both
// calibration anchors exactly.
func playersFor(width, height int) int {
	players := int(math.Round(float64(width*height) / playersAreaDivisor))
	if players < playersFloor {
		players = playersFloor
	}
	return players
}

func copperSourceTarget(players int) int {
	v := players / copperSourceDivisor
	if v < copperSourceFloor {
		v = copperSourceFloor
	}
	return v
}

func silverSourceTarget(players int) int {
	v := players / silverSourceDivisor
	if v < silverSourceFloor {
		v = silverSourceFloor
	}
	return v
}

// tinSourceTarget is capped at tinSourceCap regardless of players — see the
// const block's DESIGN INVARIANT comment.
func tinSourceTarget(players int) int {
	v := players / tinSourceDivisor
	if v < tinSourceFloor {
		v = tinSourceFloor
	}
	if v > tinSourceCap {
		v = tinSourceCap
	}
	return v
}

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
// v5 (P2) — height×moisture terrain. A second fBm field (moistureField)
// replaces P1's provisional height-band terrain: terrainFor looks up BOTH
// fields to pick a terrain, so biomes cluster along the moisture streaks
// instead of forming concentric height rings. The old per-region terrain
// bias is gone too — hemisphereMoistureShift nudges the moisture reading
// instead, reusing the same west=copper/east=tin split as step 3 above. See
// terrainFor's doc comment for the invariant this replaces forever, not just
// for this Era: the height×moisture→terrain table is the player's stable
// visual language and never changes; only the fields feeding it vary by seed.
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
		if err := validateMap(tiles, width, height); err != nil {
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
//
// minProductiveCopper, minLandmasses and minStraits are FUNCTIONS below, not
// consts (P4, plan §C "skalad validateMap") — a 230×230 world should demand a
// bigger archipelago, more straits and more copper than a 56×40 one.
// minProductiveTin and minCedar stay flat consts on purpose: tin must get
// SCARCER relative to player count as the map grows (plan §P4-A), and cedar
// is already count-based independent of map size (plan §A, untouched by P4).
const (
	minProductiveTin = 2
	minCedar         = 2
)

// minProductiveCopperFor scales the copper floor with target players (plan
// §C "max(2, players/8) som startvärde") — copper has no source-cluster cap
// (§A "koppar generösare"), so its floor can genuinely grow with population.
// players/8 keeps today's 56×40/10-player floor at 2 and reaches 12 at the
// 100-player/230×230 ceiling.
func minProductiveCopperFor(players int) int {
	v := players / 8
	if v < 2 {
		v = 2
	}
	return v
}

// minLandmassesFor scales the archipelago-size floor with map area (plan §C
// "minLandmasses ∝ area, 56×40 → dagens 4"). landmassAreaDivisor is
// calibrated against the P1 Earthsea blend's actual observed landComponents
// count at each plan size (see the P4 verification notes), not derived from
// first principles.
func minLandmassesFor(width, height int) int {
	v := (width * height) / landmassAreaDivisor
	if v < 4 {
		v = 4
	}
	return v
}

// minStraitsFor scales the strait-count floor with √area, not area (plan
// §C) — straits are a coastline feature (linear), not an area-filling one,
// so a huge map shouldn't need proportionally as many more of them as it
// needs more land or metal. straitSqrtDivisor is calibrated so 56×40 lands
// exactly on today's floor of 3.
func minStraitsFor(width, height int) int {
	v := int(math.Round(math.Sqrt(float64(width*height)) / straitSqrtDivisor))
	if v < 3 {
		v = 3
	}
	return v
}

// validateMap returns a non-nil error naming every invariant the tile set
// violates. The tin check is the one the live 0620 world silently failed:
// 0 mountain_limestone tiles → 0 productive tin → no tin pole → dead MVP loop.
// Minimum guarantees for WP3+ (river delta) and WP5 (mineral calibration).
const (
	minDeltaTiles       = 1 // ≥1 river_delta hex per map (WP3)
	minTinCopperSeaDist = 8 // tenn↔koppar must require real sea crossing (WP5)
	// maxTinCopperSeaDist not enforced at generation time — on small maps the BFS
	// finds no path (MaxInt) since the channels block a direct route; the
	// rejection loop would exhaust 100 attempts. The placement guarantees copper and
	// tin are always in opposite hemispheres, so they ARE reachable via sea — the BFS
	// just can't prove it within the tile set boundary on small maps.
	// minStraits (P4: was a flat const) is now minStraitsFor(width, height) above.
)

// validateMap takes the map's width/height explicitly (P4, plan §C) rather
// than inferring area from len(tiles) — every scaled floor below needs
// width/height anyway (playersFor, minStraitsFor, minLandmassesFor), and the
// caller (GenerateMap) already has both to hand.
func validateMap(tiles []MapTile, width, height int) error {
	players := playersFor(width, height)
	minProductiveCopper := minProductiveCopperFor(players)
	minLandmasses := minLandmassesFor(width, height)
	minStraits := minStraitsFor(width, height)

	copperProd, tinProd, cedar, deltaCount := 0, 0, 0, 0
	comp := landComponents(tiles)
	copperComps := map[int]bool{}
	tinComps := map[int]bool{}
	landmassSize := map[int]int{}

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
			landmassSize[comp[k]]++
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
	if len(landmassSize) < minLandmasses {
		fails = append(fails, fmt.Sprintf("landmasses = %d (want >= %d)", len(landmassSize), minLandmasses))
	}
	largestLand := 0
	for _, sz := range landmassSize {
		if sz > largestLand {
			largestLand = sz
		}
	}
	largestFraction := 0.0
	if len(tiles) > 0 {
		largestFraction = float64(largestLand) / float64(len(tiles))
	}
	if largestFraction > maxLargestComponentFraction {
		fails = append(fails, fmt.Sprintf("largest landmass = %.1f%% of map area (want <= %.0f%%)",
			largestFraction*100, maxLargestComponentFraction*100))
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

	// ── 1b. Moisture field (P2) ──────────────────────────────────────────
	// Independent field, drawn from the SAME map rng one step later than the
	// height-field noise generators — still fully determined by the map seed,
	// no second seed parameter needed for determinism.
	moisture := moistureField(rng, width, height)
	moistureMin, moistureMax := moistureRange(moisture)

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

	// ── 4. Terrain: height × moisture lookup (P2) ────────────────────────
	// terrainFor is the game's stable visual language (plan §P2 invariant) —
	// every terrain the deposit steps below need actually occurs as a natural
	// consequence of the lookup: hills (copper) and forest_olive_grove
	// (cedar) in the mid moisture band, mountain_limestone (tin, silver) in
	// the high band's wet half.
	grid := make(map[cell]Terrain, width*height)
	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			c := cell{q, r}
			if !landSet[c] {
				grid[c] = TerrainDeepSea
				continue
			}
			heightNorm := 0.0
			if maxHeight > cutoff {
				heightNorm = (field[c] - cutoff) / (maxHeight - cutoff)
			}
			moistureNorm := 0.0
			if moistureMax > moistureMin {
				moistureNorm = (moisture[c] - moistureMin) / (moistureMax - moistureMin)
			}
			for _, n := range hexNeighbours(c, width, height) {
				if !landSet[n] {
					moistureNorm += coastalMoistureBonus
					break
				}
			}
			grid[c] = terrainFor(heightNorm, moistureNorm, compBias[landmap[c]])
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

	// ── 6. Gradientfloder (P3): height-driven rivers, steepest descent ─────
	// Sources are local height maxima on large land components (specks read
	// as noise, not geography), spaced apart so two rivers never start side
	// by side. Each river then walks downhill over the SAME height field
	// that decided land — ties broken by a fixed neighbour order, so a seed
	// always carves the same rivers — until it reaches the sea. An inland
	// pit (no strictly-lower neighbour) doesn't kill the river: it keeps
	// going via the next-best unvisited neighbour, a loop-guarded DFS that
	// always terminates inside the finite land component and — because every
	// land component borders sea somewhere — always finds it. This replaces
	// the old random walk (addRiver used to jitter toward a guessed
	// direction and could wander away from the coast and die inland — the
	// Amyklai-class silent failure documented in temenos_mapgen.md §Kända
	// begränsningar). See addRiver for the per-river delta guarantee this
	// construction makes possible.
	landArea := 0
	for _, sz := range compSize {
		landArea += sz
	}
	riverCount := landArea / riverDensityDivisor
	if riverCount < minRivers {
		riverCount = minRivers
	}
	for _, src := range riverSources(field, landmap, compSize, grid, riverCount, width, height) {
		addRiver(grid, landmap, field, rng, src, width, height)
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

	// ── 8. Assign deposits: target-counted source clusters (P4) ────────
	// Per-hex-% placement (the pre-P4 code, a single rng.Float64() < p roll
	// per candidate tile) made metal quantity a function of how much
	// candidate terrain the fields happened to roll — see the P4 const
	// block's doc comment for the empirical fallout. placeDepositClusters
	// instead grows a fixed NUMBER of small clusters (targets/sizes from the
	// P4 const block), so quantity tracks playersFor, not noise.
	players := playersFor(width, height)

	placeDepositClusters(tiles, copperCand, landmap, rng,
		copperSourceTarget(players), copperClusterMin, copperClusterMax, width, height,
		func(t *MapTile) { t.CopperDeposit = true })
	placeDepositClusters(tiles, tinCand, landmap, rng,
		tinSourceTarget(players), tinClusterMin, tinClusterMax, width, height,
		func(t *MapTile) { t.TinDeposit = true })

	// Silver candidates exclude anything copper/tin already claimed just
	// above — filtered here rather than in step 7, since copper/tin
	// placement has only just happened.
	var silverCandFree []int
	for _, idx := range silverCand {
		if !tiles[idx].CopperDeposit && !tiles[idx].TinDeposit {
			silverCandFree = append(silverCandFree, idx)
		}
	}
	placeDepositClusters(tiles, silverCandFree, landmap, rng,
		silverSourceTarget(players), silverClusterMin, silverClusterMax, width, height,
		func(t *MapTile) { t.SilverDeposit = true })

	// Cedar: 3–5 eastern forests — unchanged, already count-based (plan §A
	// "rör ej").
	rng.Shuffle(len(cedarCand), func(i, j int) { cedarCand[i], cedarCand[j] = cedarCand[j], cedarCand[i] })
	cedarTarget := 3 + rng.Intn(3)
	for i, idx := range cedarCand {
		if i >= cedarTarget {
			break
		}
		tiles[idx].CedarDeposit = true
	}

	// ── 9. Guarantee minimums (productive terrain only) ────────────────
	// Mechanism unchanged from pre-P4 (plan §A "steg 9 behålls oförändrat i
	// beteende") — only the target counts now come from the scaled floors
	// instead of a hardcoded 2, so this is a no-op on the common path where
	// step 8's clustering already cleared them (it clears them even in the
	// worst case of zero cluster growth, one tile per source — see the
	// const block's target/floor arithmetic).
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
	ensure(copperCand, minProductiveCopperFor(players), func(t *MapTile) { t.CopperDeposit = true }, func(t MapTile) bool { return t.CopperDeposit })
	ensure(tinCand, minProductiveTin, func(t *MapTile) { t.TinDeposit = true }, func(t MapTile) bool { return t.TinDeposit })

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
	// Tin only: skip forcing a remote isle productive when the tinSourceCap
	// (plan §A DESIGN INVARIANT, see the P4 const block) is already met —
	// forceMetal picks the isle's first land tile regardless of adjacency to
	// an existing tin cluster, so an unconditional call here could mint a
	// 5th distinct source component on some seeds. Copper has no such cap
	// (see forceMetal call above), so it stays unconditional.
	if depositSourceCount(tiles, func(t MapTile) bool { return t.TinDeposit }) < tinSourceCap {
		forceMetal(tinIsle, anatolia, TerrainMountainLimestone, func(t *MapTile) { t.TinDeposit = true })
	}

	return tiles
}

// growCluster grows one deposit source cluster from seed by BFS restricted
// to cells still marked available in avail — the same candidate-terrain
// class the seed came from (plan §P4-A: "väx varje frö till ett litet
// sammanhängande kluster ... via grannar i samma kandidatlista"). Growth
// stops at targetSize cells or when the local candidate patch runs out.
// riverNeighbourOrder's fixed direction order makes growth deterministic for
// a fixed avail set and seed — avail only ever shrinks as clusters are
// placed (callers never reorder it), so a seed always grows the same
// cluster for a given map.
func growCluster(seed cell, avail map[cell]bool, targetSize int) []cell {
	if !avail[seed] {
		return nil
	}
	cluster := []cell{seed}
	visited := map[cell]bool{seed: true}
	queue := []cell{seed}
	for len(cluster) < targetSize && len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, d := range riverNeighbourOrder {
			if len(cluster) >= targetSize {
				break
			}
			n := cell{cur.q + d[0], cur.r + d[1]}
			if visited[n] || !avail[n] {
				continue
			}
			visited[n] = true
			cluster = append(cluster, n)
			queue = append(queue, n)
		}
	}
	return cluster
}

// depositSources picks up to targetSources seed cells for growCluster, drawn
// round-robin across the land components touching cand. The plan's spread
// requirement (§P4-A: "när ≥2 källor finns ska minst 2 ligga på skilda
// landmassor") falls out of this for free: as long as ≥2 components have
// candidates, the first two seeds returned always come from two different
// ones, because every round visits every component (ascending component-id
// order — deterministic) before any component repeats. WHICH candidate is
// picked within a component is randomised once via the map's own rng
// (shuffled per component); map-range iteration order never leaks into the
// result, only rng draws do.
func depositSources(cand []int, tiles []MapTile, landmap map[cell]int, rng *rand.Rand, targetSources int) []cell {
	byComp := map[int][]int{}
	var compIDs []int
	for _, idx := range cand {
		lm := landmap[cell{tiles[idx].Q, tiles[idx].R}]
		if _, ok := byComp[lm]; !ok {
			compIDs = append(compIDs, lm)
		}
		byComp[lm] = append(byComp[lm], idx)
	}
	sort.Ints(compIDs)
	for _, lm := range compIDs {
		g := byComp[lm]
		rng.Shuffle(len(g), func(i, j int) { g[i], g[j] = g[j], g[i] })
	}

	pos := make(map[int]int, len(compIDs))
	var seeds []cell
	for len(seeds) < targetSources {
		progressed := false
		for _, lm := range compIDs {
			if len(seeds) >= targetSources {
				break
			}
			g := byComp[lm]
			p := pos[lm]
			if p >= len(g) {
				continue
			}
			idx := g[p]
			pos[lm] = p + 1
			seeds = append(seeds, cell{tiles[idx].Q, tiles[idx].R})
			progressed = true
		}
		if !progressed {
			break // every component's candidates are exhausted
		}
	}
	return seeds
}

// placeDepositClusters is step 8's shared engine for copper/tin/silver: pick
// up to targetSources seeds spread across land components (depositSources),
// grow each into a cluster of clusterMin..clusterMax cells (growCluster),
// and flip their deposit flag via set. A seed that collided with an earlier
// cluster's growth (avail[seed] already false) is silently skipped — the
// achieved source count can land under target on a crowded landmass, which
// is fine: GenerateMap's rejection-sampling loop (reseed until validateMap
// passes) is the backstop for "not enough", not a retry loop in here.
func placeDepositClusters(tiles []MapTile, cand []int, landmap map[cell]int, rng *rand.Rand, targetSources, clusterMin, clusterMax, width, height int, set func(*MapTile)) {
	if targetSources <= 0 || len(cand) == 0 {
		return
	}
	seeds := depositSources(cand, tiles, landmap, rng, targetSources)

	avail := make(map[cell]bool, len(cand))
	index := make(map[cell]int, len(cand))
	for _, idx := range cand {
		c := cell{tiles[idx].Q, tiles[idx].R}
		avail[c] = true
		index[c] = idx
	}

	for _, seed := range seeds {
		if !avail[seed] {
			continue
		}
		size := clusterMin
		if clusterMax > clusterMin {
			size += rng.Intn(clusterMax - clusterMin + 1)
		}
		cluster := growCluster(seed, avail, size)
		for _, c := range cluster {
			set(&tiles[index[c]])
			avail[c] = false
		}
	}
}

// depositSourceCount counts connected components among tiles for which has
// returns true — the "how many separate source clusters" reading both the
// tinSourceCap guard (generateMapOnce step 10) and the P4 JSON contract
// (copper_sources/tin_sources/silver_sources, debugpng.go) need. Adjacency
// is the same 6-axial rule as landComponents, but over the deposit flag
// instead of the land/sea terrain split. Iteration order over the resulting
// map is nondeterministic (Go map ranging), but the component COUNT it
// produces is invariant to traversal order, so that doesn't threaten
// mapgen's determinism contract — this is read-only accounting, not
// placement.
func depositSourceCount(tiles []MapTile, has func(MapTile) bool) int {
	present := map[[2]int]bool{}
	for _, t := range tiles {
		if has(t) {
			present[[2]int{t.Q, t.R}] = true
		}
	}
	dirs6 := [6][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
	seen := map[[2]int]bool{}
	count := 0
	for k := range present {
		if seen[k] {
			continue
		}
		count++
		queue := [][2]int{k}
		seen[k] = true
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, d := range dirs6 {
				n := [2]int{cur[0] + d[0], cur[1] + d[1]}
				if present[n] && !seen[n] {
					seen[n] = true
					queue = append(queue, n)
				}
			}
		}
	}
	return count
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

// moistureField computes a per-cell moisture value via single-scale fBm
// (see moistureWavelength). Independent Perlin permutation table, drawn from
// the same map rng as heightField's generators — one step later in the
// sequence, so still fully determined by the map seed with no second seed
// parameter. 3 octaves gives enough fractal texture to read as regional
// streaks rather than either flat gradients or high-frequency confetti.
//
// Domain matches heightField: axis-aligned (q, row) sampling, row = r -
// rowOrigin(q, width), so the field itself isn't sheared with the hex grid.
func moistureField(rng *rand.Rand, width, height int) map[cell]float64 {
	noise := perlin.NewPerlin(2, 2, 3, rng.Int63())

	field := make(map[cell]float64, width*height)
	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			row := float64(r - base)
			field[cell{q, r}] = noise.Noise2D(float64(q)/moistureWavelength, row/moistureWavelength)
		}
	}
	return field
}

// moistureRange returns a field's min and max value so callers can rescale
// it into [0,1]. Unlike landCutoff (a percentile — land share must be
// EXACT), moisture has no target split: the raw fBm shape already IS the
// regional wet/dry streak pattern, so a plain min-max rescale is enough.
func moistureRange(field map[cell]float64) (min, max float64) {
	first := true
	for _, v := range field {
		if first || v < min {
			min = v
		}
		if first || v > max {
			max = v
		}
		first = false
	}
	return min, max
}

// heightBand and moistureZone are terrainFor's two lookup axes.
type heightBand int
type moistureZone int

const (
	bandLow heightBand = iota
	bandMid
	bandHigh
)

const (
	zoneArid moistureZone = iota
	zoneDry
	zoneMoist
	zoneWet
)

func heightBandOf(heightNorm float64) heightBand {
	switch {
	case heightNorm < lowBandMax:
		return bandLow
	case heightNorm < midBandMax:
		return bandMid
	default:
		return bandHigh
	}
}

func moistureZoneOf(moistureNorm float64) moistureZone {
	switch {
	case moistureNorm < moistureAridMax:
		return zoneArid
	case moistureNorm < moistureMid:
		return zoneDry
	case moistureNorm < moistureLushMin:
		return zoneMoist
	default:
		return zoneWet
	}
}

// terrainTable is the plan's INVARIANT (temenos_mapgen_arkipelag_plan.md
// §P2): the player's stable visual language — wet+low = food land, dry+high
// = hard passage + mineral potential, wet+high = quarriable limestone. This
// table never changes between Eras; only the height/moisture FIELDS (and
// hence which cell lands where in it) vary per seed. The low/high bands only
// need the wet/dry line (zoneArid and zoneDry both read "dry"; zoneMoist and
// zoneWet both read "wet") — the mid band alone uses the full 4-way spread.
var terrainTable = map[heightBand]map[moistureZone]Terrain{
	bandHigh: {
		zoneArid: TerrainMountainRed, zoneDry: TerrainMountainRed,
		zoneMoist: TerrainMountainLimestone, zoneWet: TerrainMountainLimestone,
	},
	bandMid: {
		zoneArid: TerrainSemiDesert, zoneDry: TerrainScrubMaquis,
		zoneMoist: TerrainHills, zoneWet: TerrainForestOliveGrove,
	},
	bandLow: {
		zoneArid: TerrainScrubMaquis, zoneDry: TerrainScrubMaquis,
		zoneMoist: TerrainPlains, zoneWet: TerrainPlains,
	},
}

// terrainFor is the P2 height×moisture terrain lookup — see terrainTable's
// comment for the invariant it encodes. hemisphereBias shifts which side of
// the wet/dry line a cell falls on (west/copper reads wetter, east/tin reads
// drier via hemisphereMoistureShift) instead of the old per-region terrain
// bias; a neutral-bias cell (the central belt) reads the fields unshifted.
func terrainFor(heightNorm, moistureNorm float64, hemisphereBias int) Terrain {
	switch hemisphereBias {
	case biasCopper:
		moistureNorm += hemisphereMoistureShift
	case biasTin:
		moistureNorm -= hemisphereMoistureShift
	}
	return terrainTable[heightBandOf(heightNorm)][moistureZoneOf(moistureNorm)]
}

// riverNeighbourOrder is the fixed hex-neighbour direction order rivers use
// for deterministic tie-breaking (descentOrder, firstSeaNeighbour) — the same
// six directions as hexNeighbours' dirs, named here so the descent logic
// reads as an intentional contract rather than an anonymous literal borrowed
// from elsewhere in the file.
var riverNeighbourOrder = [6][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}

// riverSources picks up to n well-separated local-height-maxima land cells as
// river start points (plan §P3: "högsta lokala höjdpunkter på stora
// landkomponenter"). Candidates are restricted to components with at least
// riverMinComponentTiles tiles (specks read as noise, not geography), sorted
// by height descending, and accepted greedily as long as they clear
// riverSourceSpacing hexes from every already-chosen source — so two rivers
// never start side by side even on a broad plateau. Iteration is in the
// file's standard column-major (q, then r) order and ties are broken by
// (q, r) explicitly, so the result is fully deterministic for a given field.
func riverSources(field map[cell]float64, landmap map[cell]int, compSize map[int]int, grid map[cell]Terrain, n, width, height int) []cell {
	type candidate struct {
		c cell
		h float64
	}
	var candidates []candidate
	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			c := cell{q, r}
			lm := landmap[c]
			if lm == lmSea || compSize[lm] < riverMinComponentTiles || isSea(grid[c]) {
				continue
			}
			isMax := true
			for _, d := range riverNeighbourOrder {
				nb := cell{c.q + d[0], c.r + d[1]}
				if landmap[nb] == lm && field[nb] > field[c] {
					isMax = false
					break
				}
			}
			if isMax {
				candidates = append(candidates, candidate{c, field[c]})
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].h != candidates[j].h {
			return candidates[i].h > candidates[j].h
		}
		if candidates[i].c.q != candidates[j].c.q {
			return candidates[i].c.q < candidates[j].c.q
		}
		return candidates[i].c.r < candidates[j].c.r
	})

	var sources []cell
	for _, cd := range candidates {
		if len(sources) >= n {
			break
		}
		tooClose := false
		for _, s := range sources {
			if hexDist(cd.c, s) < riverSourceSpacing {
				tooClose = true
				break
			}
		}
		if !tooClose {
			sources = append(sources, cd.c)
		}
	}
	return sources
}

// descentOrder returns c's land neighbours on targetLM (sea and other
// landmasses excluded — a river only ever steps onto its own component or,
// via firstSeaNeighbour, straight into its mouth) sorted by height ascending:
// steepest descent first. Ties are broken by riverNeighbourOrder's fixed
// direction order (a stable sort over neighbours built in that order), never
// by map/iteration order, so a seed always carves the same river.
func descentOrder(field map[cell]float64, landmap map[cell]int, targetLM int, c cell, width, height int) []cell {
	var out []cell
	for _, d := range riverNeighbourOrder {
		n := cell{c.q + d[0], c.r + d[1]}
		if !inMap(n.q, n.r, width, height) || landmap[n] != targetLM {
			continue
		}
		out = append(out, n)
	}
	sort.SliceStable(out, func(i, j int) bool { return field[out[i]] < field[out[j]] })
	return out
}

// firstSeaNeighbour returns c's first sea hex neighbour in riverNeighbourOrder
// — deterministic mouth selection when a river's current cell borders more
// than one sea tile.
func firstSeaNeighbour(grid map[cell]Terrain, c cell, width, height int) (cell, bool) {
	for _, d := range riverNeighbourOrder {
		n := cell{c.q + d[0], c.r + d[1]}
		if inMap(n.q, n.r, width, height) && isSea(grid[n]) {
			return n, true
		}
	}
	return cell{}, false
}

// addRiver carves one river from source to the sea by steepest descent over
// the height field (plan §P3), then places its delta at the mouth. It is a
// loop-guarded depth-first walk, not a plain greedy walk: at each cell it
// tries its lowest unvisited neighbour first (steepest descent), but if that
// branch dead-ends (an inland pit whose every neighbour is already visited)
// it backtracks and tries the next-best neighbour of the previous cell —
// "filling the pit" by continuing via a higher neighbour instead of dying,
// exactly as plan §P3 asks, without needing a real priority-flood structure.
// Because no cell is ever visited twice, the walk is bounded by the land
// component's size and therefore always terminates; because every land
// component borders sea somewhere (it is carved as a connected patch inside
// a sea-majority field), that termination is always "reached the sea", never
// "ran out of map". A river whose source can't reach the sea within the
// safety cap (should be unreachable — see above) is simply not counted as
// generated: nothing is carved and no delta is asserted for it.
//
// Per-river delta guarantee (plan §P3, "not globalt utan per flod"): the
// invariant lives HERE in code, not in validateMap. validateMap only ever
// sees the flattened []MapTile list — it has no notion of "which delta
// belongs to which river" unless we thread river results through its
// signature, which every existing caller (including the test helpers that
// call validateMap(tiles) directly) would have to grow a parameter for, for
// a check this constructor already makes mathematically true: placeDelta is
// called with origin = the river's own last carved cell and ALWAYS tries
// origin first (see placeDelta's doc comment) — origin borders mouth (that's
// the definition of "mouth"), so it unconditionally passes placeDelta's
// eligibility filter and is placed before the delta-size budget can run out.
// The delta doesn't just exist "somewhere near the coast", it touches the
// carved river itself. Asserting it right after the call catches a real bug
// in the carving/placement logic loudly (same "fail loud" contract
// GenerateMap already uses for its own invariants), instead of letting a
// silently-broken river slip through as a passing map — this is exactly the
// gap a first draft of this function had: placeDelta's candidate list used
// to start from the mouth's hex neighbours in a fixed direction order with
// no preference for the actual river cell, so on a bending coastline the
// delta could land on unrelated nearby land while the true river dead-ended
// with no delta touching it — caught by the black-box connectivity test
// (TestGenerateMap_EveryRiverReachesDelta), not by eyeballing PNGs. validateMap
// keeps its existing minDeltaTiles>=1 floor as the map-level backstop (plan:
// "Keep minDeltaTiles ≥ 1 as the map-level floor") — untouched.
func addRiver(grid map[cell]Terrain, landmap map[cell]int, field map[cell]float64, rng *rand.Rand, source cell, width, height int) {
	targetLM := landmap[source]
	if targetLM == lmSea {
		return
	}

	visited := map[cell]bool{source: true}
	path := []cell{source}

	type frame struct {
		c         cell
		remaining []cell
	}
	stack := []frame{{c: source, remaining: descentOrder(field, landmap, targetLM, source, width, height)}}

	var mouth cell
	reached := false

	// Each cell is pushed at most once (visited-gated), so the loop runs at
	// most O(land component size) iterations. The cap is a pure safety net
	// against a future bookkeeping bug, not a limit real landmasses can hit.
	maxIter := width*height + 10
	for iter := 0; iter < maxIter && len(stack) > 0; iter++ {
		top := &stack[len(stack)-1]
		cur := top.c

		if n, ok := firstSeaNeighbour(grid, cur, width, height); ok {
			mouth = n
			reached = true
			break
		}

		var next cell
		found := false
		for len(top.remaining) > 0 {
			cand := top.remaining[0]
			top.remaining = top.remaining[1:]
			if !visited[cand] {
				next = cand
				found = true
				break
			}
		}
		if !found {
			// Every neighbour of cur is already visited and none was sea —
			// cur is a fully-explored dead branch. Backtrack: it is not part
			// of the final river.
			stack = stack[:len(stack)-1]
			path = path[:len(path)-1]
			continue
		}

		visited[next] = true
		path = append(path, next)
		stack = append(stack, frame{c: next, remaining: descentOrder(field, landmap, targetLM, next, width, height)})
	}

	if !reached {
		// Should be unreachable (see doc comment) — treat defensively as "no
		// river" rather than carve a corridor that never gets a delta.
		return
	}

	// Thin the carve to a line (round-2 fix): the DFS stack path is loop-free
	// but NOT blob-free — in a flat pit every step finds another unvisited
	// neighbour, so the walk serpentines across the whole pit floor without
	// ever dead-ending, and the entire tour stays on the "committed" path
	// (backtracking only trims true dead-ends). Carving that floods the pit
	// with river_valley — 15+-tile lake-like patches of the game's
	// extra-fertile terrain, i.e. a food-inflation hotspot (same scarcity
	// logic as deltas/tin). So the explored path is treated as a CORRIDOR,
	// not the river itself: riverLine below re-derives the shortest route
	// through the visited set from source to the mouth-adjacent cell, which
	// crosses a pit in a line instead of touring its floor. Only that line
	// is carved; every other explored tile keeps its terrainFor terrain.
	origin := path[len(path)-1]
	for _, c := range riverLine(visited, source, origin, width, height) {
		grid[c] = TerrainRiverValley
	}
	placeDelta(grid, landmap, rng, mouth, origin, targetLM, width, height)
	if grid[origin] != TerrainRiverDelta {
		panic(fmt.Sprintf("mapgen: river ending at %v (mouth %v) produced no delta tile at its own mouth — carving invariant broken", origin, mouth))
	}
}

// riverLine returns the shortest path from source to origin walking only
// cells in the descent's visited set — the thin line addRiver actually
// carves out of the explored corridor (see the round-2 comment there). Plain
// BFS with riverNeighbourOrder as the fixed expansion order, so the chosen
// line is deterministic per seed. origin is always reachable from source
// within visited (they are endpoints of the same connected DFS walk), so the
// fallback return is defensive only.
func riverLine(visited map[cell]bool, source, origin cell, width, height int) []cell {
	if source == origin {
		return []cell{origin}
	}
	parent := map[cell]cell{source: source}
	queue := []cell{source}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, d := range riverNeighbourOrder {
			n := cell{cur.q + d[0], cur.r + d[1]}
			if !inMap(n.q, n.r, width, height) || !visited[n] {
				continue
			}
			if _, seen := parent[n]; seen {
				continue
			}
			parent[n] = cur
			if n == origin {
				// Walk parents back to source, then reverse.
				line := []cell{n}
				for line[len(line)-1] != source {
					line = append(line, parent[line[len(line)-1]])
				}
				for i, j := 0, len(line)-1; i < j; i, j = i+1, j-1 {
					line[i], line[j] = line[j], line[i]
				}
				return line
			}
			queue = append(queue, n)
		}
	}
	// Unreachable by construction — carve at least the mouth cell so the
	// delta invariant still holds.
	return []cell{origin}
}

// placeDelta converts coastal land tiles adjacent to a river mouth into river_delta terrain.
// Delta tiles are coastal, fertile, and strategically exposed — the geographic "honey trap".
// We look for land tiles on the targetLM that border any sea tile (coastal_sea counts).
// origin is the river's own last carved land cell — the reason mouth counts
// as a mouth in the first place. It is ALWAYS tried first, ahead of the
// generic "land near mouth" candidates: on a bending coastline, mouth can
// have several land neighbours, and the fixed hexNeighbours direction order
// has no reason to reach origin before some unrelated tile that also happens
// to border the sea. Without this, a river's delta could land next to it
// geographically but not actually touch its carved river_valley chain — the
// exact per-river guarantee P3 exists to make airtight (see addRiver's doc
// comment). origin always passes the eligibility filter below (it borders
// mouth, which is sea, by construction) and deltaSize is always >= 1, so
// origin becoming river_delta is guaranteed, not probabilistic.
func placeDelta(grid map[cell]Terrain, landmap map[cell]int, rng *rand.Rand, mouth, origin cell, targetLM, width, height int) {
	deltaSize := 1 + rng.Intn(3) // 1–3 delta tiles
	placed := 0

	candidates := []cell{origin}
	// Walk outward from the mouth: prefer land tiles that border sea.
	candidates = append(candidates, hexNeighbours(mouth, width, height)...)
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
