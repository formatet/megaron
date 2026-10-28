package world

// RiverEdges returns actual saved water adjacency on river/ford hexes. Edge
// indices follow riverDirs and the web HEX_DIRS contract:
// (1,0),(-1,0),(0,1),(0,-1),(1,-1),(-1,1).
// An empty slice is an actual endpoint, not a synthetic continuation axis.
// This is geography only; the API must apply observer visibility before sending it.
func RiverEdges(tiles []MapTile) map[[2]int][]int {
	grid := make(map[[2]int]Terrain, len(tiles))
	for _, t := range tiles {
		grid[[2]int{t.Q, t.R}] = t.Terrain
	}
	out := map[[2]int][]int{}
	for xy, terrain := range grid {
		if terrain != TerrainRiver && terrain != TerrainRiverFord {
			continue
		}
		edges := make([]int, 0, 6)
		for i, d := range riverDirs {
			neighbour := grid[[2]int{xy[0] + d[0], xy[1] + d[1]}]
			if neighbour == TerrainRiver || neighbour == TerrainRiverFord || isSea(neighbour) {
				edges = append(edges, i)
			}
		}
		out[xy] = edges
	}
	return out
}

// RiverEdgeDelta is the geometry contract shared by persisted masks and API edge references.
func RiverEdgeDelta(edge int) (dq, dr int, ok bool) {
	if edge < 0 || edge >= len(riverDirs) {
		return 0, 0, false
	}
	return riverDirs[edge][0], riverDirs[edge][1], true
}
