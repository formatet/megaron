package province

import (
	"fmt"
	"math"

	"formatet/megaron/server/internal/hexgrid"
)

// HexDistance returns the distance between two axial hex coordinates.
func HexDistance(a, b MapPosition) int {
	dq := a.Q - b.Q
	dr := a.R - b.R
	return (abs(dq) + abs(dq+dr) + abs(dr)) / 2
}

// VisibleFrom returns true if target is within radius hexes of any province in origins.
//
// This is the KNOWN-set reachability gate (live ∪ remembered ∪ contacted) used by
// messenger Send to decide whether a player can dispatch a courier to a destination
// at all — a player must keep being able to contact a city they've already
// discovered even outside current live sight, so this deliberately does not shrink
// to the tiered live-vision radii (LiveRadius). It is NOT used for surfacing
// province/city marker DATA any more (world.go's /provinces, /wanaxes, /cities used
// to gate on this flat radius too, but that let a marker surface for a hex /map
// still reported as fog — they now gate on the same tier-1∪tier-2 knowledge /map
// uses; see api/handlers/world.go's knownToPlayer, fow/provinces-samma-kunskap
// 2026-07-30). See temenos_synlighet.md for the tiered-visibility model this sits
// alongside.
func VisibleFrom(target MapPosition, origins []MapPosition, radius int) bool {
	for _, o := range origins {
		if HexDistance(o, target) <= radius {
			return true
		}
	}
	return false
}

// HexNeighbors returns the 6 axial neighbours of pos, via hexgrid.Neighbors
// (the single source of truth for this offset set — megaron_todo.md
// "7-hex-catchmentlistan är duplicerad").
func HexNeighbors(pos MapPosition) [6]MapPosition {
	var out [6]MapPosition
	for i, c := range hexgrid.Neighbors(hexgrid.Coord{Q: pos.Q, R: pos.R}) {
		out[i] = MapPosition{Q: c.Q, R: c.R}
	}
	return out
}

// Eye kinds for live-vision sources (temenos_synlighet.md tier 1).
const (
	EyeSettlement = "settlement"
	EyeLandUnit   = "land-unit"
	// EyeNomadicHost is the founder-phase host. Its live radius is the ordinary
	// land-unit radius (2) — see LiveRadius. The kind is kept distinct because
	// other surfaces identify a host by it, not because it sees differently.
	EyeNomadicHost = "nomadic-host"
	EyeShip       = "ship"
)

// Eye is a live vision source (a settlement or a positioned/marching unit) at a
// map position.
type Eye struct {
	Pos  MapPosition
	Kind string // EyeSettlement | EyeLandUnit | EyeNomadicHost | EyeShip
	// seaHorizon is the set of sea hexes this eye sees on the open horizon: those
	// within SeaHorizonRadius that an unbroken line of open water reaches
	// (SeaSightline). Filled by SetSeaHorizons from the map around the eye —
	// LoadLiveEyes and SweepLiveRadius do it for every eye they build. The zero
	// value (nil) is the fail-closed default: a hand-built or unclassified Eye
	// reads the sea at its ordinary vantage, so a missing lookup can only ever
	// hide, never reveal.
	seaHorizon map[MapPosition]struct{}
}

// SeaHorizonRadius is how far an eye sees out over open water (temenos_synlighet.md
// tier 1, "sea = 4"). It reaches only along a straight line of open water — see
// SeaSightline. Tunable, not an invariant.
const SeaHorizonRadius = 4

// IsSea reports whether terrain is open water: coastal_sea or deep_sea.
// Deliberately NOT river: the horizon comes from open water, and a 1-hex-wide
// river between tall banks opens none (megaron_floden_plan.md §5, Timothy
// 2026-07-29). Nor map_tiles.coastal, which migration 101 widened to mean
// "adjacent to any water, river included".
func IsSea(terrain string) bool {
	return terrain == "coastal_sea" || terrain == "deep_sea"
}

