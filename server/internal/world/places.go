package world

import (
	"fmt"
	"sort"
)

// Place is a static connected geographic object. The anchor is canonical,
// independent of DB row order, names, and the observer's partial FOW map.
type Place struct {
	Kind   string
	Anchor [2]int
	Hexes  [][2]int
}

// Places owns every nameable terrain component. Rivers and lakes have no
// minimum; mixed limestone/red mountains, semi-desert and cedar forest need five hexes.
// Olive groves are cultivated land and never nameable forest.
func Places(tiles []MapTile) []Place {
	var out []Place
	anchors := RiverAnchors(tiles)
	byID := map[string]*Place{}
	for _, chain := range riverComponents(tiles) {
		a := chain[0]
		p := Place{Kind: "river", Anchor: [2]int{a.q, a.r}}
		out = append(out, p)
	}
	for i := range out {
		a := out[i].Anchor
		byID[fmtAnchor(a)] = &out[i]
	}
	for h, id := range anchors {
		byID[id].Hexes = append(byID[id].Hexes, h)
	}
	specs := []struct {
		kind   string
		min    int
		accept func(Terrain) bool
	}{
		{"massif", 5, func(t Terrain) bool { return t == TerrainMountainLimestone || t == TerrainMountainRed }},
		{"desert", 5, func(t Terrain) bool { return t == TerrainSemiDesert }},
		{"forest", 5, func(t Terrain) bool { return t == TerrainForestCedar }},
		{"lake", 1, func(t Terrain) bool { return t == TerrainLake }},
		// Terrain-shaped kinds added by platstyper P1 (Timothy 2026-10-31). They overlap
		// the kinds above; a map tile shows the smallest place it belongs to.
		{"plain", PlainMin, func(t Terrain) bool { return t == TerrainPlains }},
		{"hills", HillsMin, func(t Terrain) bool { return t == TerrainHills }},
		{"delta", DeltaMin, func(t Terrain) bool { return t == TerrainRiverDelta }},
	}
	for _, s := range specs {
		for _, chain := range terrainComponents(tiles, s.accept) {
			if len(chain) < s.min {
				continue
			}
			p := Place{Kind: s.kind, Anchor: [2]int{chain[0].q, chain[0].r}}
			for _, h := range chain {
				p.Hexes = append(p.Hexes, [2]int{h.q, h.r})
			}
			out = append(out, p)
		}
	}
	out = append(out, shapePlaces(tiles)...)
	for i := range out {
		sort.Slice(out[i].Hexes, func(a, b int) bool {
			x, y := out[i].Hexes[a], out[i].Hexes[b]
			return x[0] < y[0] || x[0] == y[0] && x[1] < y[1]
		})
	}
	return out
}

// IsPlaceKind reports whether kind names a Places kind.
func IsPlaceKind(kind string) bool {
	switch kind {
	case "river", "massif", "desert", "forest", "lake", "island", "strait", "bay", "cape", "plain", "hills", "delta":
		return true
	}
	return false
}
func fmtAnchor(a [2]int) string { return fmt.Sprintf("%d,%d", a[0], a[1]) }

// Minimum component sizes of the terrain-shaped kinds, measured on 64x64 worlds
// (docs/reviews/platstyper-p1/README.md).
const (
	PlainMin = 7
	HillsMin = 3
	DeltaMin = 1
)

// StraitMaxWidth (W) is the widest gap of open water, in hexes, that still counts
// as a strait (Timothy 2026-11-02: build with 3, adjust after seeing the live map).
// BayMinLand is how many of a hex's six sides must be land for coastal sea to be a
// bay. DUPLICATES the fishing bay rule in economy.rulesWithLandNeighbors
// (server/internal/economy/hex_rules.go): world has no internal deps and cannot
// import economy; economy's TestBayParityWithWorld keeps the two equal.
const (
	StraitMaxWidth = 3
	BayMinLand     = 4
)

