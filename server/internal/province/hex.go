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
// This is the KNOWN-set gate (live ∪ remembered ∪ contacted) used by handlers that
// must not regress access when the tiered live-vision radii (LiveRadius) shrink —
// messenger Send and the Wanaxes directory gate on this, not on live eyes alone.
// See temenos_synlighet.md for the tiered-visibility model this sits alongside.
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
	EyeShip       = "ship"
)

// Eye is a live vision source (a settlement or a positioned/marching unit) at a
// map position.
type Eye struct {
	Pos  MapPosition
	Kind string // EyeSettlement | EyeLandUnit | EyeShip
}

// LiveRadius returns the live-vision radius for an eye of eyeKind looking at a tile
// of targetTerrain. Sea hides nothing, so every eye sees 4 hexes over open water.
// Land limits vision to the eye's own vantage (settlement 3 / land-unit 2 / ship 1),
// except mountains, which are landmarks visible +2 hexes further regardless of eye.
func LiveRadius(eyeKind string, targetTerrain string) int {
	if targetTerrain == "coastal_sea" || targetTerrain == "deep_sea" {
		return 4
	}
	base := 2
	switch eyeKind {
	case EyeSettlement:
		base = 3
	case EyeShip:
		base = 1
	case EyeLandUnit:
		base = 2
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
		if HexDistance(e.Pos, target) <= LiveRadius(e.Kind, targetTerrain) {
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