// LiveRadius returns the ordinary live-vision radius for an eye of eyeKind looking
// at a tile of targetTerrain: the eye's vantage (settlement 3 / land-unit 2 /
// ship 1), plus +2 for mountains, which are landmarks. A sea tile is read at this
// same vantage — the open horizon over the sea (SeaHorizonRadius) is NOT a radius,
// because it depends on what lies between eye and target, not on distance alone.
// It is granted per eye by SetSeaHorizons and applied in Eye.Sees.
//
// History: until 2026-08-05 the sea branch returned 4 for every eye; until
// 2026-09-25 it returned 4 for every eye "at the water" (own hex sea, or a sea
// neighbour) as a full disk — so a spearman on a shore saw an enclosed lake 4 hexes
// behind him across forest, hills and a mountain ridge. Timothy 2026-09-25: the
// open horizon is a SIGHTLINE rule.
func LiveRadius(eyeKind string, targetTerrain string) int {
	base := 2
	switch eyeKind {
	case EyeSettlement:
		base = 3
	case EyeShip:
		base = 1
	case EyeLandUnit:
		base = 2
	case EyeNomadicHost:
		// 1 → 2, Timothy 2026-08-22: "synradie för alla landenheter är två".
		// This reverses the 2026-07-15 decision that halved the host's reach
		// ("a people on the move, not a scout"). The rule is now uniform — every
		// eye on land reads ordinary ground at 2; the only departures from that
		// are the settlement's vantage (3), the ship's blindness inland (1), the
		// mountain landmark bonus and the open horizon over water (Eye.Sees).
		base = 2
	}
	if targetTerrain == "mountain_limestone" || targetTerrain == "mountain_red" {
		base += 2
	}
	return base
}

// Sees reports whether this eye has live sight of target (of targetTerrain): within
// its ordinary vantage (LiveRadius), or a sea hex on its open-water horizon. This
// is the one sight test — AnyEyeSees and SweepLiveRadius both go through it.
func (e Eye) Sees(target MapPosition, targetTerrain string) bool {
	if HexDistance(e.Pos, target) <= LiveRadius(e.Kind, targetTerrain) {
		return true
	}
	_, onHorizon := e.seaHorizon[target]
	return onHorizon
}

// AnyEyeSees returns true if target (of targetTerrain) is within live vision of any
// of the given eyes — see Eye.Sees.
func AnyEyeSees(eyes []Eye, target MapPosition, targetTerrain string) bool {
	for _, e := range eyes {
		if e.Sees(target, targetTerrain) {
			return true
		}
	}
	return false
}

// SetSeaHorizons fills each eye's open-water horizon: every sea hex within
// SeaHorizonRadius that SeaSightline reaches from the eye. terrainAt returns a
// hex's terrain, or "" for a hex it does not know (off the map, not loaded) —
// which reads as not-sea and so blocks, fail closed. It must cover the
// SeaHorizonRadius disk around every eye. Mutates eyes in place.
func SetSeaHorizons(eyes []Eye, terrainAt func(MapPosition) string) {
	isSea := func(p MapPosition) bool { return IsSea(terrainAt(p)) }
	for i := range eyes {
		var horizon map[MapPosition]struct{}
		for _, c := range hexgrid.Disk(hexgrid.Coord{Q: eyes[i].Pos.Q, R: eyes[i].Pos.R}, SeaHorizonRadius) {
			target := MapPosition{Q: c.Q, R: c.R}
			if target == eyes[i].Pos || !SeaSightline(eyes[i].Pos, target, isSea) {
				continue
			}
			if horizon == nil {
				horizon = make(map[MapPosition]struct{})
			}
			horizon[target] = struct{}{}
		}
		eyes[i].seaHorizon = horizon
	}
}

// SeaSightline reports whether an eye at from looks across open water to the sea
// hex to: to must be sea, and EVERY hex strictly between them on the straight hex
// line must be sea. The eye's own hex may be land — a unit on the shore, a coastal
// city — so the line may start from it. One land (or river, or unknown) hex
// anywhere between blocks the horizon (Timothy 2026-09-25). An eye with no sea
// neighbour therefore never has a horizon: its first step off its own hex is land.
//
// Tie rule: where the line runs exactly along the edge between two hexes, it is
// drawn twice, nudged a hair to either side (hexLine), and the sightline holds if
// EITHER drawing is all open water. A line skimming the edge of a sea hex skims
// open water. A single fixed nudge would settle every edge tie toward the same
// compass side of the map, so a coast running one way would block views that the
// mirrored coast lets through; checking both sides has no such bias. (The rule is
// symmetric, from→to equals to→from, either way: both endpoints get the same
// nudge, so the drawn line does not depend on its direction.)
func SeaSightline(from, to MapPosition, isSea func(MapPosition) bool) bool {
	if !isSea(to) {
		return false
	}
	for _, side := range [2]float64{1, -1} {
		clear := true
		for _, p := range hexLine(from, to, side) {
			if p == from || p == to {
				continue
			}
			if !isSea(p) {
				clear = false
				break
			}
		}
		if clear {
			return true
		}
	}
	return false
}

