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
	// and cedar-forest stands only seed from forest_olive_grove (S2,
	// megaron_cederskogen_plan.md) — shift too far and the east loses its own strategic terrain.
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

	// minLandFragment (megaron_floden_plan.md §7e, Timothy 2026-07-29): a river
	// deliberately WALLS its landmass in two — that is the point (land units
	// cannot cross it). But a carve that leaves a splinter this small or
	// smaller is a mapgen bug, not geography: it is undone (the whole carve —
	// line, flanks, delta — is reverted for that source) rather than kept.
	// 12 sits comfortably below riverMinComponentTiles (25, the floor for even
	// STARTING a river) so a river is never rejected by a fragment it created
	// out of ordinary, playable ground — only genuine slivers.
	minLandFragment = 12
)

// riverFlankable lists the terrains a river's flank may convert to
// river_valley (megaron_floden_plan.md §7b, Timothy 2026-07-29: "as a rule,
// on either side of the river"). Mountains, sea, delta, another river and any
// river_valley already placed are deliberately absent — the exception is that
// a mountain flank makes the river a ravine (no valley on that side), not a
// bug.
var riverFlankable = map[Terrain]bool{
	TerrainPlains:           true,
	TerrainScrubMaquis:      true,
	TerrainHills:            true,
	TerrainForestOliveGrove: true,
	TerrainSemiDesert:       true,
}

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
	copperSourceFloor   = 4 // plan §A literal: "max(4, players/6)"
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
//   - At least 2 productive copper and 2 productive tin tiles, and ≥2
//     contiguous cedar-forest stands (forest_cedar terrain) on the eastern
//     landmass (S2, megaron_cederskogen_plan.md).
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
//
// minCedar (S2, megaron_cederskogen_plan.md step 6): cedar is no longer
// scattered single hexes, it is CONTIGUOUS STANDS (cedarStandCountMin stands
// of cedarStandSizeMin+ hexes each, see the S2 const block below) — so the
// floor is raised from the old flat 2 to "at least two minimum-size stands"
// (2×3), keeping the same floor SHAPE (a raw hex count validateMap can check
// without knowing about stands) while matching the new generation contract.
const (
	minProductiveTin = 2
	minCedar         = cedarStandCountMin * cedarStandSizeMin
)

