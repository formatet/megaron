package world

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestIncenseSourceTarget(t *testing.T) {
	for _, tc := range []struct{ w, h, want, east int }{
		{16, 16, 3, 2}, // floor
		{64, 64, 10, 6},
		{128, 128, 40, 25},
	} {
		if got := incenseSourceTarget(tc.w, tc.h); got != tc.want {
			t.Errorf("%dx%d: target %d, want %d", tc.w, tc.h, got, tc.want)
		}
		if got := incenseEastTarget(tc.want); got != tc.east {
			t.Errorf("target %d: east %d, want %d", tc.want, got, tc.east)
		}
	}
}

// Order 32: about ten incense hexes per 64x64 world, only on scrub_maquis /
// semi_desert, spaced like silver, spread over both halves, deterministic per
// seed, and no other field of the map moves.
func TestGenerateMapIncenseSources64x64(t *testing.T) {
	for i := int64(1); i <= 20; i++ {
		seed := 1009 * i
		t.Run(fmt.Sprintf("seed_%d", seed), func(t *testing.T) {
			tiles, eff := GenerateMap(uuid.Nil, seed, 64, 64)
			var inc []cell
			east, west := 0, 0
			for _, tile := range tiles {
				if !tile.IncenseDeposit {
					continue
				}
				if tile.Terrain != TerrainScrubMaquis && tile.Terrain != TerrainSemiDesert {
					t.Fatalf("incense on %s at %d,%d", tile.Terrain, tile.Q, tile.R)
				}
				if tile.CopperDeposit || tile.TinDeposit || tile.SilverDeposit {
					t.Fatalf("incense shares a hex with a metal at %d,%d", tile.Q, tile.R)
				}
				inc = append(inc, cell{tile.Q, tile.R})
				if tile.Q >= 32 {
					east++
				} else {
					west++
				}
			}
			t.Logf("seed %d: %d incense (east %d, west %d)", eff, len(inc), east, west)
			if len(inc) < 8 || len(inc) > 10 {
				t.Fatalf("seed %d: %d incense hexes, want about 10 (8..10)", eff, len(inc))
			}
			if east == 0 || west == 0 || east <= west {
				t.Errorf("seed %d: east %d / west %d, want both halves present and the east larger", eff, east, west)
			}
			for j, a := range inc {
				for _, b := range inc[:j] {
					if hexDist(a, b) < incenseSourceSpacing {
						t.Fatalf("incense %v and %v closer than %d", a, b, incenseSourceSpacing)
					}
				}
			}

			// Deterministic: the same seed gives the same map.
			again, _ := GenerateMap(uuid.Nil, seed, 64, 64)
			if !reflect.DeepEqual(tiles, again) {
				t.Fatal("same seed gave a different map")
			}

			// Nothing else moved: the pre-incense map (same engine, no incense pass)
			// equals this one with the incense flag cleared.
			before := generateMapOnce(uuid.Nil, eff, 64, 64)
			addTinSources(before, eff, 64, 64)
			PopulateRiverMetadata(before, 64, 64)
			cleared := make([]MapTile, len(tiles))
			copy(cleared, tiles)
			for k := range cleared {
				cleared[k].IncenseDeposit = false
			}
			if !reflect.DeepEqual(before, cleared) {
				t.Fatal("the incense pass changed something other than the incense flag")
			}
		})
	}
}

// AddIncenseSources is idempotent: the seed tool may be run twice.
func TestAddIncenseSourcesIsIdempotent(t *testing.T) {
	tiles, eff := GenerateMap(uuid.Nil, 1009, 64, 64)
	snapshot := make([]MapTile, len(tiles))
	copy(snapshot, tiles)
	if n := AddIncenseSources(tiles, eff, 64, 64); n != 0 {
		t.Errorf("second run placed %d, want 0", n)
	}
	if !reflect.DeepEqual(tiles, snapshot) {
		t.Error("second run changed the map")
	}
	// Stripped of its flags, the same call restores exactly the same hexes.
	stripped := make([]MapTile, len(tiles))
	copy(stripped, tiles)
	for k := range stripped {
		stripped[k].IncenseDeposit = false
	}
	if n := AddIncenseSources(stripped, eff, 64, 64); n == 0 || !reflect.DeepEqual(stripped, snapshot) {
		t.Errorf("re-seeding a stripped map must reproduce the generated one (placed %d)", n)
	}
}