// hexLine returns the hexes on the straight line from a to b, both included,
// sampled at N = HexDistance(a,b) equal steps in cube space and rounded to the
// nearest hex (the standard cube-lerp line). Both endpoints are shifted by a tiny
// nudge — side = +1 or -1 picks its direction — so a sample that falls exactly on a
// hex edge rounds to one consistent side instead of by float accident. The nudge
// (1,2,-3)·ε in cube (q,s,r) is orthogonal to none of the three hex axes, so it
// breaks every edge tie.
func hexLine(a, b MapPosition, side float64) []MapPosition {
	const eps = 1e-6
	dq, ds, dr := side*eps, side*2*eps, -side*3*eps
	aq, ar := float64(a.Q)+dq, float64(a.R)+dr
	bq, br := float64(b.Q)+dq, float64(b.R)+dr
	as, bs := -float64(a.Q)-float64(a.R)+ds, -float64(b.Q)-float64(b.R)+ds
	n := HexDistance(a, b)
	out := make([]MapPosition, 0, n+1)
	for i := 0; i <= n; i++ {
		t := 0.0
		if n > 0 {
			t = float64(i) / float64(n)
		}
		out = append(out, cubeRound(aq+(bq-aq)*t, as+(bs-as)*t, ar+(br-ar)*t))
	}
	return out
}

// cubeRound rounds fractional cube coordinates (q, s = -q-r, r) to the nearest
// hex: round each, then recompute the one with the largest rounding error so the
// three still sum to zero.
func cubeRound(q, s, r float64) MapPosition {
	rq, rs, rr := math.Round(q), math.Round(s), math.Round(r)
	dq, ds, dr := math.Abs(rq-q), math.Abs(rs-s), math.Abs(rr-r)
	switch {
	case dq > ds && dq > dr:
		rq = -rs - rr
	case ds > dr:
		// s is the worst — it is implied by q and r, which are returned as-is.
	default:
		rr = -rq - rs
	}
	return MapPosition{Q: int(rq), R: int(rr)}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// compassSectors are the 8 directions FuzzyBearing buckets a bearing angle
// into, ordered by increasing angle (radians, standard atan2 convention:
// 0 = +x axis, counter-clockwise).
var compassSectors = [8]string{"E", "NE", "N", "NW", "W", "SW", "S", "SE"}

// FuzzyBearing describes target's approximate position relative to landmark as
// a coarse compass direction + a fuzzed distance, e.g. "~5 hexes E" — used for
// rumour-known settlements (temenos_gossip.md PASS 2b), which must never
// expose exact (q,r). The caller appends the landmark's name ("... of Byblos");
// FuzzyBearing itself only sees positions, not names.
//
// Axial deltas are converted to a cartesian vector (x = dq + dr/2,
// y = dr·√3/2) before bucketing into 8 sectors — a continuous angle describes
// direction more naturally than the hex grid's 6-neighbour geometry would.
func FuzzyBearing(target, landmark MapPosition) string {
	dist := HexDistance(target, landmark)
	if dist == 0 {
		return "right at the landmark"
	}

	dq := float64(target.Q - landmark.Q)
	dr := float64(target.R - landmark.R)
	x := dq + dr/2
	y := dr * math.Sqrt(3) / 2

	angle := math.Atan2(y, x)
	if angle < 0 {
		angle += 2 * math.Pi
	}
	sector := int(math.Round(angle/(math.Pi/4))) % 8
	dir := compassSectors[sector]

	return fmt.Sprintf("~%d hexes %s", fuzzDistance(dist), dir)
}

// fuzzDistance rounds an exact hex distance to the nearest 5 (minimum 5) so a
// rumour never reads as a precise measurement. Tunable bucket size, not an
// invariant — see temenos_gossip.md PASS 2b.
func fuzzDistance(dist int) int {
	rounded := ((dist + 2) / 5) * 5
	if rounded < 5 {
		rounded = 5
	}
	return rounded
}
