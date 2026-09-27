// Package movement is the shared answer to "where is an actor, and when is
// it in which hex" for everything that moves (messengers, armies, ships,
// caravans). Zero internal dependencies (G1 tier) — standard library only.
//
// It implements R1-R3 of megaron_rorelseregler.md (the source of truth for
// these rules): time follows terrain hex-by-hex (R1), one rounding to the
// nearest whole tick followed by exact sub-tick boundaries (R2), and contact
// means overlapping presence in the same hex (R3). This package only answers
// "where/when" — it does not decide what happens on contact (that is R4-R8,
// slice 3), and it builds no Move from terrain (that is slice 2, A*).
//
// Nothing else in the repository may depend on this package yet: it has no
// consumer. It is a pure substrate, proven by tests and fuzzing, not by eye.
package movement

import (
	"fmt"
	"math/big"
	"sort"
)

// Hex is an axial hex coordinate. It intentionally does not reuse
// hexgrid.Coord: this package has zero internal dependencies.
type Hex struct{ Q, R int }

// Milli is a duration or point in time measured in thousandths of a game
// tick. Milli(1000) is one tick. This package never touches time.Time or a
// clock — callers pass ticks in, and everything downstream is integer math.
type Milli int64

// Move is a decided journey. Hexes[0] is the start hex. Costs[i] is the cost
// of stepping INTO Hexes[i+1] (R1: the start hex itself costs nothing — the
// classic pathfind.go rule of summing cost over path[1:]). That cost IS the
// time spent standing in Hexes[i+1] (R1, "val A" — see the comment above
// PositionAt for exactly which hex owns which interval). Costs are an
// integer scale chosen by the caller (e.g. thousandths of
// TerrainMoveTicks); this package only cares about their ratios.
type Move struct {
	StartTick, EndTick int     // whole ticks; EndTick > StartTick. The rounding to a whole tick (R2) is already done by the caller.
	Hexes              []Hex   // at least two
	Costs              []int64 // len == len(Hexes)-1, all > 0
}

// Interval is a hex occupancy window, half-open: [In, Out).
type Interval struct {
	Hex     Hex
	In, Out Milli
}

// millisPerTick is how many Milli units make up one whole tick.
const millisPerTick = 1000

// Validate checks the structural shape of m and the R2 overflow guard: the
// product D*T (total duration in Milli times total cost) must fit in int64,
// since Boundaries multiplies them together for every prefix sum.
func (m Move) Validate() error {
	if len(m.Hexes) < 2 {
		return fmt.Errorf("movement: Move needs at least two hexes, got %d", len(m.Hexes))
	}
	if len(m.Costs) != len(m.Hexes)-1 {
		return fmt.Errorf("movement: len(Costs) = %d, want %d (len(Hexes)-1)", len(m.Costs), len(m.Hexes)-1)
	}
	if m.EndTick <= m.StartTick {
		return fmt.Errorf("movement: EndTick %d must be greater than StartTick %d", m.EndTick, m.StartTick)
	}
	var total int64
	for i, c := range m.Costs {
		if c <= 0 {
			return fmt.Errorf("movement: Costs[%d] = %d, must be > 0", i, c)
		}
		total += c
	}

	// D*T overflow guard (plan: "vägrar Validate om D*T inte ryms i int64").
	// Computed with big.Int since this runs once per Move, not in a hot loop.
	d := new(big.Int).Mul(big.NewInt(int64(m.EndTick-m.StartTick)), big.NewInt(millisPerTick))
	product := new(big.Int).Mul(d, big.NewInt(total))
	if !product.IsInt64() {
		return fmt.Errorf("movement: D*T overflows int64 (D=%s, T=%d)", d.String(), total)
	}
	return nil
}

// Boundaries returns the b_i from R2: the exact Milli at which the actor
// crosses into Hexes[i]. len(result) == len(m.Hexes); result[0] is
// StartTick*1000 exactly and result[len(Hexes)-1] is EndTick*1000 exactly.
//
// Formula: D = (EndTick-StartTick)*1000, T = sum(Costs), P_i = prefix sum of
// the first i costs, b_i = StartTick*1000 + (D*P_i + T/2) / T — integer
// division, i.e. round-half-up. b_n is exact because floor(T/2) < T, so the
// remainder term always divides away to zero.
func Boundaries(m Move) ([]Milli, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	var total int64
	for _, c := range m.Costs {
		total += c
	}
	d := int64(m.EndTick-m.StartTick) * millisPerTick
	start := int64(m.StartTick) * millisPerTick
	half := total / 2

	bounds := make([]Milli, len(m.Hexes))
	bounds[0] = Milli(start)
	var prefix int64
	for i, c := range m.Costs {
		prefix += c
		bounds[i+1] = Milli(start + (d*prefix+half)/total)
	}
	return bounds, nil
}