// straitHexes marks every open-sea hex whose hex distance (through water) to its two
// nearest DIFFERENT landmasses sums to at most StraitMaxWidth+1. A gap of W hexes
// gives every hex on a straight line across it the sum W+1. The result may include
// lake hexes the BFS passed through; the caller keeps only open sea.
func straitHexes(terrain map[cell]Terrain, land [][]cell) map[cell]bool {
	const maxDist = StraitMaxWidth + 1
	// dist[c] holds, per landmass reaching c, the distance (BFS through sea, capped).
	dist := map[cell][]int{}
	for _, comp := range land {
		seen := map[cell]int{}
		front := make([]cell, 0, len(comp))
		for _, h := range comp {
			seen[h] = 0
			front = append(front, h)
		}
		for d := 1; d <= maxDist && len(front) > 0; d++ {
			var next []cell
			for _, x := range front {
				for _, y := range riverNeighbours(x) {
					t, ok := terrain[y]
					if _, done := seen[y]; done || !ok || !isSea(t) {
						continue
					}
					seen[y] = d
					next = append(next, y)
					dist[y] = append(dist[y], d)
				}
			}
			front = next
		}
	}
	out := map[cell]bool{}
	for c, ds := range dist {
		if len(ds) < 2 {
			continue
		}
		sort.Ints(ds)
		if ds[0]+ds[1] <= StraitMaxWidth+1 {
			out[c] = true
		}
	}
	return out
}

// shapePlaces finds the kinds defined by the shape of land against water:
// islands (a landmass wholly inside the map that is not the biggest), straits
// (open water within W hexes of two different landmasses), bays (coastal sea with
// land on four or more sides) and capes (landmass hexes with four or
// more sea neighbours). Land is anything that is not sea; rivers and fords are land.
func shapePlaces(tiles []MapTile) []Place {
	terrain := make(map[cell]Terrain, len(tiles))
	for _, t := range tiles {
		terrain[cell{t.Q, t.R}] = t.Terrain
	}
	var out []Place
	add := func(kind string, chain []cell) {
		p := Place{Kind: kind, Anchor: [2]int{chain[0].q, chain[0].r}}
		for _, h := range chain {
			p.Hexes = append(p.Hexes, [2]int{h.q, h.r})
		}
		out = append(out, p)
	}
	land := terrainComponents(tiles, func(t Terrain) bool { return !isSea(t) })
	biggest := -1
	for i, c := range land {
		if biggest < 0 || len(c) > len(land[biggest]) {
			biggest = i
		}
	}
	landSize := map[cell]int{}
	for i, c := range land {
		inside := i != biggest
		for _, h := range c {
			landSize[h] = len(c)
			for _, n := range riverNeighbours(h) {
				if _, ok := terrain[n]; !ok {
					inside = false
				}
			}
		}
		if inside {
			add("island", c)
		}
	}
	isOpenSea := func(t Terrain) bool { return t == TerrainCoastalSea || t == TerrainDeepSea }
	landNeighbours := func(c cell) int {
		n := 0
		for _, nb := range riverNeighbours(c) {
			if t, ok := terrain[nb]; ok && !isSea(t) {
				n++
			}
		}
		return n
	}
	straitHex := straitHexes(terrain, land)
	var strait, bay, cape []MapTile
	for _, t := range tiles {
		c := cell{t.Q, t.R}
		if isOpenSea(t.Terrain) {
			if straitHex[c] {
				// A bay inside a strait is strait water: the strait wins.
				strait = append(strait, t)
			} else if t.Terrain == TerrainCoastalSea && landNeighbours(c) >= BayMinLand {
				bay = append(bay, t)
			}
		} else if !isSea(t.Terrain) && landSize[c] >= 3 {
			n := 0
			for _, nb := range riverNeighbours(c) {
				if isOpenSea(terrain[nb]) {
					n++
				}
			}
			if n >= 4 {
				cape = append(cape, t)
			}
		}
	}
	all := func(Terrain) bool { return true }
	for _, chain := range terrainComponents(strait, all) {
		add("strait", chain)
	}
	for _, chain := range terrainComponents(bay, all) {
		add("bay", chain)
	}
	for _, chain := range terrainComponents(cape, all) {
		add("cape", chain)
	}
	return out
}
