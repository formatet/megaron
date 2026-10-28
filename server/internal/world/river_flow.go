package world

// RiverFlow describes direction justified by the saved river graph and the
// generator's existing sea+delta mouth invariant. Unsupported geometry remains
// unknown; no height or historical carve order is invented.
type RiverFlow struct {
	Known      bool
	Upstream   []int
	Downstream []int
}

func RiverFlows(tiles []MapTile, width, height int) map[[2]int]RiverFlow {
	grid := make(map[cell]Terrain, len(tiles))
	for _, t := range tiles {
		grid[cell{t.Q, t.R}] = t.Terrain
	}
	sea := mainSeaComponent(grid, width, height)
	out := map[[2]int]RiverFlow{}
	for _, chain := range riverComponents(tiles) {
		members := map[cell]bool{}
		for _, c := range chain {
			members[c] = true
			out[[2]int{c.q, c.r}] = RiverFlow{Upstream: []int{}, Downstream: []int{}}
		}
		degree := map[cell]int{}
		mouth := map[cell]bool{}
		var roots []cell
		edgeCount := 0
		for _, c := range chain {
			delta, open := false, false
			for _, n := range riverNeighbours(c) {
				if members[n] {
					degree[c]++
					edgeCount++
				}
				if grid[n] == TerrainRiverDelta {
					delta = true
				}
				if sea[n] {
					open = true
				}
			}
			if degree[c] <= 1 && delta && open {
				mouth[c] = true
			}
			if degree[c] <= 1 && !mouth[c] {
				roots = append(roots, c)
			}
		}
		// A singleton still has a provable inlet if it touches an inland source
		// lake, and a sea+delta outlet. Several touching sea edges are natural exits.
		if len(chain) == 1 {
			c := chain[0]
			flow := out[[2]int{c.q, c.r}]
			if mouth[c] {
				for i, d := range riverDirs {
					n := cell{c.q + d[0], c.r + d[1]}
					if sea[n] {
						flow.Downstream = append(flow.Downstream, i)
					} else if isSea(grid[n]) {
						flow.Upstream = append(flow.Upstream, i)
					}
				}
				flow.Known = len(flow.Upstream) > 0 && len(flow.Downstream) > 0
			}
			if flow.Known {
				out[[2]int{c.q, c.r}] = flow
			}
			continue
		}
		if edgeCount/2 != len(chain)-1 || len(roots) != 1 || len(mouth) == 0 {
			continue
		}
		root := roots[0]
		distance := map[cell]int{root: 0}
		queue := []cell{root}
		for i := 0; i < len(queue); i++ {
			c := queue[i]
			for _, n := range riverNeighbours(c) {
				if !members[n] {
					continue
				}
				if _, seen := distance[n]; !seen {
					distance[n] = distance[c] + 1
					queue = append(queue, n)
				}
			}
		}
		for _, c := range chain {
			flow := RiverFlow{Known: true, Upstream: []int{}, Downstream: []int{}}
			for i, d := range riverDirs {
				n := cell{c.q + d[0], c.r + d[1]}
				if members[n] {
					if distance[n] < distance[c] {
						flow.Upstream = append(flow.Upstream, i)
					} else {
						flow.Downstream = append(flow.Downstream, i)
					}
				} else if isSea(grid[n]) {
					if c == root && !sea[n] {
						flow.Upstream = append(flow.Upstream, i)
					} else if mouth[c] && sea[n] {
						flow.Downstream = append(flow.Downstream, i)
					}
				}
			}
			out[[2]int{c.q, c.r}] = flow
		}
	}
	return out
}
