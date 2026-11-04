package economy

import (
	"fmt"
	"testing"

	"formatet/megaron/server/internal/world"
)

type parityID struct{}

func (parityID) String() string { return "bay-parity" }

// world.Places names a "bay" from its own copy of the fishing bay rule (world cannot
// import economy). Over generated maps the two must agree: every hex where
// rulesWithLandNeighbors doubles the fish rate is either in a bay or in a strait
// (a bay inside a strait is strait water), and every bay hex has the doubled rate.
func TestBayParityWithWorld(t *testing.T) {
	t.Parallel()
	for i := 0; i < 10; i++ {
		tiles, seed := world.GenerateMap(parityID{}, int64(100+i*7919), 56, 40)
		terrain := map[[2]int]world.Terrain{}
		for _, tile := range tiles {
			terrain[[2]int{tile.Q, tile.R}] = tile.Terrain
		}
		inKind := map[string]map[[2]int]bool{"bay": {}, "strait": {}}
		for _, p := range world.Places(tiles) {
			if m, ok := inKind[p.Kind]; ok {
				for _, h := range p.Hexes {
					m[h] = true
				}
			}
		}
		rule := 0
		for _, tile := range tiles {
			land := 0
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}} {
				if n, ok := terrain[[2]int{tile.Q + d[0], tile.R + d[1]}]; ok && n != world.TerrainCoastalSea && n != world.TerrainDeepSea && n != world.TerrainLake {
					land++
				}
			}
			doubled := false
			for _, r := range rulesWithLandNeighbors(string(tile.Terrain), false, false, false, false, land) {
				doubled = doubled || r.good == "fish" && r.rateMult == 2
			}
			h := [2]int{tile.Q, tile.R}
			if doubled {
				rule++
			}
			switch {
			case inKind["bay"][h] && !doubled:
				t.Fatalf("seed %d: world bay hex %v is not a fishing bay by economy's rule", seed, h)
			case doubled && !inKind["bay"][h] && !inKind["strait"][h]:
				t.Fatalf("seed %d: economy bay hex %v is neither bay nor strait in world.Places", seed, h)
			case inKind["bay"][h] && inKind["strait"][h]:
				t.Fatalf("seed %d: hex %v is both bay and strait", seed, h)
			}
		}
		if rule == 0 {
			t.Fatal(fmt.Sprintf("seed %d: no bay hexes, test proves nothing", seed))
		}
	}
}
