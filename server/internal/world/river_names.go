package world

import (
	"fmt"
	"sort"
)

var riverDirs = [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}

func riverNeighbours(c cell) []cell {
	out := make([]cell, 0, 6)
	for _, d := range riverDirs {
		out = append(out, cell{c.q + d[0], c.r + d[1]})
	}
	return out
}
func cellLess(a, b cell) bool { return a.q < b.q || a.q == b.q && a.r < b.r }

// riverComponents is the shared connected-component owner for map metrics
// and naming. Sorting the input and each component makes SQL row order irrelevant.
func riverComponents(tiles []MapTile) [][]cell {
	return terrainComponents(tiles, func(t Terrain) bool { return t == TerrainRiver || t == TerrainRiverFord })
}

func terrainComponents(tiles []MapTile, accepts func(Terrain) bool) [][]cell {
	remaining := map[cell]bool{}
	for _, t := range tiles {
		if accepts(t.Terrain) {
			remaining[cell{t.Q, t.R}] = true
		}
	}
	starts := make([]cell, 0, len(remaining))
	for c := range remaining {
		starts = append(starts, c)
	}
	sort.Slice(starts, func(i, j int) bool { return cellLess(starts[i], starts[j]) })
	var out [][]cell
	for _, start := range starts {
		if !remaining[start] {
			continue
		}
		queue := []cell{start}
		delete(remaining, start)
		for i := 0; i < len(queue); i++ {
			for _, n := range riverNeighbours(queue[i]) {
				if remaining[n] {
					delete(remaining, n)
					queue = append(queue, n)
				}
			}
		}
		sort.Slice(queue, func(i, j int) bool { return cellLess(queue[i], queue[j]) })
		out = append(out, queue)
	}
	return out
}

// RiverAnchors identifies immutable river+ford chains by their smallest (q,r).
// Delta islands inherit the first adjacent canonical chain. No names are generated.
func RiverAnchors(tiles []MapTile) map[[2]int]string {
	out := map[[2]int]string{}
	chains := riverComponents(tiles)
	for _, chain := range chains {
		anchor := chain[0]
		id := fmt.Sprintf("%d,%d", anchor.q, anchor.r)
		for _, c := range chain {
			out[[2]int{c.q, c.r}] = id
		}
	}

	// Connected delta islands inherit the adjacent chain. If an island touches
	// more than one chain, the lexicographically first canonical chain owns it.
	deltas := map[cell]bool{}
	for _, t := range tiles {
		if t.Terrain == TerrainRiverDelta {
			deltas[cell{t.Q, t.R}] = true
		}
	}
	for _, chain := range chains {
		name := out[[2]int{chain[0].q, chain[0].r}]
		queue := append([]cell(nil), chain...)
		for i := 0; i < len(queue); i++ {
			for _, n := range riverNeighbours(queue[i]) {
				if deltas[n] {
					delete(deltas, n)
					out[[2]int{n.q, n.r}] = name
					queue = append(queue, n)
				}
			}
		}
	}
	return out
}
