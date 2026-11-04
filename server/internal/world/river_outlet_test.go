package world

import "testing"

// Every river system (river + ford + delta) touches the open sea: no river ends
// inland or in a lake (order sjo-flod-alfabet 2026-11-01 point 2; measured 2026-11-04:
// 268 systems on 40 generated maps and all 8 on the live map reach the sea).
func TestGenerateMap_EveryRiverSystemReachesTheSea(t *testing.T) {
	t.Parallel()
	isRiver := func(t Terrain) bool { return t == TerrainRiver || t == TerrainRiverFord || t == TerrainRiverDelta }
	for _, sz := range [][2]int{{64, 64}, {56, 40}} {
		for seed := int64(0); seed < 20; seed++ {
			tiles, _ := GenerateMap(stubID{}, seed, sz[0], sz[1])
			terrain := map[cell]Terrain{}
			for _, x := range tiles {
				terrain[cell{x.Q, x.R}] = x.Terrain
			}
			seen := map[cell]bool{}
			for c, tr := range terrain {
				if !isRiver(tr) || seen[c] {
					continue
				}
				sea := false
				queue := []cell{c}
				seen[c] = true
				for len(queue) > 0 {
					x := queue[0]
					queue = queue[1:]
					for _, n := range riverNeighbours(x) {
						nt, ok := terrain[n]
						if !ok {
							continue
						}
						if nt == TerrainCoastalSea || nt == TerrainDeepSea {
							sea = true
						}
						if isRiver(nt) && !seen[n] {
							seen[n] = true
							queue = append(queue, n)
						}
					}
				}
				if !sea {
					t.Errorf("%dx%d seed %d: river system at (%d,%d) never reaches the sea", sz[0], sz[1], seed, c.q, c.r)
				}
			}
		}
	}
}
