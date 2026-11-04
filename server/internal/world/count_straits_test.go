package world

import "testing"

// straitTestMap: plains columns at the given q values, deep sea elsewhere, on a 14x8 axial grid.
func straitTestMap(land func(q, r int) bool) []MapTile {
	var tiles []MapTile
	for q := 0; q < 14; q++ {
		for r := 0; r < 8; r++ {
			t := TerrainDeepSea
			if land(q, r) {
				t = TerrainPlains
			}
			tiles = append(tiles, MapTile{Q: q, R: r, Terrain: t})
		}
	}
	return tiles
}

// The generator's strait floor counts the strait place (water between two landmasses
// within StraitMaxWidth), not the old opposing-axis rule that counted narrow bays.
func TestCountStraits_SamePredicateAsStraitPlace(t *testing.T) {
	narrow := straitTestMap(func(q, _ int) bool { return q <= 3 || q >= 4+StraitMaxWidth })
	if n := countStraits(narrow); n == 0 {
		t.Fatalf("gap of %d between two landmasses: want strait hexes, got 0", StraitMaxWidth)
	}
	wide := straitTestMap(func(q, _ int) bool { return q <= 3 || q >= 5+StraitMaxWidth })
	if n := countStraits(wide); n != 0 {
		t.Fatalf("gap of %d: want 0 strait hexes, got %d", StraitMaxWidth+1, n)
	}
	// One landmass with a one-hex inlet: the old rule called it a strait.
	inlet := straitTestMap(func(q, r int) bool { return q <= 6 && !(q == 6 && r == 4) })
	if n := countStraits(inlet); n != 0 {
		t.Fatalf("inlet in one landmass: want 0 strait hexes, got %d", n)
	}
}
