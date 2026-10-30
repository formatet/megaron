package world

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
)

func TestSilverSourceTargetScalesWithMapArea(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ width, height, want int }{
		{16, 16, 4}, // existing small-map floor
		{56, 40, 5},
		{60, 60, 9},
		{64, 64, 10},
		{64, 128, 20},
		{128, 64, 20},
		{128, 128, 40},
		{230, 230, 129},
	} {
		t.Run(fmt.Sprintf("%dx%d", tc.width, tc.height), func(t *testing.T) {
			if got := silverSourceTarget(tc.width, tc.height); got != tc.want {
				t.Fatalf("silver target = %d, want %d", got, tc.want)
			}
		})
	}
}

// This pins the twenty requested calibration maps, not every possible seed
// or a spawn-coverage guarantee. GenerateMap's existing validation stays in use.
func TestGenerateMapTenSilverSourcesOn64x64(t *testing.T) {
	t.Parallel()
	seen := map[int64]bool{}
	for i := int64(1); i <= 20; i++ {
		t.Run(fmt.Sprintf("seed_%d", 1009*i), func(t *testing.T) {
			tiles, effective := GenerateMap(uuid.Nil, 1009*i, 64, 64)
			if seen[effective] {
				t.Fatalf("duplicate effective seed %d", effective)
			}
			seen[effective] = true
			if err := validateMap(tiles, 64, 64); err != nil {
				t.Fatalf("invalid generated map: %v", err)
			}
			var silver []cell
			for _, tile := range tiles {
				if tile.SilverDeposit {
					silver = append(silver, cell{tile.Q, tile.R})
				}
			}
			if len(silver) != 10 {
				t.Fatalf("effective seed %d: %d silver hexes, want 10", effective, len(silver))
			}
			for j, a := range silver {
				for _, b := range silver[:j] {
					if hexDist(a, b) < 5 {
						t.Fatalf("silver sources %v and %v violate unchanged spacing 5", a, b)
					}
				}
			}
		})
	}
}
