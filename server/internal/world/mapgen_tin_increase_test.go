package world

import (
	"fmt"
	"reflect"
	"testing"
)

func TestTinAdditionalBudgetScalesWithArea(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ w, h, want int }{{32, 32, 1}, {56, 40, 1}, {64, 64, 2}, {128, 128, 8}, {230, 230, 26}} {
		if got := tinAdditionalSourceTarget(tc.w, tc.h); got != tc.want {
			t.Fatalf("%dx%d additional source budget=%d want%d", tc.w, tc.h, got, tc.want)
		}
	}
}

// The tin pass may only ADD tin: every other field of the finished map, and
// every tin deposit the base generator placed, is unchanged by addTinSources.
// Until 2026-11-22 this compared against maps recorded at 2be784c2 (the
// slice's own proof that existing worlds kept their geography); the channel
// meander changed every map on purpose, so the invariant is now checked within
// one run — the base map before the pass against the same map after it.
func TestGenerateMap_TinIncreasePreservesGeography(t *testing.T) {
	t.Parallel()
	for _, size := range []int{64, 128} {
		for i := int64(1); i <= 20; i++ {
			requested := 1009 * i
			t.Run(fmt.Sprintf("%d/seed%d", size, requested), func(t *testing.T) {
				tiles, eff := GenerateMap(stubID{}, requested, size, size)
				if err := validateMap(tiles, size, size); err != nil {
					t.Fatal(err)
				}
				base := generateMapOnce(stubID{}, eff, size, size)
				pre := append([]MapTile(nil), base...)
				addTinSources(pre, eff, size, size)
				added := 0
				for k := range base {
					before, after := base[k], pre[k]
					if after.TinDeposit && !before.TinDeposit {
						added++
						if after.Terrain != TerrainMountainLimestone {
							t.Fatal("added tin on wrong terrain")
						}
					}
					if before.TinDeposit && !after.TinDeposit {
						t.Fatal("an old tin deposit was moved/removed")
					}
					after.TinDeposit = before.TinDeposit
					if !reflect.DeepEqual(before, after) {
						t.Fatalf("tin pass changed a non-tin field at (%d,%d)", before.Q, before.R)
					}
				}
				// GenerateMap = this pre-lake map plus incense, river metadata and lakes.
				AddIncenseSources(pre, eff, size, size)
				PopulateRiverMetadata(pre, size, size)
				MarkLakes(pre, size, size)
				if !reflect.DeepEqual(pre, tiles) {
					t.Fatal("GenerateMap differs from base + tin + incense + rivers + lakes")
				}
			})
		}
	}
}
