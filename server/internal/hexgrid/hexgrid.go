// Package hexgrid is the single source of truth for axial hex-coordinate
// geometry (neighbor directions, disks/rings) shared across the codebase.
// Zero internal dependencies (G1 tier, alongside clock/gossip/notify) —
// economy and kharis may not import province, so this geometry primitive
// cannot live there; every package that needs it may import hexgrid.
//
// Before this package existed, the same six axial neighbor offsets were
// hand-copied into 13+ live call sites (SQL WHERE clauses and Go literals
// across economy/combat/handlers/province/kharis) plus ~15 tests — cataloged
// in megaron_todo.md, "7-hex-catchmentlistan är duplicerad". That todo item
// required consolidation to ONE exported helper before the catchment radius
// was ever touched (Timothy 2026-08-07): this package is that helper.
package hexgrid

// CatchmentRadius is a settlement's economic catchment: its own hex plus
// every hex within this many steps (megaron_plan_fysisk_gubbemodell.md P1,
// Timothy 2026-08-07 — "Radie 1→2: 19 hexar, 18 produktiva"). Lives here
// (not in economy, where the concept semantically belongs) because
// province/economy/combat/handlers all need the SAME value and province is
// zero-dep — hexgrid is the only tier every one of them can import.
//
// Changing this number is a balance decision with a full-economy blast
// radius (production potential scales with hex count) — never tune it
// without also redoing megaron_plan_foda_konsistens.md's grainPerCitizen and
// the herd-size calibration alongside it (same rebalancing pass, not two).
const CatchmentRadius = 2

// Coord is an axial hex coordinate (q, r), with the implicit third cube
// coordinate s = -q-r. This is the SAME convention every duplicated site used
// (q/r columns on map_tiles/provinces).
type Coord struct {
	Q, R int
}

// axialDirs are the six axial neighbor offsets, in the exact order every
// duplicated site historically listed them: (+1,0) (-1,0) (0,+1) (0,-1)
// (+1,-1) (-1,+1). Order is preserved for callers that build ordered SQL
// literals or tests asserting exact output — it carries no geometric meaning
// (a hex has no "first" neighbor), only continuity with existing code.
var axialDirs = [6]Coord{
	{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1},
}

// Neighbors returns the six hexes directly adjacent to c, in the fixed
// axialDirs order.
func Neighbors(c Coord) [6]Coord {
	var out [6]Coord
	for i, d := range axialDirs {
		out[i] = Coord{c.Q + d.Q, c.R + d.R}
	}
	return out
}

// Distance returns the hex distance (number of steps) between a and b on the
// axial grid — the standard cube-distance formula with s = -q-r implicit.
func Distance(a, b Coord) int {
	dq := a.Q - b.Q
	dr := a.R - b.R
	ds := -dq - dr
	return maxInt(absInt(dq), maxInt(absInt(dr), absInt(ds)))
}

// Disk returns every hex within radius steps of center, INCLUDING center
// itself — this is "catchment": radius 0 gives just the center (1 hex),
// radius 1 gives center + Neighbors (7 hexes, the pre-P1 catchment), radius 2
// gives 19 hexes (18 productive + center, [[megaron_plan_fysisk_gubbemodell]]
// P1). Order is deterministic (q ascending, then r ascending within q) but
// not otherwise meaningful — callers that need a stable center-first order
// should look for it at index matching center explicitly, not index 0.
func Disk(center Coord, radius int) []Coord {
	if radius < 0 {
		radius = 0
	}
	out := make([]Coord, 0, 1+3*radius*(radius+1))
	for dq := -radius; dq <= radius; dq++ {
		r1 := maxInt(-radius, -dq-radius)
		r2 := minInt(radius, -dq+radius)
		for dr := r1; dr <= r2; dr++ {
			out = append(out, Coord{center.Q + dq, center.R + dr})
		}
	}
	return out
}

// Ring returns every hex within radius steps of center, EXCLUDING center
// itself — the productive catchment (megaron_plan_fysisk_gubbemodell.md §3.2:
// the settlement's own hex is not a normal production hex, so every
// production-potential query wants the ring, not the disk). Equivalent to
// Disk(center, radius) with center removed, but does not allocate the
// discarded element.
func Ring(center Coord, radius int) []Coord {
	if radius < 0 {
		radius = 0
	}
	out := make([]Coord, 0, 3*radius*(radius+1))
	for dq := -radius; dq <= radius; dq++ {
		r1 := maxInt(-radius, -dq-radius)
		r2 := minInt(radius, -dq+radius)
		for dr := r1; dr <= r2; dr++ {
			if dq == 0 && dr == 0 {
				continue
			}
			out = append(out, Coord{center.Q + dq, center.R + dr})
		}
	}
	return out
}

// QRArrays splits a coord list into parallel q/r int32 slices — the shape
// every SQL call site needs to bind as two `int[]` params and pair back up
// with `unnest($n::int[], $m::int[]) AS t(q, r)` (a plain `q = ANY($n) AND r
// = ANY($m)` would match the cross-product, not paired coordinates — the bug
// this shape exists to avoid). int32 matches Postgres `int`/`int4` (map_tiles
// q/r), which pgx encodes directly without a driver-side conversion.
func QRArrays(coords []Coord) (qs, rs []int32) {
	qs = make([]int32, len(coords))
	rs = make([]int32, len(coords))
	for i, c := range coords {
		qs[i] = int32(c.Q)
		rs[i] = int32(c.R)
	}
	return qs, rs
}

// RingOrdinal returns c's 1-based position in Ring(center, radius)'s
// deterministic order, and whether c is in the ring at all. Ring's own order
// (q ascending, then r ascending within q) is exactly the address space P0-UI
// locked for the catchment raster and keryx (megaron_plan_fysisk_gubbemodell.md
// P0-UI answer 7: "adressrymden är hex-ordinal 1–18... samma ordning rastret
// numrerar") — a stored ordinal would drift the moment Ring's iteration order
// changed, so callers derive it on demand from the same function that
// produces the ring, rather than persisting a second copy of the numbering.
func RingOrdinal(center Coord, radius int, c Coord) (ordinal int, ok bool) {
	for i, rc := range Ring(center, radius) {
		if rc == c {
			return i + 1, true
		}
	}
	return 0, false
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
