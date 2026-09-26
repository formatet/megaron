package province

import (
	"math"
	"time"
)

// What a watcher can read off a marching unit — and nothing more.
//
// Timothy 2026-09-26 (megaron_styrande_beslut.md, megaron_plan_karavanbeslag.md
// slice 7): a foreign march shows its DIRECTION, never its destination, on every
// surface. "Riktningen kan ju vara fel om enheten t ex måste gå runt berg" — the
// watcher sees which way it goes now. And for the asynchronicity gate: if it
// SEEMS to be heading for part of your city's catchment, you are told when it
// would arrive IF that is where it is going. Both answers are computed here,
// once, so /foreign-units, the sighting notice and keryx can never disagree.

// compassWords is the 8-point compass in counter-clockwise order from east.
var compassWords = [8]string{"east", "north-east", "north", "north-west", "west", "south-west", "south", "south-east"}

// ScreenCompass is the 8-point compass direction from one hex to another AS DRAWN
// on the map: flat-top hexes, x = 1.5·q, y = √3·(r + q/2), y growing downward
// (web/static/js/megaron/render/map.js hexPx). The web's compassFromPixels
// (ui/hover.js) is the same function on pixels. "" for the same hex.
//
// Not FuzzyBearing's geometry: that one treats +r as up-right, so a step that is
// due south on screen reads "NE" there (megaron_todo, measured 2026-09-26).
func ScreenCompass(from, to MapPosition) string {
	dq := float64(to.Q - from.Q)
	dr := float64(to.R - from.R)
	if dq == 0 && dr == 0 {
		return ""
	}
	x := 1.5 * dq
	y := -math.Sqrt(3) * (dr + dq/2) // screen y down → compass y up
	angle := math.Atan2(y, x)
	if angle < 0 {
		angle += 2 * math.Pi
	}
	return compassWords[int(math.Round(angle/(math.Pi/4)))%8]
}

// ApparentMarch is a marching unit as a watcher sees it.
type ApparentMarch struct {
	Pos     MapPosition // where it is now — InterpolateAlongPath's hex
	Heading string      // which way its current step goes (ScreenCompass)
	// stepsDone is the fractional number of path steps walked; stepDur the time
	// one step takes. Together they turn "N hexes further" into a time.
	stepsDone float64
	stepDur   time.Duration
	departsAt time.Time
}

// ReadMarch resolves a march on path to what a watcher sees at now. ok=false
// for a path too short to have a direction.
func ReadMarch(path []MapPosition, departsAt, arrivesAt, now time.Time) (ApparentMarch, bool) {
	if len(path) < 2 {
		return ApparentMarch{}, false
	}
	steps := len(path) - 1
	total := arrivesAt.Sub(departsAt)
	progress := 0.0
	if total > 0 {
		progress = math.Min(1, math.Max(0, float64(now.Sub(departsAt))/float64(total)))
	}
	pos := InterpolateAlongPath(now, departsAt, arrivesAt, path)
	// The step being walked: from the hex it stands on to the next one — or, on
	// the last hex, the step that brought it there.
	idx := int(math.Round(progress * float64(steps)))
	if idx >= steps {
		idx = steps - 1
	}
	return ApparentMarch{
		Pos:       pos,
		Heading:   ScreenCompass(path[idx], path[idx+1]),
		stepsDone: progress * float64(steps),
		stepDur:   total / time.Duration(steps),
		departsAt: departsAt,
	}, true
}

// SeemsBoundFor reports whether the march appears headed into catchment: some
// hex of it lies in the march's heading as seen from where it stands (or it is
// already inside). dist is the hex distance to the nearest such hex. The answer
// is about appearance only — a road that bends round a mountain fools it, by
// design.
func (m ApparentMarch) SeemsBoundFor(catchment []MapPosition) (dist int, ok bool) {
	best := -1
	for _, c := range catchment {
		if c == m.Pos {
			return 0, true
		}
		if ScreenCompass(m.Pos, c) != m.Heading {
			continue
		}
		if d := HexDistance(m.Pos, c); best < 0 || d < best {
			best = d
		}
	}
	return best, best >= 0
}

// ArrivalIf is when the march would reach a hex dist steps ahead at its own
// pace — "if that is where it is going". Never its real arrival.
func (m ApparentMarch) ArrivalIf(dist int) time.Time {
	return m.departsAt.Add(time.Duration((m.stepsDone + float64(dist)) * float64(m.stepDur)))
}

// ArrivalTickIf is ArrivalIf in world ticks, scaled off the march's own
// depart/arrive ticks over the same path.
func (m ApparentMarch) ArrivalTickIf(dist, departTick, arriveTick, pathSteps int) int {
	if pathSteps <= 0 {
		return arriveTick
	}
	perStep := float64(arriveTick-departTick) / float64(pathSteps)
	return departTick + int(math.Ceil((m.stepsDone+float64(dist))*perStep))
}
