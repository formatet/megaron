package world

import (
	"reflect"
	"testing"
)

func nameFixture() []MapTile {
	return []MapTile{{Q: 0, Terrain: TerrainRiver}, {Q: 1, Terrain: TerrainRiverFord}, {Q: 2, Terrain: TerrainRiver}, {Q: 3, Terrain: TerrainRiverDelta}, {Q: 4, Terrain: TerrainRiverDelta}, {Q: 10, Terrain: TerrainRiver}, {Q: 11, Terrain: TerrainRiver}, {Q: 12, Terrain: TerrainRiverDelta}, {Q: 20, Terrain: TerrainPlains}}
}
func TestRiverAnchorsStableAndDeltaInheritance(t *testing.T) {
	ts := nameFixture()
	want := RiverAnchors(ts)
	for i, j := 0, len(ts)-1; i < j; i, j = i+1, j-1 {
		ts[i], ts[j] = ts[j], ts[i]
	}
	if !reflect.DeepEqual(want, RiverAnchors(ts)) {
		t.Fatal("SQL order changes river identity")
	}
	for _, q := range []int{0, 1, 2, 3, 4} {
		if want[[2]int{q, 0}] != "0,0" {
			t.Fatalf("river/ford/delta have different anchors at %d", q)
		}
	}
	if want[[2]int{10, 0}] != "10,0" || want[[2]int{12, 0}] != "10,0" || want[[2]int{20, 0}] != "" {
		t.Fatal("incorrect distinct chain or nonriver anchor", want)
	}
}
