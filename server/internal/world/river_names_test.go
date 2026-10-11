package world

import (
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"testing"
)

func nameFixture() []MapTile {
	return []MapTile{{Q: 0, Terrain: TerrainRiver}, {Q: 1, Terrain: TerrainRiverFord}, {Q: 2, Terrain: TerrainRiver}, {Q: 3, Terrain: TerrainRiverDelta}, {Q: 4, Terrain: TerrainRiverDelta}, {Q: 10, Terrain: TerrainRiver}, {Q: 11, Terrain: TerrainRiver}, {Q: 12, Terrain: TerrainRiverDelta}, {Q: 20, Terrain: TerrainPlains}}
}
func TestRiverNamesDeterministicAcrossProcesses(t *testing.T) {
	ts := nameFixture()
	want := RiverNames("Mycenae", ts)
	if os.Getenv("RIVER_NAME_CHILD") == "1" {
		b, _ := json.Marshal(want[[2]int{0, 0}])
		os.Stdout.Write(b)
		os.Exit(0)
	}
	for i, j := 0, len(ts)-1; i < j; i, j = i+1, j-1 {
		ts[i], ts[j] = ts[j], ts[i]
	}
	if !reflect.DeepEqual(want, RiverNames("Mycenae", ts)) {
		t.Fatal("SQL order changes names")
	}
	for i := 0; i < 2; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRiverNamesDeterministicAcrossProcesses$")
		cmd.Env = append(os.Environ(), "RIVER_NAME_CHILD=1")
		b, e := cmd.Output()
		if e != nil {
			t.Fatal(e)
		}
		var name string
		if e = json.Unmarshal(b, &name); e != nil {
			t.Fatal(e)
		}
		if name != want[[2]int{0, 0}] {
			t.Fatal("restart changed name")
		}
	}
	if reflect.DeepEqual(want, RiverNames("Tiryns", ts)) {
		t.Fatal("world ID must salt names")
	}
}
func TestRiverNamesUniqueAndDeltaInheritance(t *testing.T) {
	ns := RiverNames("Mycenae", nameFixture())
	if ns[[2]int{0, 0}] == "" || ns[[2]int{0, 0}] == ns[[2]int{10, 0}] {
		t.Fatal("distinct chains need unique names")
	}
	for _, q := range []int{1, 2, 3, 4} {
		if ns[[2]int{q, 0}] != ns[[2]int{0, 0}] {
			t.Fatal("ford/delta did not inherit", q)
		}
	}
	if ns[[2]int{20, 0}] != "" {
		t.Fatal("plains got named")
	}
	var ts []MapTile
	for q := 0; q < (len(riverNamePool)+3)*5; q += 5 {
		ts = append(ts, MapTile{Q: q, Terrain: TerrainRiver})
	}
	seen := map[string]bool{}
	for _, n := range RiverNames("Overflow", ts) {
		if seen[n] {
			t.Fatal("duplicate name", n)
		}
		seen[n] = true
	}
	if len(seen) != len(ts) {
		t.Fatal("lost chain")
	}
}
