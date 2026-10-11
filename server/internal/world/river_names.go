package world

import (
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"sort"
	"strings"
)

//go:embed river_names.csv
var riverNamesCSV string

var riverNamePool = func() []string {
	records, err := csv.NewReader(strings.NewReader(riverNamesCSV)).ReadAll()
	if err != nil {
		panic(err)
	}
	names := make([]string, 0, len(records)-1)
	seen := map[string]bool{}
	for _, row := range records[1:] {
		if len(row) != 1 || row[0] == "" || seen[row[0]] {
			panic("invalid or duplicate river name")
		}
		seen[row[0]] = true
		names = append(names, row[0])
	}
	if len(names) == 0 {
		panic("empty river names")
	}
	return names
}()

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
	remaining := map[cell]bool{}
	for _, t := range tiles {
		if t.Terrain == TerrainRiver || t.Terrain == TerrainRiverFord {
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

// RiverNames derives names from a complete immutable map, never a player's
// partial FOW view. No persistence, process-local seed or generation rerun.
// Canonical chains claim a hash-selected unused name in canonical order.
// If the editable pool is exhausted, numbered names remain unique.
func RiverNames(worldID string, tiles []MapTile) map[[2]int]string {
	out := map[[2]int]string{}
	chains := riverComponents(tiles)
	used := map[string]bool{}
	for _, chain := range chains {
		anchor := chain[0]
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", worldID, anchor.q, anchor.r)))
		pick := int(binary.BigEndian.Uint64(sum[:8]) % uint64(len(riverNamePool)))
		name := ""
		for i := 0; i < len(riverNamePool); i++ {
			candidate := riverNamePool[(pick+i)%len(riverNamePool)]
			if !used[candidate] {
				name = candidate
				break
			}
		}
		if name == "" {
			for n := 2; name == ""; n++ {
				candidate := fmt.Sprintf("%s %d", riverNamePool[pick], n)
				if !used[candidate] {
					name = candidate
				}
			}
		}
		used[name] = true
		for _, c := range chain {
			out[[2]int{c.q, c.r}] = name
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
