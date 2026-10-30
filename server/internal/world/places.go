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
	case "river", "massif", "desert", "forest", "lake":
		return true
	}
	return false
}
func fmtAnchor(a [2]int) string { return fmt.Sprintf("%d,%d", a[0], a[1]) }
