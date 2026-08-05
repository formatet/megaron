package province

import (
	"fmt"
	"math"
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

// axialNeighborDirs lists the 6 axial hex neighbour offsets (shared with pathfind.go's
// axialDirs; kept local to avoid import-order coupling within the same package).
var axialNeighborDirs = [6][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}

// HexNeighbors returns the 6 axial neighbours of pos.
func HexNeighbors(pos MapPosition) [6]MapPosition {
	var out [6]MapPosition
	for i, d := range axialNeighborDirs {
		out[i] = MapPosition{Q: pos.Q + d[0], R: pos.R + d[1]}
	}
	return out
}

// Eye kinds for live-vision sources (temenos_synlighet.md tier 1).
const (
	EyeSettlement = "settlement"
	EyeLandUnit   = "land-unit"
	// EyeNomadicHost is the founder-phase host: a people on the move, not a scout.
	EyeNomadicHost = "nomadic-host"
	EyeShip       = "ship"
)

// Eye is a live vision source (a settlement or a positioned/marching unit) at a
// map position.
type Eye struct {
	Pos  MapPosition
	Kind string // EyeSettlement | EyeLandUnit | EyeNomadicHost | EyeShip
	// AtWater is true when the eye itself stands at open water: on a sea hex, or
	// on a land hex with a sea neighbour (a coastal city, a unit on the shore).
	// Only such an eye gets the open horizon — see LiveRadius. Set by LoadLiveEyes
	// from the map; the zero value (false = inland) is the fail-closed default, so
	// a hand-built or unclassified Eye can only ever see LESS, never more.
	AtWater bool
}

// LiveRadius returns the live-vision radius for an eye of eyeKind looking at a tile
// of targetTerrain. Sea hides nothing, but the open horizon belongs to whoever
// STANDS at the water — a ship, a coastal city, a unit on the shore (eyeAtWater).
// An eye inland reads the sea at its ordinary land vantage.
// Land limits vision to the eye's own vantage (settlement 3 / land-unit 2 / ship 1),
// except mountains, which are landmarks visible +2 hexes further regardless of eye.
//
// The eyeAtWater condition is new on 2026-08-05. Until then the sea branch returned
// 4 before the eye was even read, so an army deep inland saw every sea hex within 4
// (Timothy 2026-08-04: "där har vi ett designfel idag"). temenos_synlighet.md's
// "sea = 4 for all eyes" is amended, not repealed: it is 4 for all eyes AT the water.
func LiveRadius(eyeKind string, eyeAtWater bool, targetTerrain string) int {
	// A naval unit floats on the sea by definition and always carries the open
	// horizon. This is a domain truth, not a fallback for a missing lookup — a
	// ship built by hand in a test must not read as inland.
	if eyeKind == EyeShip {
		eyeAtWater = true
	}
	if eyeAtWater && (targetTerrain == "coastal_sea" || targetTerrain == "deep_sea") {
		return 4
	}
	// Deliberately NOT river: the sea's radius 4 comes from an open horizon over
	// open water. A 1-hex-wide river between tall banks opens no horizon — it
	// falls through to the ordinary land vantage below (megaron_floden_plan.md
	// §5, Timothy 2026-07-29).
	base := 2
	switch eyeKind {
	case EyeSettlement:
		base = 3
	case EyeShip:
		base = 1
	case EyeLandUnit:
		base = 2
	case EyeNomadicHost:
		// Half a land unit's reach: a people on the move, not a scout. Only the
		// BASE is lowered — a host ON the coast still reads open sea at 4, and it
		// reads a mountain at 1+2 like anyone else (Timothy 2026-07-15). It is
		// short-sighted, not blind: what it cannot do is peer across ordinary ground.
		base = 1
	}
	if targetTerrain == "mountain_limestone" || targetTerrain == "mountain_red" {
		base += 2
	}
	return base
}

// AnyEyeSees returns true if target (of targetTerrain) is within live vision of any
// of the given eyes, using the per-eye-kind × per-target-terrain radius.
func AnyEyeSees(eyes []Eye, target MapPosition, targetTerrain string) bool {
	for _, e := range eyes {
		if HexDistance(e.Pos, target) <= LiveRadius(e.Kind, e.AtWater, targetTerrain) {
			return true
		}
	}
	return false
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
