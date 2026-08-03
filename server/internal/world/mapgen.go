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

	// Height-percentile bands within the land range that terrainFor reads:
	// below lowBandMax → low band (food land / scrub), below midBandMax →
	// mid band (the full moisture spread), at/above → high band (bare rock).
	//
	// mapgen/hojdnormalisering: these were min-max thresholds (0.35/0.7)
	// against a rescaled [cutoff, maxHeight] range, calibrated against a
	// distribution shape that itself drifted with map size (the exact bug
	// this slice fixes — see heightPercentile's doc comment). heightNorm is
	// now a percentile, so a raw port of 0.35/0.7 would mean "35 % of land
	// is bandLow, 35 % bandMid, 30 % bandHigh" — roughly 30 % mountain, a
	// ~4x inflation over what the game has shipped and been eyeballed with.
	// Re-derived instead from the 56×40 baseline (smallest map, least
	// compressed, the terrain mix Timothy approved): measured directly off
	// the height field's band membership (bypassing terrainTable, since
	// bandMid's scrub_maquis output overlaps bandLow's across the arid/dry
	// moisture split — terrain-tile counts alone can't isolate the height
	// thresholds) across effective seeds 43/1340/4243 —
	// low=0.586/0.638/0.677 (avg 0.633), mid=0.332/0.270/0.295 (avg 0.299),
	// high=0.082/0.093/0.029 (avg 0.068). A percentile field is uniform on
	// [0,1] by construction, so the cumulative fractions ARE the new
	// thresholds directly: lowBandMax = 0.63, midBandMax = 0.63+0.30 = 0.93.
	lowBandMax = 0.63
	midBandMax = 0.93

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
	// riverBudget is now PER LANDMASS, not global (megaron_plan_flodbudget_och_
	// vadstalle.md, Timothy 2026-07-31: "1-2 STÖRRE floder per landmassa, inte
	// ~26-30 globalt"). The old riverDensityDivisor/minRivers pair (global
	// count = max(minRivers, landArea/riverDensityDivisor), then riverSources
	// ranked every candidate on the WHOLE MAP by height) is gone: on a
	// 230×230 seed a landmass that happened to hold the map's tallest peaks
	// drew 16 of the 18 rivers generated while two dozen other qualifying
	// landmasses got zero (measured red baseline, three seeds, before this
	// slice: rivers_per_landmass [16 1 1] / [10 5 3 1] / [12 6 2 1 1] against
	// 30/23/31 qualifying landmasses — see the slice's proof package). The
	// root was the global sort, not the divisor: no divisor tuning can fix
	// "which landmass" the budget lands on. riverSources now ranks and spaces
	// candidates WITHIN each landmass against that landmass's own budget.

	// riverMinComponentTiles: a land component smaller than this never gets a
	// river source — rivers on specks read as noise, not geography (plan §P3
	// "high-elevation tiles ... preferring LARGE components"). Comfortably
	// above remoteIsleMaxTiles (15) so a forced-metal remote isle never
	// doubles as a river source too.
	riverMinComponentTiles = 25

	// riversSecondRiverTiles: a landmass gets its SECOND river only once it is
	// at least this big — "1-2 per landmass" per Timothy's decision, with the
	// second reserved for landmasses substantial enough to plausibly carry two
	// separate drainage systems rather than one river and its own echo. Set at
	// 4× riverMinComponentTiles: comfortably above the 1-river floor (so most
	// modest qualifying landmasses stop at 1, matching "1-2" reading as
	// "usually 1, sometimes 2" rather than "always 2"), and measured against
	// the three-seed 230×230 baseline (megaron_plan_flodbudget_och_vadstalle.md
	// A1) to land in the 1-2 band with no landmass exceeding 2 — see the
	// slice's proof package for the measured histogram this number produced.
	riversSecondRiverTiles = 100

	// riverSourceSpacing is the minimum hex distance between two river
	// sources on the SAME landmass — plan §P3 "no two sources adjacent or
	// near-adjacent". Landmasses are always sea-separated from each other, so
	// this never needed a cross-landmass check even under the old global
	// ranking; scoping it explicitly per-landmass (see riverSources) is a
	// clarity change, not a behaviour change.
	riverSourceSpacing = 6

	// fordSpacingHexes sets vadställe density: one river_ford per this many
	// hexes of a river's OWN chain length (megaron_plan_flodbudget_och_
	// vadstalle.md, Timothy 2026-08-02), measured in the chain itself, never
	// in map area — a river shorter than this gets zero fords (going around a
	// short river on foot is still reasonable; the port only matters once the
	// wall gets long). Placed at addRiver's own ordered path (see addRiver's
	// doc comment) — that ordering only exists there, before it collapses
	// into an unordered grid write.
	fordSpacingHexes = 10

	// minLandFragment (megaron_floden_plan.md §7e, Timothy 2026-07-29): a river
	// deliberately WALLS its landmass in two — that is the point (land units
	// cannot cross it). But a carve that leaves a splinter this small or
	// smaller is a mapgen bug, not geography: it is undone (the whole carve —
	// line, flanks, delta — is reverted for that source) rather than kept.
	// 12 sits comfortably below riverMinComponentTiles (25, the floor for even
	// STARTING a river) so a river is never rejected by a fragment it created
	// out of ordinary, playable ground — only genuine slivers.
	minLandFragment = 12

	// riverMeanderWavelength/riverMeanderJitter (ögonkoll 2026-07-29 fix — see
	// descentOrder's doc comment): a SPATIALLY SMOOTH low-frequency noise field
	// (riverMeanderField), not per-step independent randomness, is added to
	// each candidate's height before steepest-descent comparison. Independent
	// per-candidate jitter was tried first and rejected: it broke straightness
	// but produced a dense, jagged zigzag (sharp reversals every 1-2 hexes,
	// the "maze" a 230×230 eye-check caught) because nothing tied one step's
	// wobble to the next. Smooth spatial noise instead biases a whole
	// neighbourhood consistently for several hexes before drifting, which
	// reads as a gentle curve — the same fBm technique heightField/moistureField
	// already use for terrain, just applied to path choice instead of terrain
	// lookup. Wavelength sits between highFreqWavelength (8, terrain scatter)
	// and moistureWavelength (14, regional streaks) — long enough for one
	// curve to span several hexes, short enough that a modest river still
	// bends more than once. Jitter amplitude ~ half the mean adjacent-hex
	// height difference (measured 0.041 on a 230×230 field): enough to
	// override a genuine near-tie and turn the river, not enough to make it
	// climb real hills or ignore a strong downhill signal.
	riverMeanderWavelength = 10.0
	riverMeanderJitter     = 0.02
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
	// cedarStandCountMin/Spread: the ABSOLUTE floor on stand count (used by
	// minCedar below, unchanged so small-map validateMap invariants stay
	// intact) — the actual count generateMapOnce uses is scaled up from this
	// floor by land area, see cedarStandAreaDivisor.
	cedarStandCountMin    = 2
	cedarStandCountSpread = 2 // rng.Intn(2) → 0 or 1, extra variety on top of the scaled base

	// cedarStandSizeMin is the REJECTION floor (unchanged — a stand smaller
	// than this reads as an isolated hex, not a forest; see the rejection
	// check's integration-defect comment) and feeds minCedar below. It is NOT
	// the generation target any more — see cedarStandSizeTargetMin/Spread.
	cedarStandSizeMin = 3

	// cedarFractionTarget (Timothy 2026-07-29, mapgen/fuktnormalisering):
	// "cederskog kanske kan ligga kring 3%" — and "cederskogarna" in plural,
	// i.e. a handful of regions a Wanax can sail to and hold, not fifty
	// single-hex dungar. Cedar's total AREA target is this fraction of land,
	// scaled like every other density number in this file (grove/river) — NOT
	// a fixed hex-count literal, so the fraction stays the same shape at every
	// map size instead of only working out at the one size it was measured
	// against (a flat 30-50-hex-per-stand literal was tried first and
	// overshot ~3x on 56×40 — see the process report).
	cedarFractionTarget = 0.03

	// cedarAreaOvershoot compensates for growPatch truncation: with ~10+
	// stands growing simultaneously and competing for the same limited
	// same-landmass tin-biased forest_olive_grove/hills candidates, patches
	// consistently fell short of their per-stand target — stands run out of
	// eligible neighbours before reaching the requested size far more often
	// than the small single-stand case (S2's original 3-7-hex stands) ever
	// did. Seeding the target above cedarFractionTarget's true intent, rather
	// than raising cedarFractionTarget itself, keeps that constant an honest
	// answer to "what fraction should cedar be" separate from "how much to
	// ask growPatch for to actually get there".
	cedarAreaOvershoot = 1.65

	// cedarStandAreaDivisor derives the STAND COUNT from land area
	// (landArea/cedarStandAreaDivisor, floored at cedarStandCountMin) —
	// calibrated so 230×230 (~13 200 land tiles) lands on Timothy's "tiotal
	// bestånd" (~10 stands). Each stand's SIZE is then the area target divided
	// by this count (cedarAreaTarget/cedarStandCount in generateMapOnce), not
	// a separate literal — so a small map with the same count floor (2-3
	// stands) automatically gets small stands totalling ~3% of its own
	// (smaller) land, instead of demanding 230×230-sized regions everywhere.
	cedarStandAreaDivisor = 1000

	// groveDensityDivisor sets small-olive-grove seed count = landArea /
	// groveDensityDivisor (same landArea/divisor style the river budget used
	// before megaron_plan_flodbudget_och_vadstalle.md moved rivers to a
	// per-landmass budget — groves stay global, unaffected). Originally
	// calibrated to 12 against a moisture field that was min-max normalised
	// (squeezed toward the middle on big maps, so terrainFor alone produced
	// almost no natural forest_olive_grove on 230×230 — this pass had to
	// supply nearly the whole forestFraction band by itself). Re-tuned to 20
	// for mapgen/fuktnormalisering: percentile-normalising the moisture field
	// (see moisturePercentile) already gives ~7-14% natural forest_olive_grove
	// at every map size on its own, so the additive top-up only needs to close
	// the gap to the 22-30% band, not supply the whole thing — the old value
	// double-counted and pushed forest_fraction to 0.31-0.38, forcing repeated
	// reseeds.
	groveDensityDivisor = 17

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
	//   NOT IN (coastal_sea, deep_sea, river, river_ford, mountain_limestone, mountain_red, semi_desert)
	// "Catchment" = the 6 axial neighbours RecomputeProduction reads (same as production logic).
	// "West" = q <= maxQ/2; "East" = q > maxQ/2 (east hemisphere, where tin is placed).
	//
	// A tile with a deposit that has ≥1 buildable neighbour is sufficient: that neighbour is
	// a valid colony site and the deposit tile is in its 6-hex catchment.
	halfQ := maxQ / 2
	dirs6 := [6][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
	isBuildable := func(t MapTile) bool {
		switch t.Terrain {
		case TerrainCoastalSea, TerrainDeepSea, TerrainRiver, TerrainRiverFord,
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

	// Every river_ford hex has exactly 2 river-family (river ∪ river_ford)
	// neighbours (megaron_plan_flodbudget_och_vadstalle.md steg 3/8, Timothy
	// 2026-08-02: "vadstället ligger i floden") — it sits mid-chain, never at
	// an endpoint or a branch. This alone also proves a ford never lands on a
	// chain's source or mouth-adjacent hex: those are a simple path's only
	// two degree-1 nodes, so an endpoint mistakenly converted would trip this
	// exact assertion (see addRiver's ford-placement comment).
	for c, t := range grid {
		if t != TerrainRiverFord {
			continue
		}
		fam := 0
		for _, d := range riverNeighbourOrder {
			nt := grid[cell{c.q + d[0], c.r + d[1]}]
			if nt == TerrainRiver || nt == TerrainRiverFord {
				fam++
			}
		}
		if fam != 2 {
			fails = append(fails, fmt.Sprintf("river_ford hex (%d,%d) has %d river-family neighbours (want exactly 2)", c.q, c.r, fam))
		}
	}

	// Two river_ford hexes never share an edge (steg 3) — distinct from the
	// degree-2 check above: two adjacent fords would each still show degree 2
	// (each other counts as river-family), so this needs its own assertion.
	for c, t := range grid {
		if t != TerrainRiverFord {
			continue
		}
		for _, d := range riverNeighbourOrder {
			if grid[cell{c.q + d[0], c.r + d[1]}] == TerrainRiverFord {
				fails = append(fails, fmt.Sprintf("two river_ford hexes share an edge at (%d,%d)", c.q, c.r))
			}
		}
	}

	// Per-chain checks (§7a/§7d, unchanged in spirit from before this slice,
	// plus the two NEW ford invariants steg 8 adds) and the per-landmass
	// river-count budget (steg 2/8) — all derived from the same riverChains
	// BFS (see its doc comment for why a river_ford counts as "river" for that
	// traversal).
	comp := landComponents(tiles)
	landmassSize := map[int]int{}
	for _, t := range tiles {
		if tileIsLand(t.Terrain) {
			landmassSize[comp[[2]int{t.Q, t.R}]]++
		}
	}
	riversByLandmass := map[int]int{}
	for _, ci := range riverChains(tiles, width, height) {
		if !ci.hasDelta {
			fails = append(fails, fmt.Sprintf("river chain on landmass %d (size %d) has no river_delta tile", ci.landmass, ci.size))
		}
		if !ci.hasMainSea {
			fails = append(fails, fmt.Sprintf("river chain on landmass %d (size %d) never reaches the main sea", ci.landmass, ci.size))
		}
		if ci.size >= fordSpacingHexes && ci.fords < 1 {
			fails = append(fails, fmt.Sprintf("river chain on landmass %d (size %d) has no river_ford despite being >= %d hexes",
				ci.landmass, ci.size, fordSpacingHexes))
		}
		riversByLandmass[ci.landmass]++
	}
	for lm, size := range landmassSize {
		if size < riverMinComponentTiles {
			continue
		}
		n := riversByLandmass[lm]
		if n < 1 || n > 2 {
			fails = append(fails, fmt.Sprintf("landmass %d (%d tiles) has %d rivers (want 1-2)", lm, size, n))
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
	cutoff, _ := landCutoff(field, landFraction)
	landSet := make(map[cell]bool, width*height)
	for c, v := range field {
		if v >= cutoff {
			landSet[c] = true
		}
	}
	// Percentile-normalised (mapgen/hojdnormalisering), same mechanism as
	// moisturePct below — see heightPercentile's doc comment. landCutoff's
	// maxHeight return is no longer read: the mid-max rescale it fed is gone.
	heightPct := heightPercentile(field, landSet)

	// ── 1b. Moisture field (P2) ──────────────────────────────────────────
	// Independent field, drawn from the SAME map rng one step later than the
	// height-field noise generators — still fully determined by the map seed,
	// no second seed parameter needed for determinism.
	//
	// Percentile-normalised (mapgen/fuktnormalisering), same mechanism as
	// landCutoff for the height field: a plain min-max rescale has a
	// denominator (max-min) that grows with sample count, so bigger maps
	// sample more extreme noise values and ordinary readings get squeezed
	// toward the middle — measured as the 230×230 olive-grove collapse this
	// slice fixes. moisturePercentile ranks every LAND cell instead, so the
	// normalised distribution's shape no longer depends on map size.
	moisture := moistureField(rng, width, height)
	moisturePct := moisturePercentile(moisture, landSet)

	// ── 1c. River meander field (ögonkoll 2026-07-29 fix) ────────────────
	// Independent field, one more deterministic rng draw after moisture —
	// see riverMeanderField's doc comment for why this needs to be spatially
	// smooth noise rather than per-step randomness.
	riverMeander := riverMeanderField(rng, width, height)

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
			heightNorm := heightPct[c]
			moistureNorm := moisturePct[c]
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
	// riverBudget: 1 river for every landmass >= riverMinComponentTiles, 2 for
	// every one of those also >= riversSecondRiverTiles (megaron_plan_
	// flodbudget_och_vadstalle.md steg 2 — see the const block's doc comment
	// for the red-baseline skew this budget shape replaces). A landmass absent
	// from this map gets budget[lm]==0 via Go's zero value, which riverSources
	// reads the same as an explicit 0 — no separate "not qualifying" branch
	// needed.
	riverBudget := map[int]int{}
	for lm, sz := range compSize {
		if lm == lmSea || sz < riverMinComponentTiles {
			continue
		}
		riverBudget[lm] = 1
		if sz >= riversSecondRiverTiles {
			riverBudget[lm] = 2
		}
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

	for _, src := range riverSources(field, landmap, grid, riverBudget, width, height) {
		targetLM := landmap[src]
		cells := landmassCells[targetLM]

		before := make(map[cell]Terrain, len(cells))
		for _, c := range cells {
			before[c] = grid[c]
		}

		addRiver(grid, landmap, field, riverMeander, mainSea, rng, src, width, height)
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
		m := moisturePct[c]
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

	// ── 6b. Cedar forest stands (S2 plan step 4; recalibrated
	// mapgen/fuktnormalisering to hit ~3% of land at EVERY map size) ────────
	// forest_cedar is now its own terrain, not a flag on forest_olive_grove:
	// seed hexes (same tin-biased-forest bias the old scattered-flag code
	// used), area-scaled count (cedarStandAreaDivisor — ~10 stands at
	// 230×230), each grow toward a CONTIGUOUS REGION sized so all stands
	// together total cedarFractionTarget of land (not a flat hex-count
	// literal — see cedarFractionTarget's comment for why a fixed 30-50
	// overshot small maps ~3x), by converting same-landmass
	// forest_olive_grove/hills neighbours (including any grove 6a just grew —
	// harmless, it just means fewer, bigger patches). Run AFTER 6a and rivers
	// so cedar always gets first claim on the final terrain, and BEFORE step
	// 7's tile build so CedarDeposit can be derived as a pure mirror of the
	// terrain in exactly one place.
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
	// Stand count scales with land area (cedarStandAreaDivisor's comment) so
	// 230×230 gets ~10 regions instead of the old flat 2-3 — the flat count
	// could never reach the 3%-of-land target no matter how big each stand
	// grew, since 2-3 stands capped at 3-7 hexes each maxed out around 21
	// hexes total (0.1-0.2% of land, not 3%).
	cedarStandCount := landAreaForGroves / cedarStandAreaDivisor
	if cedarStandCount < cedarStandCountMin {
		cedarStandCount = cedarStandCountMin
	}
	cedarStandCount += rng.Intn(cedarStandCountSpread)
	// cedarAreaTarget is cedarFractionTarget of THIS map's land, so the
	// per-stand size (avgCedarStandSize below) automatically shrinks on small
	// maps instead of always demanding a 230×230-sized region — a fixed
	// 30-50-hex literal here overshot 56×40 (560 land tiles) by ~3x, since
	// even 2 stands of 35+ hexes each is already >10% of that map's land.
	cedarAreaTarget := int(math.Round(cedarFractionTarget * cedarAreaOvershoot * float64(landAreaForGroves)))
	avgCedarStandSize := cedarAreaTarget / cedarStandCount
	if avgCedarStandSize < cedarStandSizeMin {
		avgCedarStandSize = cedarStandSizeMin
	}
	// ±1/3 jitter around the average so stands vary in size like every other
	// per-seed random pick in this file, without losing the area target.
	cedarSizeSpread := avgCedarStandSize / 3
	if cedarSizeSpread < 1 {
		cedarSizeSpread = 1
	}
	cedarBuilt := 0
	for _, seed := range cedarSeedCand {
		if cedarBuilt >= cedarStandCount {
			break
		}
		if cedarUsed[seed] {
			continue
		}
		target := avgCedarStandSize - cedarSizeSpread + rng.Intn(2*cedarSizeSpread+1)
		if target < cedarStandSizeMin {
			target = cedarStandSizeMin
		}
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
				// kust åt sig själv, därav det explicita undantaget. Vadstället
				// är samma sak (megaron_plan_flodbudget_och_vadstalle.md): det
				// är också vatten, aldrig kust åt sig själv, men en granne till
				// ett vadställe får full kuststatus precis som en flodgranne
				// (se hasWaterNeighbour).
				Coastal: !isSea(terrain) && terrain != TerrainRiver && terrain != TerrainRiverFord &&
					hasWaterNeighbour(grid, c, width, height),
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

// isWalkableLand reports whether a land unit can stand on t — land per
// tileIsLand MINUS river, which is water carved INTO a landmass, not a
// terrain type of its own kind of sea (megaron_floden_plan.md ögonkoll
// 2026-07-29, Timothy: "floden är land för ytmått ... men INTE för
// framkomlighet"). tileIsLand/isSea/landComponents are deliberately left
// alone: LandFraction, LargestComponentFraction and the copper/tin-share-
// component check are surface-AREA measures — a river is not open sea, it
// lies inside the landmass, so it should keep counting as land there. This
// function exists ONLY for the walkability half: can a land unit actually
// reach this tile on foot. Used by walkableComponents below.
func isWalkableLand(t Terrain) bool {
	return tileIsLand(t) && t != TerrainRiver
}

// walkableComponents groups contiguous WALKABLE tiles (isWalkableLand, i.e.
// land minus river) into connected components — the same shape as
// landComponents, but river is a wall here instead of invisible. Two tiles
// on the same landComponents id can land in different walkableComponents ids
// if a river runs between them; that split is the whole point (§7e's
// fragment guard, and the "how much of the biggest landmass is actually
// reachable" question an ögonkoll on 230×230 raised: landComponents alone
// reported identical largest_component_fraction with and without river
// carving, because isSea(river) is false and tileIsLand(river) is
// therefore true — a real wall no surface-area invariant could ever see).
func walkableComponents(tiles []MapTile) map[[2]int]int {
	terrain := map[[2]int]Terrain{}
	for _, t := range tiles {
		terrain[[2]int{t.Q, t.R}] = t.Terrain
	}
	comp := map[[2]int]int{}
	next := 0
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
	for _, t := range tiles {
		key := [2]int{t.Q, t.R}
		if !isWalkableLand(t.Terrain) {
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
				if !ok || !isWalkableLand(tt) {
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

// riverMeanderField is a spatially smooth ([-1,1]-ish) noise field descentOrder
// adds to the height field before sorting a river's candidate steps —
// megaron_floden_plan.md ögonkoll 2026-07-29 fix, see riverMeanderJitter's
// comment for why this needs to be spatially SMOOTH (a whole neighbourhood
// biased consistently) rather than independently random per candidate.
// Same technique and independent seed draw as moistureField; a distinct
// field (not a reuse of height or moisture) so meander doesn't accidentally
// correlate with — and so cancel out or double up on — the terrain those
// already shape.
func riverMeanderField(rng *rand.Rand, width, height int) map[cell]float64 {
	noise := perlin.NewPerlin(2, 2, 2, rng.Int63())

	field := make(map[cell]float64, width*height)
	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			row := float64(r - base)
			field[cell{q, r}] = noise.Noise2D(float64(q)/riverMeanderWavelength, row/riverMeanderWavelength)
		}
	}
	return field
}

// moisturePercentile ranks every LAND cell's moisture value and returns its
// percentile (0..1, rank/(n-1)) instead of a plain min-max rescale
// (mapgen/fuktnormalisering, replaces the old moistureRange). The old
// approach's denominator (max-min) grows with sample count, so bigger maps
// sample more extreme noise values and ORDINARY readings get squeezed toward
// the middle — a larger map therefore reads as more uniformly "medium
// moisture" even though the underlying fBm shape hasn't changed. This is the
// exact mechanism landCutoff already uses for the height field (a percentile
// threshold, chosen so land share is IDENTICAL at every map size); here every
// value is ranked, not just a single cutoff, since terrainFor needs a full
// [0,1] reading per cell rather than a land/sea split.
//
// Scoped to landSet, not the whole field: moistureNorm is only ever read for
// land cells (terrainFor, grove/cedar seeding) — folding sea noise into the
// population would dilute the land distribution with values nobody
// classifies, and moisture is only computed for land tiles' terrain anyway.
func moisturePercentile(field map[cell]float64, landSet map[cell]bool) map[cell]float64 {
	type entry struct {
		c cell
		v float64
	}
	entries := make([]entry, 0, len(landSet))
	for c := range landSet {
		entries = append(entries, entry{c, field[c]})
	}
	// Deterministic sort: value first, then (q, r) as a tie-break so equal
	// float values (rare, but Perlin can repeat) don't depend on Go's
	// unordered map iteration.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].v != entries[j].v {
			return entries[i].v < entries[j].v
		}
		if entries[i].c.q != entries[j].c.q {
			return entries[i].c.q < entries[j].c.q
		}
		return entries[i].c.r < entries[j].c.r
	})
	norm := make(map[cell]float64, len(entries))
	n := len(entries)
	for i, e := range entries {
		if n > 1 {
			norm[e.c] = float64(i) / float64(n-1)
		} else {
			norm[e.c] = 0
		}
	}
	return norm
}

// heightPercentile ranks every LAND cell's raw height value and returns its
// percentile (0..1, rank/(n-1)) instead of a plain min-max rescale
// (mapgen/hojdnormalisering, replaces the old (field[c]-cutoff)/(maxHeight-
// cutoff) reading generateMapOnce used). Same mechanism and same reason as
// moisturePercentile: a min-max denominator grows with sample count, so
// bigger maps sample more extreme noise and ordinary readings get squeezed
// toward the middle — measured on this field as bandHigh (mountain) land
// share going 6.8 % (56×40) → 5.0 % (120×120) → 1.65 % (230×230), the same
// collapse moisturePercentile already fixed one field over.
//
// Body is a duplicate of moisturePercentile's rather than a shared helper —
// two independent fields with the same shape of fix, kept separate so each
// can be read (and re-derived) on its own without a shared abstraction
// standing between them.
func heightPercentile(field map[cell]float64, landSet map[cell]bool) map[cell]float64 {
	type entry struct {
		c cell
		v float64
	}
	entries := make([]entry, 0, len(landSet))
	for c := range landSet {
		entries = append(entries, entry{c, field[c]})
	}
	// Deterministic sort: value first, then (q, r) as a tie-break so equal
	// float values (rare, but Perlin can repeat) don't depend on Go's
	// unordered map iteration.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].v != entries[j].v {
			return entries[i].v < entries[j].v
		}
		if entries[i].c.q != entries[j].c.q {
			return entries[i].c.q < entries[j].c.q
		}
		return entries[i].c.r < entries[j].c.r
	})
	norm := make(map[cell]float64, len(entries))
	n := len(entries)
	for i, e := range entries {
		if n > 1 {
			norm[e.c] = float64(i) / float64(n-1)
		} else {
			norm[e.c] = 0
		}
	}
	return norm
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

// riverSources picks well-separated local-height-maxima land cells as river
// start points (plan §P3: "högsta lokala höjdpunkter på stora
// landkomponenter"), PER LANDMASS (megaron_plan_flodbudget_och_vadstalle.md
// steg 2): budget[lm] caps how many sources landmass lm may contribute — 0
// for anything not in the map (a landmass below riverMinComponentTiles, see
// the caller). Ranking and spacing are unchanged in spirit from the old
// global version — candidates sorted by height descending, ties broken by
// (q, r) for full determinism — but accepted greedily WITHIN each landmass
// against that landmass's own budget and its own already-chosen sources, so
// the tallest peaks on ONE landmass can no longer starve every other
// landmass's budget (the red-baseline bug this steg fixes — see the const
// block's doc comment). "Färre men längre" (Timothy) falls out of this for
// free: a shrunk per-landmass budget still always picks that landmass's OWN
// highest point(s), giving the longest possible descent to the sea — verify
// this in the A1 measurement, don't assume it.
func riverSources(field map[cell]float64, landmap map[cell]int, grid map[cell]Terrain, budget map[int]int, width, height int) []cell {
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
			if budget[lm] <= 0 || isSea(grid[c]) {
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

	chosen := map[int]int{}       // landmass id -> sources accepted so far
	chosenOnLM := map[int][]cell{} // landmass id -> those sources, for spacing
	var sources []cell
	for _, cd := range candidates {
		lm := landmap[cd.c]
		if chosen[lm] >= budget[lm] {
			continue
		}
		tooClose := false
		for _, s := range chosenOnLM[lm] {
			if hexDist(cd.c, s) < riverSourceSpacing {
				tooClose = true
				break
			}
		}
		if tooClose {
			continue
		}
		sources = append(sources, cd.c)
		chosenOnLM[lm] = append(chosenOnLM[lm], cd.c)
		chosen[lm]++
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

// adjacentToOwnPath reports whether candidate cand — a prospective next step
// from cur — is hex-adjacent to any cell this SAME descent has already
// explored, other than cur itself (cand's would-be legitimate predecessor).
// Prevention, not cure, for the self-touch half of §7c (adjacentToExistingRiver
// above is the cross-river half): meander (riverMeanderJitter) makes the walk
// double back near its own earlier ground far more often than pure steepest
// descent ever did, and thinRiverJunctions' after-the-fact fix-up can only
// ever demote ONE of a pinch's cells without risking a different river's
// delta connectivity elsewhere (megaron_floden_plan.md ögonkoll 2026-07-29 —
// meander alone, without this, turned a handful of self-touches per map into
// dozens, overwhelming that backstop at 230×230). visited also holds
// abandoned dead-end branches (never cleared on backtrack), so this is
// intentionally conservative — a path that never comes within 1 hex of its
// own past is guaranteed pinch-free by construction, which is worth some
// extra backtracking to get.
func adjacentToOwnPath(visited map[cell]bool, cur, cand cell, width, height int) bool {
	for _, d := range riverNeighbourOrder {
		n := cell{cand.q + d[0], cand.r + d[1]}
		if n == cur {
			continue // cur is cand's own legitimate predecessor
		}
		if visited[n] {
			return true
		}
	}
	return false
}

// descentOrder returns c's land neighbours on targetLM (sea and other
// landmasses excluded — a river only ever steps onto its own component or,
// via firstSeaNeighbour, straight into its mouth) sorted by height-PLUS-MEANDER
// ascending: steepest descent first, but with riverMeanderField's smooth
// spatial noise added to each candidate's height before comparing
// (megaron_floden_plan.md ögonkoll 2026-07-29 — Timothy: "en flod ska
// meandra"). Pure steepest-descent alone still meanders where the terrain
// itself undulates, but on a smooth, near-uniform slope the SAME neighbour
// wins every single step, and a perfectly consistent winner is a straight
// line — the "circuit board" symptom the eye-check caught once the line
// became visible water instead of invisible river_valley. meander is a fixed
// field (computed once per map from the world's rng, fully deterministic per
// seed), not fresh randomness per call, and is intentionally spatially SMOOTH
// rather than independently random per candidate — an independent-per-step
// version was tried first and produced a jagged zigzag (sharp reversals every
// 1-2 hexes) instead of a curve, because nothing tied one step's wobble to
// the next. Genuine height differences larger than the jitter amplitude still
// dominate, so the river still trends downhill overall; it just no longer
// picks the identical direction every time two neighbours are close in
// height.
func descentOrder(field, meander map[cell]float64, landmap map[cell]int, targetLM int, c cell, width, height int) []cell {
	var out []cell
	for _, d := range riverNeighbourOrder {
		n := cell{c.q + d[0], c.r + d[1]}
		if !inMap(n.q, n.r, width, height) || landmap[n] != targetLM {
			continue
		}
		out = append(out, n)
	}
	biased := make(map[cell]float64, len(out))
	for _, n := range out {
		biased[n] = field[n] + meander[n]*riverMeanderJitter
	}
	sort.SliceStable(out, func(i, j int) bool { return biased[out[i]] < biased[out[j]] })
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
func addRiver(grid map[cell]Terrain, landmap map[cell]int, field, meander map[cell]float64, mainSea map[cell]bool, rng *rand.Rand, source cell, width, height int) {
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
	stack := []frame{{c: source, remaining: descentOrder(field, meander, landmap, targetLM, source, width, height)}}

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
			// Refuse a candidate that would pinch the width invariant (§7c),
			// either against an EARLIER river (grid only holds rivers already
			// accepted from prior sources at this point — the current walk's
			// own path isn't written into grid until after it finishes) or
			// against THIS walk's own past (adjacentToOwnPath — meander makes
			// self-touch common, not the rare case thinRiverJunctions was
			// originally sized for). Both are prevention, not cure:
			// thinRiverJunctions afterward is now a backstop for whatever
			// slips through, not the primary mechanism.
			if !visited[cand] && !adjacentToExistingRiver(grid, cand, width, height) &&
				!adjacentToOwnPath(visited, cur, cand, width, height) {
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
		stack = append(stack, frame{c: next, remaining: descentOrder(field, meander, landmap, targetLM, next, width, height)})
	}

	if !reached {
		// Should be unreachable (see doc comment) — treat defensively as "no
		// river" rather than carve a corridor that never gets a delta.
		return
	}

	// The DFS stack IS the river (ögonkoll 2026-07-29 fix, superseding the
	// "round-2" comment this replaced): path is exactly the current stack at
	// every step (backtracking pops both in lockstep — see the !found branch
	// above), so it is already loop-free and 1-cell-wide as a SEQUENCE, with
	// no re-derivation needed. The previous version instead re-derived the
	// SHORTEST path from source to origin through the whole visited corridor
	// (riverLine, since removed) to avoid "flat pit" runs ballooning into a
	// wide fertile blob — but a shortest path through a hex grid is by
	// construction close to a single straight line, which was invisible while
	// the line became river_valley (fertile ground, no shape to judge) and is
	// now a glaring "circuit board" of dead-straight water once the line
	// itself is the terrain (Timothy's eye-check, 230×230). The blob risk
	// riverLine solved is also gone on its own terms: flanks are now a THIN
	// band one hex deep along whatever line is carved (§7b), not a fill of
	// the whole explored area, so a longer, more winding path just means a
	// longer, still-thin river+valley system — meander is not a starvation
	// hazard here, only a straight line to nowhere was. descentOrder's random
	// jitter (added in the same fix) does the actual meandering: pure
	// steepest-descent alone still picks the identical winner on every step
	// of a smooth slope.
	origin := path[len(path)-1]
	line := path

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

	// Vadställen: the river's port (megaron_plan_flodbudget_och_vadstalle.md
	// steg 3, Timothy 2026-08-02). Placed HERE, on `line` — the DFS's own
	// ordered path — because that ordering is only available here, before it
	// scatters into an unordered grid write; validateMap's black-box checks
	// (riverInvariantFailures) can only re-derive UNORDERED connectivity from
	// tiles afterward, not "where along the chain" a hex sits.
	//
	// Density: floor(len(line)/fordSpacingHexes), evenly spaced at
	// fordSpacingHexes/2 + k*fordSpacingHexes — a river shorter than
	// fordSpacingHexes gets zero (going around a short river on foot is still
	// reasonable). Never index 0 (source) or len(line)-1 (origin, the
	// mouth-adjacent hex, already asserted above to border the delta): those
	// are the chain's only two degree-1 nodes in the river-family graph, so
	// converting either would make it fail riverInvariantFailures' "every
	// river_ford hex has exactly 2 river-family neighbours" check — that
	// assertion alone is sufficient proof no endpoint ever becomes a ford,
	// which is why there is no separate index-bounds test for it.
	fords := len(line) / fordSpacingHexes
	for k := 0; k < fords; k++ {
		idx := fordSpacingHexes/2 + k*fordSpacingHexes
		if idx <= 0 || idx >= len(line)-1 {
			continue
		}
		grid[line[idx]] = TerrainRiverFord
	}
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
// belongs to a connected river-family (river ∪ river_ford) group that contains
// at least one TerrainRiverDelta tile. Used by thinRiverJunctions as the guard
// rail: a demotion is only ever kept if the map still satisfies this
// afterward. A river_ford counts as traversable here (megaron_plan_
// flodbudget_och_vadstalle.md steg 3/6) — it is called once PER SOURCE, in the
// SAME outer loop that places fords (see generateMapOnce), so by the time a
// later river's pinch gets tested, an earlier river may already have a ford
// mid-chain; without this, that ford would silently cut its own river's
// TerrainRiver-only BFS into two "components", one of which never reaches a
// delta — a false failure that would make every subsequent thinRiverJunctions
// call on the whole map reject every candidate demotion, not just that one
// river's.
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
					if (t == TerrainRiver || t == TerrainRiverFord) && !seen[n] {
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
// Timothy 2026-07-29: "exakt 1 hex bred"). The carved line is addRiver's own
// DFS stack (path) and is therefore already loop-free by construction, but
// not touch-free: two cells that are NOT consecutive in the path can still
// end up hex-adjacent (a diagonal pinch where the meandering route passes
// close to itself, or two different rivers' lines coming near each other),
// which would locally read as two hexes wide. addRiver's own descent now
// actively AVOIDS creating these (adjacentToOwnPath, adjacentToExistingRiver)
// — this pass is the backstop for whatever slips through anyway, not the
// primary mechanism.
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

// riverChainInfo is one connected "river family" component — river ∪
// river_ford tiles, hex-adjacent — the same shape as one addRiver call
// produced (minus its river_valley flanks and delta, which are not part of
// the water path itself). Re-derived from the flattened tile list, never
// threaded through from generation, exactly like every other black-box check
// in this file (see riverInvariantFailures's doc comment).
type riverChainInfo struct {
	landmass   int
	size       int // river + river_ford tile count — "the chain's own length"
	fords      int
	hasDelta   bool
	hasMainSea bool
}

// riverChains groups every TerrainRiver/TerrainRiverFord tile into connected
// river-family components. A river_ford counts as "river" for THIS traversal
// (it sits mid-chain, still water) but is never itself a BFS start point —
// every chain's two path endpoints (source and the mouth-adjacent hex) are
// always plain TerrainRiver by construction (addRiver never places a ford on
// index 0 or len(line)-1), so starting only from TerrainRiver cells still
// reaches every chain in full.
//
// Used both by riverInvariantFailures (the per-chain §7a/§7d/ford-density
// assertions) and by ComputeMapMetrics (the "rivers per landmass" + "chain
// length" A1 report) — one BFS, two consumers, instead of duplicating it.
func riverChains(tiles []MapTile, width, height int) []riverChainInfo {
	grid := make(map[cell]Terrain, len(tiles))
	for _, t := range tiles {
		grid[cell{t.Q, t.R}] = t.Terrain
	}
	if len(grid) == 0 {
		return nil
	}
	comp := landComponents(tiles)
	mainSea := mainSeaComponent(grid, width, height)

	seen := map[cell]bool{}
	var chains []riverChainInfo
	for q := 0; q < width; q++ {
		base := rowOrigin(q, width)
		for r := base; r < base+height; r++ {
			start := cell{q, r}
			if grid[start] != TerrainRiver || seen[start] {
				continue
			}
			info := riverChainInfo{landmass: comp[[2]int{start.q, start.r}]}
			queue := []cell{start}
			seen[start] = true
			for len(queue) > 0 {
				cur := queue[0]
				queue = queue[1:]
				info.size++
				if grid[cur] == TerrainRiverFord {
					info.fords++
				}
				for _, n := range hexNeighbours(cur, width, height) {
					nt := grid[n]
					if nt == TerrainRiverDelta {
						info.hasDelta = true
					}
					if mainSea[n] {
						info.hasMainSea = true
					}
					if (nt == TerrainRiver || nt == TerrainRiverFord) && !seen[n] {
						seen[n] = true
						queue = append(queue, n)
					}
				}
			}
			chains = append(chains, info)
		}
	}
	return chains
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

// hasWaterNeighbour reports whether a land tile borders any coastal_sea,
// river or river_ford tile — full coastal status (megaron_floden_plan.md,
// Timothy 2026-07-29: a settlement on a river gets harbour/fish/purple/embark
// same as a sea coast; megaron_plan_flodbudget_och_vadstalle.md extends the
// same rule to river_ford — a settlement beside the port is beside water,
// exactly as beside the river itself, and NOT extending it would carve a
// donut of missing coastal status into an otherwise-coastal riverbank
// wherever a ford happens to sit). Renamed from hasCoastalSeaNeighbour: the
// name is the whole story of what the flag now means.
func hasWaterNeighbour(grid map[cell]Terrain, c cell, w, h int) bool {
	for _, n := range hexNeighbours(c, w, h) {
		if grid[n] == TerrainCoastalSea || grid[n] == TerrainRiver || grid[n] == TerrainRiverFord {
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