// ── S2 cederskogen calibration (bor i koden, itereras via cmd/mapgen-debug) ─
// megaron_cederskogen_plan.md step 4-5. Cedar forests are now CONTIGUOUS
// STANDS grown from the same seed candidates the old scattered-flag code used
// (forest_olive_grove on the tin-biased landmass) — a Wanax reads "here is a
// forest", not "here is a resource icon". Grove seeding is a separate,
// additive pass answering Timothy's "det finns en brist på skog" (2026-07-29)
// without touching terrainTable (the documented mapgen invariant).
const (
	// cedarStandCountMin/Spread: 2-3 seed hexes per map — the same count the
	// old scattered-cedar code rolled (cedarTarget := 3 + rng.Intn(3) was
	// 3-5; kept slightly lower here since each "hex" is now a multi-tile
	// stand, not a single deposit flag).
	cedarStandCountMin    = 2
	cedarStandCountSpread = 2 // rng.Intn(2) → 0 or 1, so count is 2 or 3

	// cedarStandSizeMin/Spread: each stand grows to 3-7 contiguous hexes
	// (plan §4), converting same-landmass forest_olive_grove/hills neighbours.
	cedarStandSizeMin    = 3
	cedarStandSizeSpread = 5 // rng.Intn(5) → 0..4, so size is 3..7

	// groveDensityDivisor sets small-olive-grove seed count = landArea /
	// groveDensityDivisor (same style as riverDensityDivisor). Calibrated
	// against cmd/mapgen-debug's forest-fraction measurement (A4 gate:
	// 15.7% → 22-28% of land) — see the process report for the histogram
	// this was tuned against.
	groveDensityDivisor = 12

	// groveMoistureMin: grove seeds only land in the moist half of the
	// moisture field ("fuktfältets våta halva", plan §5) — moistureNorm is
	// 0..1 after normalisation, so 0.5 is exactly that half.
	groveMoistureMin = 0.5

	// groveStandSizeMin/Spread: each grove is 2-4 hexes (plan §5).
	groveStandSizeMin    = 2
	groveStandSizeSpread = 3 // rng.Intn(3) → 0..2, so size is 2..4

	// forestFractionMin/Max: the A4 target band, enforced as a validateMap
	// invariant so a map can never host a world with a forest share outside
	// it (same rejection-sampling mechanism as every other floor here).
	// "Forest" = forest_olive_grove + forest_cedar, as a fraction of LAND
	// tiles (not all map tiles) — sea dilutes both sides equally otherwise.
	forestFractionMin = 0.22
	forestFractionMax = 0.30
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
	landTiles, forestTiles := 0, 0
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
			landTiles++
		}
		if t.Terrain == TerrainForestOliveGrove || t.Terrain == TerrainForestCedar {
			forestTiles++
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
	// S2 (megaron_cederskogen_plan.md step 5, A4 gate): forest share of LAND
	// (not map area — sea dilutes both sides of the fraction equally and
	// would just shrink the band's meaning as channel width changes).
	forestFraction := 0.0
	if landTiles > 0 {
		forestFraction = float64(forestTiles) / float64(landTiles)
	}
	if forestFraction < forestFractionMin || forestFraction > forestFractionMax {
		fails = append(fails, fmt.Sprintf("forest fraction = %.3f (want %.2f..%.2f)",
			forestFraction, forestFractionMin, forestFractionMax))
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
	//   NOT IN (coastal_sea, deep_sea, river, mountain_limestone, mountain_red, semi_desert)
	// "Catchment" = the 6 axial neighbours RecomputeProduction reads (same as production logic).
	// "West" = q <= maxQ/2; "East" = q > maxQ/2 (east hemisphere, where tin is placed).
	//
	// A tile with a deposit that has ≥1 buildable neighbour is sufficient: that neighbour is
	// a valid colony site and the deposit tile is in its 6-hex catchment.
	halfQ := maxQ / 2
	dirs6 := [6][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
	isBuildable := func(t MapTile) bool {
		switch t.Terrain {
		case TerrainCoastalSea, TerrainDeepSea, TerrainRiver,
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

	fails = append(fails, riverInvariantFailures(tiles, width, height)...)

	if len(fails) > 0 {
		return fmt.Errorf("invalid map: %s", strings.Join(fails, "; "))
	}
	return nil
}

// riverInvariantFailures asserts §7f's independent, black-box river checks —
// verified against the flattened []MapTile output, never trusting the carving
// code's own internal panics (same "fail loud, verify separately" contract as
// the rest of validateMap). Re-derives its own grid + mainSea rather than
// threading generation-time state through, exactly like every other check in
// this function.
func riverInvariantFailures(tiles []MapTile, width, height int) []string {
	grid := make(map[cell]Terrain, len(tiles))
	for _, t := range tiles {
		grid[cell{t.Q, t.R}] = t.Terrain
	}
	if len(grid) == 0 {
		return nil
	}

	var fails []string

	// Every river hex has at most 2 river neighbours (§7c: exactly 1 hex wide).
	for c, t := range grid {
		if t != TerrainRiver {
			continue
		}
		n := 0
		for _, d := range riverNeighbourOrder {
			if grid[cell{c.q + d[0], c.r + d[1]}] == TerrainRiver {
				n++
			}
		}
		if n > 2 {
			fails = append(fails, fmt.Sprintf("river hex (%d,%d) has %d river neighbours (want <= 2)", c.q, c.r, n))
		}
	}

	// Every connected river+delta group touches at least one river_delta tile
	// (§7d) AND at least one main-sea tile (§7a: the mouth is always the
	// Thalassa, never a landlocked lake).
	mainSea := mainSeaComponent(grid, width, height)
	seen := map[cell]bool{}
	for start, t := range grid {
		if t != TerrainRiver || seen[start] {
			continue
		}
		hasDelta, hasMainSea := false, false
		queue := []cell{start}
		seen[start] = true
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, n := range hexNeighbours(cur, width, height) {
				nt := grid[n]
				if nt == TerrainRiverDelta {
					hasDelta = true
				}
				if mainSea[n] {
					hasMainSea = true
				}
				if nt == TerrainRiver && !seen[n] {
					seen[n] = true
					queue = append(queue, n)
				}
			}
		}
		if !hasDelta {
			fails = append(fails, fmt.Sprintf("river component at (%d,%d) has no river_delta tile", start.q, start.r))
		}
		if !hasMainSea {
			fails = append(fails, fmt.Sprintf("river component at (%d,%d) never reaches the main sea", start.q, start.r))
		}
	}

	// Every river hex has ≥1 river_valley neighbour (§7b's promise to the
	// player), UNLESS every one of its non-water neighbours is mountain — that
	// is §7b's own named exception (a mountain flank makes the river a ravine,
	// not a bug), so it cannot also be a violation here.
	for c, t := range grid {
		if t != TerrainRiver {
			continue
		}
		hasValley, hasNonMountainLand := false, false
		for _, d := range riverNeighbourOrder {
			nt, ok := grid[cell{c.q + d[0], c.r + d[1]}]
			if !ok {
				continue
			}
			if nt == TerrainRiverValley {
				hasValley = true
			}
			if riverFlankable[nt] {
				hasNonMountainLand = true
			}
		}
		if !hasValley && hasNonMountainLand {
			fails = append(fails, fmt.Sprintf("river hex (%d,%d) has no river_valley neighbour", c.q, c.r))
		}
	}

	// No RIVER-ADJACENT walkable-land component is smaller than minLandFragment
	// (§7e) — this is the whole-map backstop for the same guard the generation
	// loop already applies per-carve. Scoped to components that touch a river
	// hex, not every walkable component map-wide: a legitimate remote isle
	// (forceMetal's copperIsle/tinIsle, deliberately as small as 1 tile and
	// never touched by any river) is not a river-carving defect and must not
	// trip this check.
	walkable := func(t Terrain) bool { return !isSea(t) && t != TerrainRiver }
	seenLand := map[cell]bool{}
	for start, t := range grid {
		if !walkable(t) || seenLand[start] {
			continue
		}
		size := 0
		touchesRiver := false
		queue := []cell{start}
		seenLand[start] = true
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			size++
			for _, d := range riverNeighbourOrder {
				n := cell{cur.q + d[0], cur.r + d[1]}
				nt, ok := grid[n]
				if !ok {
					continue
				}
				if nt == TerrainRiver {
					touchesRiver = true
				}
				if walkable(nt) && !seenLand[n] {
					seenLand[n] = true
					queue = append(queue, n)
				}
			}
		}
		if touchesRiver && size < minLandFragment {
			fails = append(fails, fmt.Sprintf("river-adjacent land fragment at (%d,%d) = %d tiles (want >= %d)",
				start.q, start.r, size, minLandFragment))
		}
	}

	return fails
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
	// mainSea (§7a): the sea component touching the map's edge — the only sea
	// a river's mouth may ever count as reaching. Computed once, before any
	// carving, from the coastline this step just finished — carving never
	// creates or removes sea tiles, so it stays valid across every addRiver
	// call below.
	mainSea := mainSeaComponent(grid, width, height)

	// landmassCells groups every cell by its landmap id, captured once before
	// any river touches the map — landmap itself is immutable from step 3
	// onward, so this doubles as the per-landmass membership §7e's fragment
	// check needs and the "before" snapshot §7e's revert needs (a river only
	// ever converts cells within its own landmap[source] component — see
	// addRiver/placeDelta's landmap[c]==targetLM guards).
	landmassCells := map[int][]cell{}
	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			c := cell{q, r}
			if lm := landmap[c]; lm != lmSea {
				landmassCells[lm] = append(landmassCells[lm], c)
			}
		}
	}

	for _, src := range riverSources(field, landmap, compSize, grid, riverCount, width, height) {
		targetLM := landmap[src]
		cells := landmassCells[targetLM]

		before := make(map[cell]Terrain, len(cells))
		for _, c := range cells {
			before[c] = grid[c]
		}

		addRiver(grid, landmap, field, mainSea, rng, src, width, height)
		thinRiverJunctions(grid, width, height)

		// §7e: a river deliberately walls its landmass in two — that's the
		// point — but a splinter smaller than minLandFragment is a mapgen bug,
		// not geography. Undo the WHOLE carve (line, flanks, delta) rather
		// than keep a fragment no Wanax could ever found a viable city on.
		if smallestFragment(grid, cells, width, height) < minLandFragment {
			for c, t := range before {
				grid[c] = t
			}
		}
	}

	// ── 6a. Small groves (S2 plan step 5): "det finns en brist på skog" ────
	// Additive pass, run AFTER rivers so a river never overwrites a grove it
	// didn't know about. terrainTable itself (the height×moisture invariant)
	// is untouched — this sprinkles small forest_olive_grove patches onto
	// plains hexes in the moist half of the moisture field, deterministically,
	// growing forest cover without touching the lookup at all. Plains (the
	// single biggest terrain, ~52% of land pre-S2) bears the cost.
	landAreaForGroves := 0
	for _, sz := range compSize {
		landAreaForGroves += sz
	}
	moistureNormAt := func(c cell) float64 {
		m := 0.0
		if moistureMax > moistureMin {
			m = (moisture[c] - moistureMin) / (moistureMax - moistureMin)
		}
		for _, n := range hexNeighbours(c, width, height) {
			if !landSet[n] {
				m += coastalMoistureBonus
				break
			}
		}
		return m
	}
	groveSeedTarget := landAreaForGroves / groveDensityDivisor
	var groveSeedCand []cell
	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			c := cell{q, r}
			if grid[c] == TerrainPlains && moistureNormAt(c) >= groveMoistureMin {
				groveSeedCand = append(groveSeedCand, c)
			}
		}
	}
	rng.Shuffle(len(groveSeedCand), func(i, j int) { groveSeedCand[i], groveSeedCand[j] = groveSeedCand[j], groveSeedCand[i] })
	growPatch := func(seed cell, target int, used map[cell]bool, sameTerrain Terrain, extraTerrain Terrain, sameLandmass bool) []cell {
		patch := []cell{seed}
		used[seed] = true
		for len(patch) < target {
			var frontier []cell
			for _, s := range patch {
				for _, n := range hexNeighbours(s, width, height) {
					if used[n] {
						continue
					}
					if sameLandmass && landmap[n] != landmap[seed] {
						continue
					}
					if grid[n] != sameTerrain && (extraTerrain == "" || grid[n] != extraTerrain) {
						continue
					}
					frontier = append(frontier, n)
				}
			}
			if len(frontier) == 0 {
				break // ran out of eligible neighbours — smaller patch than target
			}
			next := frontier[rng.Intn(len(frontier))]
			used[next] = true
			patch = append(patch, next)
		}
		return patch
	}
	groveUsed := map[cell]bool{}
	groveBuilt := 0
	for _, seed := range groveSeedCand {
		if groveBuilt >= groveSeedTarget {
			break
		}
		if groveUsed[seed] || grid[seed] != TerrainPlains {
			continue
		}
		target := groveStandSizeMin + rng.Intn(groveStandSizeSpread)
		for _, c := range growPatch(seed, target, groveUsed, TerrainPlains, "", false) {
			grid[c] = TerrainForestOliveGrove
		}
		groveBuilt++
	}

	// ── 6b. Cedar forest stands (S2 plan step 4) ────────────────────────
	// forest_cedar is now its own terrain, not a flag on forest_olive_grove:
	// 2-3 seed hexes (same tin-biased-forest bias the old scattered-flag code
	// used) each grow into a 3-7 hex CONTIGUOUS stand by converting
	// same-landmass forest_olive_grove/hills neighbours (including any grove
	// 6a just grew — harmless, it just means fewer, bigger patches). Run
	// AFTER 6a and rivers so cedar always gets first claim on the final
	// terrain, and BEFORE step 7's tile build so CedarDeposit can be derived
	// as a pure mirror of the terrain in exactly one place.
	var cedarSeedCand []cell
	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			c := cell{q, r}
			if grid[c] == TerrainForestOliveGrove && compBias[landmap[c]] == biasTin {
				cedarSeedCand = append(cedarSeedCand, c)
			}
		}
	}
	rng.Shuffle(len(cedarSeedCand), func(i, j int) { cedarSeedCand[i], cedarSeedCand[j] = cedarSeedCand[j], cedarSeedCand[i] })
	cedarUsed := map[cell]bool{}
	cedarStandCount := cedarStandCountMin + rng.Intn(cedarStandCountSpread)
	cedarBuilt := 0
	for _, seed := range cedarSeedCand {
		if cedarBuilt >= cedarStandCount {
			break
		}
		if cedarUsed[seed] {
			continue
		}
		target := cedarStandSizeMin + rng.Intn(cedarStandSizeSpread)
		patch := growPatch(seed, target, cedarUsed, TerrainForestOliveGrove, TerrainHills, true)
		// Ett frö som inte når minimistorleken FÖRKASTAS i stället för att bli
		// ett ensamt cederhex. Skälet är en äkta integrationsdefekt som varken
		// flod- eller cederslicen kunde se ensam: floderna carvas i steg 6 och
		// konverterar sina flanker (inklusive olivlund) till river_valley, så
		// ett cederfrö som hamnat intill en flod kan sakna mark att växa i.
		// Utan den här kontrollen blev beståndet 1 hex — och "cederskog" som
		// ett isolerat hex är varken en skog att rendera eller en fyndighet
		// att hålla. Fröna är redan shufflade, så nästa kandidat prövas.
		if len(patch) < cedarStandSizeMin {
			for _, c := range patch {
				delete(cedarUsed, c)
			}
			continue
		}
		for _, c := range patch {
			grid[c] = TerrainForestCedar
		}
		cedarBuilt++
	}

	// ── 7. Build tiles + collect deposit candidates by bias & terrain ──
	tiles := make([]MapTile, 0, width*height)
	index := map[cell]int{}

	var (
		copperCand []int // hills on a copper-biased landmass
		tinCand    []int // mountain_limestone on a tin-biased landmass
		silverCand []int // any productive metal terrain, no copper/tin
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
				Terrain: terrain,
				// CedarDeposit is a pure mirror of the terrain, derived in
				// exactly this one place (S2, megaron_cederskogen_plan.md
				// step 4) — no other assignment site may exist, so the flag
				// and the terrain can never drift apart.
				CedarDeposit: terrain == TerrainForestCedar,
				// Coastal betyder sedan S1 "granne till vatten", inte "granne
				// till hav" — en flodstad får full kuststatus (Timothy
				// 2026-07-29). Floden själv är vatten och kan aldrig vara
				// kust åt sig själv, därav det explicita undantaget.
				Coastal:   !isSea(terrain) && terrain != TerrainRiver && hasWaterNeighbour(grid, c, width, height),
				Fertility: 0.2 + rng.Float64()*0.8,
				Mineral:   0.1 + rng.Float64()*0.7,
			})

			switch terrain {
			case TerrainHills:
				// Hills is itself buildable — a colony founded directly on the
				// deposit tile already has it in catchment, so no reachability
				// filter is needed here (see hasBuildableNeighbour's doc comment).
				if compBias[lm] == biasCopper {
					copperCand = append(copperCand, idx)
				}
				silverCand = append(silverCand, idx)
			case TerrainMountainLimestone:
				// P1c: mountain_limestone is NOT itself buildable, so only tiles
				// with a colonisable neighbour are viable candidates — otherwise
				// the deposit can never land in any settlement's catchment.
				if hasBuildableNeighbour(grid, c, width, height) {
					if compBias[lm] == biasTin {
						tinCand = append(tinCand, idx)
					}
					silverCand = append(silverCand, idx)
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

	// Cedar is no longer assigned here — it is now the forest_cedar terrain
	// grown by step 6b above, and CedarDeposit was already set as a mirror of
	// that terrain in step 7's tile-build loop.

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
		// P1c: prefer a tile that ALSO has a buildable neighbour — forcing the
		// metal onto a tile with no colonisable neighbour (only relevant for
		// mountain_limestone terrain; hills is self-buildable, see
		// hasBuildableNeighbour's doc comment) would mint a phantom source no
		// settlement can ever mine, defeating the "always a real source"
		// guarantee this step exists for. Degrade through: right terrain +
		// buildable neighbour → right terrain alone → any tile with a
		// buildable neighbour (convert it) → any tile at all (convert it,
		// the pre-P1c fallback) — so a genuinely all-mountain remote isle
		// still gets SOME source rather than none.
		pick := func(want func(idx int) bool) int {
			for _, idx := range landTiles {
				if want(idx) {
					return idx
				}
			}
			return -1
		}
		buildable := func(idx int) bool {
			return hasBuildableNeighbour(grid, cell{tiles[idx].Q, tiles[idx].R}, width, height)
		}
		target := pick(func(idx int) bool { return tiles[idx].Terrain == terrain && buildable(idx) })
		if target == -1 {
			// Reachability beats terrain-match here: a buildable-neighbour tile
			// converted to the right terrain is mineable; a same-terrain tile
			// with no buildable neighbour never is (see doc comment above).
			target = pick(buildable)
		}
		if target == -1 {
			target = pick(func(idx int) bool { return tiles[idx].Terrain == terrain })
		}
		if target == -1 {
			target = landTiles[0]
		}
		if tiles[target].Terrain != terrain {
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

	// ── 11. Final reachability sweep (P1c) ───────────────────────────────
	// Belt-and-suspenders on top of step 7's candidate pre-filter and step
	// 10's buildable-preferring target selection: forceMetal's own terrain
	// conversions mutate `tiles` after `grid` (step 4) was captured, so a
	// forced tile can steal the one buildable neighbour an already-placed
	// tin/silver tile (from step 8) depended on — checking against the
	// stale `grid` snapshot misses that. Re-check against `tiles`' CURRENT
	// terrain (via `index`, built in step 7 and still valid — tiles are
	// mutated in place, never reordered) and clear any mountain_limestone
	// deposit that ended up with zero buildable neighbour. validateMap's
	// minProductiveTin/eastTinCatchment floors then naturally trigger a
	// reseed if too few genuinely-mineable tin tiles remain, instead of
	// silently serving a phantom deposit no settlement can ever catch.
	currentlyBuildable := func(c cell) bool {
		for _, n := range hexNeighbours(c, width, height) {
			if idx, ok := index[n]; ok && spawnBuildable(tiles[idx].Terrain) {
				return true
			}
		}
		return false
	}
	for i := range tiles {
		if tiles[i].Terrain != TerrainMountainLimestone {
			continue
		}
		if !tiles[i].TinDeposit && !tiles[i].SilverDeposit {
			continue
		}
		if !currentlyBuildable(cell{tiles[i].Q, tiles[i].R}) {
			tiles[i].TinDeposit = false
			tiles[i].SilverDeposit = false
		}
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

// adjacentToExistingRiver reports whether any of c's hex neighbours is
// already TerrainRiver. Used only during descent (§7c prevention, see
// addRiver's stepping loop) — at that point grid holds exactly the rivers
// ALREADY accepted from earlier sources, since the current walk's own path is
// written into grid only after the whole descent finishes.
func adjacentToExistingRiver(grid map[cell]Terrain, c cell, width, height int) bool {
	for _, d := range riverNeighbourOrder {
		if grid[cell{c.q + d[0], c.r + d[1]}] == TerrainRiver {
			return true
		}
	}
	return false
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

// firstSeaNeighbour returns c's first MAIN-SEA hex neighbour in
// riverNeighbourOrder — deterministic mouth selection when a river's current
// cell borders more than one sea tile. Only mainSea counts (megaron_floden_plan.md
// §7a, Timothy 2026-07-29: "mynningen är alltid Thalassa — aldrig en småsjö").
// Before mainSea existed, every sea tile shared the single lmSea id regardless
// of connectivity, so a landlocked lake enclosed by land was indistinguishable
// from the open sea and could pass as a valid mouth.
func firstSeaNeighbour(grid map[cell]Terrain, mainSea map[cell]bool, c cell, width, height int) (cell, bool) {
	for _, d := range riverNeighbourOrder {
		n := cell{c.q + d[0], c.r + d[1]}
		if inMap(n.q, n.r, width, height) && mainSea[n] {
			return n, true
		}
	}
	return cell{}, false
}

// mainSeaComponent flood-fills from every sea tile touching the generation
// domain's edge and returns the set of sea cells connected to it — "the
// Thalassa", as opposed to a landlocked lake that happens to be enclosed by
// land (megaron_floden_plan.md §7a). isSea(t) alone cannot make this
// distinction: it is purely a terrain-type test, blind to connectivity, and
// landmap collapses every sea tile onto the single lmSea id for the same
// reason. Both remain correct for their existing callers — this is a
// deliberately separate, narrower concept used only to gate valid river
// mouths.
func mainSeaComponent(grid map[cell]Terrain, width, height int) map[cell]bool {
	main := map[cell]bool{}
	var queue []cell
	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			c := cell{q, r}
			if !isSea(grid[c]) || main[c] {
				continue
			}
			onEdge := false
			for _, d := range riverNeighbourOrder {
				n := cell{c.q + d[0], c.r + d[1]}
				if !inMap(n.q, n.r, width, height) {
					onEdge = true
					break
				}
			}
			if onEdge {
				main[c] = true
				queue = append(queue, c)
			}
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, d := range riverNeighbourOrder {
			n := cell{cur.q + d[0], cur.r + d[1]}
			if inMap(n.q, n.r, width, height) && isSea(grid[n]) && !main[n] {
				main[n] = true
				queue = append(queue, n)
			}
		}
	}
	return main
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
func addRiver(grid map[cell]Terrain, landmap map[cell]int, field map[cell]float64, mainSea map[cell]bool, rng *rand.Rand, source cell, width, height int) {
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

		if n, ok := firstSeaNeighbour(grid, mainSea, cur, width, height); ok {
			mouth = n
			reached = true
			break
		}

		var next cell
		found := false
		for len(top.remaining) > 0 {
			cand := top.remaining[0]
			top.remaining = top.remaining[1:]
			// Refuse a candidate already touching an EARLIER river (§7c,
			// prevention rather than cure): grid only holds rivers already
			// accepted from prior sources at this point (the current walk's
			// own path isn't written into grid until after it finishes), so
			// this keeps a 1-hex buffer between different rivers and stops
			// most cross-river pinches from ever being carved — the
			// thinRiverJunctions pass afterward only has to catch the rarer
			// self-touch of a single river's own path.
			if !visited[cand] && !adjacentToExistingRiver(grid, cand, width, height) {
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
	line := riverLine(visited, source, origin, width, height)

	// The line itself is the water (megaron_floden_plan.md S1, Timothy
	// 2026-07-29: river is its own vatten-terräng, not fertile valley ground).
	for _, c := range line {
		grid[c] = TerrainRiver
	}
	// Flanks: every land neighbour of every line hex becomes river_valley, but
	// only when it is currently one of riverFlankable's whitelisted terrains
	// (§7b). Checked against grid[n] BEFORE any overwrite, so a neighbour that
	// is mountain, sea, delta, another river or an already-placed river_valley
	// is left exactly as it is — that is the "ravine" exception, not a bug.
	for _, c := range line {
		for _, n := range hexNeighbours(c, width, height) {
			if riverFlankable[grid[n]] {
				grid[n] = TerrainRiverValley
			}
		}
	}

	placeDelta(grid, landmap, mainSea, rng, mouth, origin, targetLM, width, height)

	// The per-river invariant (§7d): origin is now WATER itself (the river's
	// own last hex), so the assertion can no longer be "origin became the
	// delta" — it is "at least one of origin's neighbours did", i.e. the delta
	// touches the river's actual mouth instead of landing somewhere merely
	// near the coast.
	hasDelta := false
	for _, n := range hexNeighbours(origin, width, height) {
		if grid[n] == TerrainRiverDelta {
			hasDelta = true
			break
		}
	}
	if !hasDelta {
		panic(fmt.Sprintf("mapgen: river ending at %v (mouth %v) produced no delta tile bordering its own mouth hex — carving invariant broken", origin, mouth))
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

// placeDelta converts a land tile at the river's mouth into river_delta
// terrain. Delta tiles are coastal, fertile, and strategically exposed — the
// geographic "honey trap".
//
// megaron_floden_plan.md §7d, Timothy 2026-07-29: origin — the river's own
// last carved cell — is now WATER (addRiver line-carves it to TerrainRiver
// before calling here), so the old guarantee ("the delta IS origin") no
// longer parses. The new one: the delta is a LAND hex that borders BOTH
// origin (the river's last hex) and the main sea. On a hex grid, any two
// adjacent hexes (origin and mouth are adjacent by construction — that's the
// definition of "mouth") always share exactly two common neighbours, the
// "bowtie" corners of their shared edge. Those are tried first — normally at
// least one is land and, bordering mouth (which is itself in mainSea),
// automatically fronts the real coast rather than some inland lake. The wider
// rings around origin and mouth are the same defensive fallback the original
// code used for a bigger (1-3 tile) delta, now filtered by mainSea instead of
// any isSea() tile.
func placeDelta(grid map[cell]Terrain, landmap map[cell]int, mainSea map[cell]bool, rng *rand.Rand, mouth, origin cell, targetLM, width, height int) {
	deltaSize := 1 + rng.Intn(3) // 1–3 delta tiles
	placed := 0

	bordersMainSea := func(c cell) bool {
		for _, n := range hexNeighbours(c, width, height) {
			if mainSea[n] {
				return true
			}
		}
		return false
	}

	var candidates []cell
	// The two bowtie corners shared by origin and mouth — the natural delta site.
	originNb := hexNeighbours(origin, width, height)
	mouthNb := hexNeighbours(mouth, width, height)
	for _, n := range originNb {
		for _, m := range mouthNb {
			if n == m {
				candidates = append(candidates, n)
			}
		}
	}
	// Fallback rings for a bigger delta / a corner that didn't qualify:
	// origin's own neighbours (still touching the river), then mouth's
	// neighbours and their neighbours (still fronting the real coast, per the
	// bordersMainSea filter below).
	candidates = append(candidates, originNb...)
	candidates = append(candidates, mouthNb...)
	for _, n := range mouthNb {
		candidates = append(candidates, hexNeighbours(n, width, height)...)
	}

	for _, c := range candidates {
		if placed >= deltaSize {
			break
		}
		if !inMap(c.q, c.r, width, height) {
			continue
		}
		t := grid[c]
		// Land (not sea, not the river itself) on our landmass, fronting the
		// main sea, not already a delta tile from an earlier pass through this
		// candidate list.
		if !isSea(t) && t != TerrainRiver && t != TerrainRiverDelta &&
			landmap[c] == targetLM && bordersMainSea(c) {
			grid[c] = TerrainRiverDelta
			placed++
		}
	}
}

// riverComponentsAllTouchDelta reports whether every TerrainRiver cell in grid
// belongs to a connected river+delta group that contains at least one
// TerrainRiverDelta tile. Used by thinRiverJunctions as the guard rail: a
// demotion is only ever kept if the map still satisfies this afterward.
func riverComponentsAllTouchDelta(grid map[cell]Terrain, width, height int) bool {
	seen := map[cell]bool{}
	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			start := cell{q, r}
			if grid[start] != TerrainRiver || seen[start] {
				continue
			}
			hasDelta := false
			queue := []cell{start}
			seen[start] = true
			for len(queue) > 0 {
				cur := queue[0]
				queue = queue[1:]
				for _, n := range hexNeighbours(cur, width, height) {
					t := grid[n]
					if t == TerrainRiverDelta {
						hasDelta = true
					}
					if t == TerrainRiver && !seen[n] {
						seen[n] = true
						queue = append(queue, n)
					}
				}
			}
			if !hasDelta {
				return false
			}
		}
	}
	return true
}

// thinRiverJunctions enforces river width == 1 (megaron_floden_plan.md §7c,
// Timothy 2026-07-29: "exakt 1 hex bred"). riverLine is a BFS shortest path
// and therefore already loop-free, but not touch-free: two cells that are NOT
// consecutive in the path can still end up hex-adjacent (a diagonal pinch
// where the route passes close to itself, or two different rivers' lines
// coming near each other), which would locally read as two hexes wide.
//
// A first version demoted the last neighbour found, unconditionally — but a
// "pinch" cell's extra neighbour is not always redundant: it can be a cell
// that is ITSELF load-bearing for a different stretch of the same (or a
// different) river's connectivity to its own delta. Demoting it unconditionally
// silently split a river in half, one half left with no delta — the exact
// Amyklai-class failure P3 exists to prevent (caught by
// TestGenerateMap_EveryRiverReachesDelta, seed 1 @ 120×84). So every candidate
// demotion is now tentative: apply it, and keep it only if
// riverComponentsAllTouchDelta still holds for the WHOLE map afterward;
// otherwise undo it and try the next excess neighbour. If none of a cell's
// excess neighbours can be safely demoted, that pinch is left as-is — a rare,
// locally 2-wide junction is a far smaller defect than a river cut off from
// its own mouth, and validateMap's width assertion (§7f) will surface it
// loudly rather than let it hide.
func thinRiverJunctions(grid map[cell]Terrain, width, height int) {
	unresolved := map[cell]bool{}
	for changed := true; changed; {
		changed = false
		for q := 0; q < width; q++ {
			base := rowOrigin(q, width)
			for r := base; r < base+height; r++ {
				c := cell{q, r}
				if grid[c] != TerrainRiver || unresolved[c] {
					continue
				}
				var riverNbrs []cell
				for _, d := range riverNeighbourOrder {
					n := cell{c.q + d[0], c.r + d[1]}
					if inMap(n.q, n.r, width, height) && grid[n] == TerrainRiver {
						riverNbrs = append(riverNbrs, n)
					}
				}
				if len(riverNbrs) <= 2 {
					continue
				}
				// Try demoting each neighbour in turn — not just the "excess"
				// tail — since which of them is truly redundant depends on the
				// wider network, not on riverNeighbourOrder's arbitrary scan
				// order.
				fixed := false
				for i := len(riverNbrs) - 1; i >= 0; i-- {
					cand := riverNbrs[i]
					grid[cand] = TerrainRiverValley
					if riverComponentsAllTouchDelta(grid, width, height) {
						fixed = true
						changed = true
						break
					}
					grid[cand] = TerrainRiver // undo, try the next candidate
				}
				if !fixed {
					unresolved[c] = true
				}
			}
		}
	}
}

// smallestFragment returns the size of the smallest connected component of
// WALKABLE land (not sea, not river — a river is exactly the wall a land unit
// cannot cross) among cells, using hex adjacency restricted to cells that are
// still walkable. Used by §7e's fragment guard to detect a river carve that
// split its own landmass into an unplayably small splinter. cells is always
// landmassCells[targetLM] — the landmass's full, immutable membership from
// before any river touched it — so this measures exactly what the carve did
// to THIS landmass, not incidental smallness elsewhere on the map.
func smallestFragment(grid map[cell]Terrain, cells []cell, width, height int) int {
	walkable := make(map[cell]bool, len(cells))
	for _, c := range cells {
		if t := grid[c]; !isSea(t) && t != TerrainRiver {
			walkable[c] = true
		}
	}
	visited := map[cell]bool{}
	smallest := -1 // -1: no walkable component found yet
	for _, start := range cells {
		if !walkable[start] || visited[start] {
			continue
		}
		size := 0
		queue := []cell{start}
		visited[start] = true
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			size++
			for _, d := range riverNeighbourOrder {
				n := cell{cur.q + d[0], cur.r + d[1]}
				if walkable[n] && !visited[n] {
					visited[n] = true
					queue = append(queue, n)
				}
			}
		}
		if smallest == -1 || size < smallest {
			smallest = size
		}
	}
	if smallest == -1 {
		// Nothing walkable left at all — the worst possible fragment, not "no
		// problem". Never actually reachable (a single river cannot consume an
		// entire ≥riverMinComponentTiles landmass), but a real 0 is the
		// correct answer if it ever were.
		return 0
	}
	return smallest
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

// hasBuildableNeighbour reports whether c has at least one hex-adjacent
// neighbour on colonisable terrain (spawnBuildable, the same exclusion list
// join.go's capital placement uses).
//
// P1c (soak 2026-07-18): mountain_limestone — tin's and (one branch of)
// silver's deposit terrain — is ITSELF excluded from spawnBuildable, unlike
// hills (copper, self-buildable: a colony can found directly on the deposit
// tile, trivially landing it in that settlement's own catchment hex). A tin
// tile with no buildable neighbour can never fall inside ANY settlement's
// 7-hex catchment (own hex + 6 neighbours) — it is placed but permanently
// unmineable, independent of how many tin tiles exist in total. Empirically
// (230×230, seeds 0-29) up to 4 of a map's 4-11 tin tiles landed this way —
// a placement-candidate bug, orthogonal to and NOT a relaxation of the
// tinSourceCap scarcity design invariant (Timothy 2026-07-16, plan §A):
// filtering candidates here doesn't change how many clusters get placed,
// only which candidate tiles are eligible to receive one.
func hasBuildableNeighbour(grid map[cell]Terrain, c cell, w, h int) bool {
	for _, n := range hexNeighbours(c, w, h) {
		if spawnBuildable(grid[n]) {
			return true
		}
	}
	return false
}

// hasWaterNeighbour reports whether a land tile borders any coastal_sea or
// river tile — full coastal status (megaron_floden_plan.md, Timothy
// 2026-07-29: a settlement on a river gets harbour/fish/purple/embark same as
// a sea coast). Renamed from hasCoastalSeaNeighbour: the name is the whole
// story of what the flag now means.
func hasWaterNeighbour(grid map[cell]Terrain, c cell, w, h int) bool {
	for _, n := range hexNeighbours(c, w, h) {
		if grid[n] == TerrainCoastalSea || grid[n] == TerrainRiver {
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
