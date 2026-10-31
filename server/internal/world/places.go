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
	case "river", "massif", "desert", "forest", "lake", "island", "strait", "cape", "plain", "hills", "delta":
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

// shapePlaces finds the kinds defined by the shape of land against water:
// islands (a landmass wholly inside the map that is not the biggest), straits
// (sea with land on two opposite sides) and capes (landmass hexes with four or
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
	isLand := func(c cell) bool { t, ok := terrain[c]; return ok && !isSea(t) }
	var strait, cape []MapTile
	for _, t := range tiles {
		c := cell{t.Q, t.R}
		if isOpenSea(t.Terrain) {
			for _, d := range [][2]cell{{{1, 0}, {-1, 0}}, {{0, 1}, {0, -1}}, {{1, -1}, {-1, 1}}} {
				if isLand(cell{c.q + d[0].q, c.r + d[0].r}) && isLand(cell{c.q + d[1].q, c.r + d[1].r}) {
					strait = append(strait, t)
					break
				}
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
	for _, chain := range terrainComponents(cape, all) {
		add("cape", chain)
	}
	return out
}