// Which hex owns which interval (Timothy 2026-09-27, "val A", corrected the
// same day the first version of this file paired a hex with the cost of the
// NEXT hex — see git history): the cost of stepping into a hex is the time
// spent standing in it (R1: "saktar in i vadstället"), so the boundaries
// bound the PREVIOUS hex, not the next one.
//
//   - Hexes[0] (start) occupies (-inf, b_0) — everything before departure.
//     It is left at b_0.
//   - Hexes[i], 1 <= i <= n-1, occupies [b_{i-1}, b_i).
//   - Hexes[n] (target) occupies [b_{n-1}, +inf) — the actor has already
//     reached it before the journey's end; b_n = EndTick*1000 is only the
//     EXECUTED arrival (R4's "reached" vs "executed"), not a hex boundary.
//
// Equivalently: of the n+1 boundaries in Boundaries' result, only the first
// n (b_0..b_{n-1}) are edges between hexes; b_n never bounds an interval.

// PositionAt returns the hex containing t: before b_0, the start hex;
// otherwise the hex whose interval (see above) contains t. Each hex's
// window is half-open, so at an exact boundary b_i (i<n) the actor has
// already moved on to Hexes[i+1].
func PositionAt(m Move, t Milli) (Hex, error) {
	bounds, err := Boundaries(m)
	if err != nil {
		return Hex{}, err
	}
	edges := bounds[:len(bounds)-1] // b_0..b_{n-1}; b_n is the executed arrival, not an edge
	j := sort.Search(len(edges), func(i int) bool { return edges[i] > t })
	return m.Hexes[j], nil
}

// Presence returns the hex intervals that overlap the half-open window
// [from, to), clipped to it. Hexes[0]'s window is unbounded below and
// Hexes[n]'s is unbounded above; Presence clips both to the query window.
// A hex whose true window has zero length (two boundaries coincide, e.g. a
// very cheap hex on a very short trip, R2) never appears: it overlaps
// nothing, by construction.
func Presence(m Move, from, to Milli) ([]Interval, error) {
	bounds, err := Boundaries(m)
	if err != nil {
		return nil, err
	}
	if to <= from {
		return nil, nil
	}
	edges := bounds[:len(bounds)-1]
	last := len(m.Hexes) - 1
	var out []Interval
	for i, hex := range m.Hexes {
		in := from // Hexes[0]: unbounded below, clipped to the window
		if i > 0 {
			in = edges[i-1]
		}
		end := to // Hexes[last]: unbounded above, clipped to the window
		if i < last {
			end = edges[i]
		}
		clippedIn, clippedOut := in, end
		if clippedIn < from {
			clippedIn = from
		}
		if clippedOut > to {
			clippedOut = to
		}
		if clippedIn < clippedOut {
			out = append(out, Interval{Hex: hex, In: clippedIn, Out: clippedOut})
		}
	}
	return out, nil
}

// FirstContact finds the earliest R3 contact between two actors, given as
// their hex-by-hex presence (as returned by Presence — a contiguous,
// time-ordered interval list, i.e. consecutive entries share a boundary:
// a[i].Out == a[i+1].In).
//
// Two contact shapes are checked:
//  1. Overlap: intervals of a and b in the same hex overlap. Contact time is
//     max(a.In, b.In).
//  2. Exact opposite-direction swap: a leaves hex X for Y at time t while b
//     leaves Y for X at the same t. Neither interval overlaps the other in
//     the half-open model (they touch, not overlap), so this is checked
//     separately, from adjacent pairs in each list. This also catches a
//     swap that passes through a zero-length hex, since dropping a
//     zero-length hex from the interval list does not break adjacency: the
//     hex before and after it still share that hex's (coincident) boundary.
//
// Returns the earliest contact; ties broken by the lowest (Q, R) so the
// result is deterministic.
func FirstContact(a, b []Interval) (hex Hex, at Milli, ok bool) {
	type candidate struct {
		hex Hex
		at  Milli
	}
	var candidates []candidate

	byHex := make(map[Hex][]Interval, len(b))
	for _, bi := range b {
		byHex[bi.Hex] = append(byHex[bi.Hex], bi)
	}
	for _, ai := range a {
		for _, bi := range byHex[ai.Hex] {
			if ai.In < bi.Out && bi.In < ai.Out {
				t := ai.In
				if bi.In > t {
					t = bi.In
				}
				candidates = append(candidates, candidate{ai.Hex, t})
			}
		}
	}

	for i := 0; i+1 < len(a); i++ {
		if a[i].Out != a[i+1].In {
			continue // not a contiguous transition in this list
		}
		t := a[i].Out
		x, y := a[i].Hex, a[i+1].Hex
		for j := 0; j+1 < len(b); j++ {
			if b[j].Out != t || b[j+1].In != t {
				continue
			}
			if b[j].Hex == y && b[j+1].Hex == x {
				candidates = append(candidates, candidate{x, t}) // contact in the hex a leaves
			}
		}
	}

	if len(candidates) == 0 {
		return Hex{}, 0, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		ci, cj := candidates[i], candidates[j]
		if ci.at != cj.at {
			return ci.at < cj.at
		}
		if ci.hex.Q != cj.hex.Q {
			return ci.hex.Q < cj.hex.Q
		}
		return ci.hex.R < cj.hex.R
	})
	best := candidates[0]
	return best.hex, best.at, true
}
