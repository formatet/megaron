package world

import (
	"reflect"
	"testing"
)

func riverFlowFixture(changes map[[2]int]Terrain) []MapTile {
	var tiles []MapTile
	for q := 0; q < 9; q++ {
		for r := rowOrigin(q, 9); r < rowOrigin(q, 9)+9; r++ {
			terrain := TerrainPlains
			if q == 8 {
				terrain = TerrainCoastalSea
			}
			if v, ok := changes[[2]int{q, r}]; ok {
				terrain = v
			}
			tiles = append(tiles, MapTile{Q: q, R: r, Terrain: terrain})
		}
	}
	return tiles
}
func TestRiverFlowUsesSeaDeltaMouthNotSourceWater(t *testing.T) {
	changes := map[[2]int]Terrain{{1, 7}: TerrainCoastalSea, {7, 8}: TerrainRiverDelta}
	for q := 2; q <= 7; q++ {
		changes[[2]int{q, 7}] = TerrainRiver
	}
	changes[[2]int{4, 7}] = TerrainRiverFord
	for _, sourceOpen := range []bool{false, true} {
		if sourceOpen {
			changes[[2]int{0, 7}] = TerrainCoastalSea
		}
		flows := RiverFlows(riverFlowFixture(changes), 9, 9)
		for q := 2; q <= 7; q++ {
			f := flows[[2]int{q, 7}]
			wantDown := []int{0}
			if q == 7 {
				wantDown = []int{0, 4}
			}
			if !f.Known || !reflect.DeepEqual(f.Upstream, []int{1}) || !reflect.DeepEqual(f.Downstream, wantDown) {
				t.Errorf("q%d sourceOpen%v flow=%+v want upstream1/downstream0", q, sourceOpen, f)
			}
		}
	}
}
func TestRiverFlowTreeForkPointsTowardBothMouths(t *testing.T) {
	changes := map[[2]int]Terrain{{1, 7}: TerrainCoastalSea, {7, 5}: TerrainRiverDelta, {3, 10}: TerrainRiverDelta, {4, 10}: TerrainCoastalSea}
	for _, xy := range [][2]int{{2, 7}, {3, 7}, {4, 7}, {5, 6}, {6, 5}, {7, 4}, {4, 8}, {4, 9}} {
		changes[xy] = TerrainRiver
	}
	flows := RiverFlows(riverFlowFixture(changes), 9, 9)
	for xy, f := range flows {
		if !f.Known {
			t.Errorf("tree fork%v unknown", xy)
		}
	}
	fork := flows[[2]int{4, 7}]
	if !reflect.DeepEqual(fork.Upstream, []int{1}) || !reflect.DeepEqual(fork.Downstream, []int{2, 4}) {
		t.Fatalf("fork=%+v want upstream1/downstream2,4", fork)
	}
}
func TestRiverFlowSingletonConnectsLakeToSeaAndUnsupportedGraphStaysUnknown(t *testing.T) {
	tiles := riverFlowFixture(map[[2]int]Terrain{{7, 7}: TerrainRiver, {6, 7}: TerrainCoastalSea, {7, 8}: TerrainRiverDelta})
	f := RiverFlows(tiles, 9, 9)[[2]int{7, 7}]
	if !f.Known || !reflect.DeepEqual(f.Upstream, []int{1}) || !reflect.DeepEqual(f.Downstream, []int{0, 4}) {
		t.Fatalf("singleton=%+v", f)
	}
	// The triangular river graph has no preserved ordering. Never synthesize
	// an arrow for it, even when one corner touches a sea+delta mouth.
	tiles = riverFlowFixture(map[[2]int]Terrain{{6, 7}: TerrainRiver, {7, 7}: TerrainRiver, {6, 8}: TerrainRiver, {7, 8}: TerrainRiverDelta})
	for xy, f := range RiverFlows(tiles, 9, 9) {
		if f.Known || len(f.Upstream) > 0 || len(f.Downstream) > 0 {
			t.Errorf("unsupported cycle%v invented flow%+v", xy, f)
		}
	}
}
