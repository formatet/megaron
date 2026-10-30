package world

// MarkLakes relabels every sea tile that does not reach the map's edge through
// sea as TerrainLake (Timothy 2026-10-30). Rivers do not connect: a lake with a
// river out to the Thalassa is still a lake. It runs last in GenerateMap, after
// validation and river metadata, so no earlier rule ever sees a lake.
// Migration 232 applies the same rule to existing worlds.
func MarkLakes(tiles []MapTile, width, height int) {
	grid := make(map[cell]Terrain, len(tiles))
	for _, t := range tiles {
		grid[cell{t.Q, t.R}] = t.Terrain
	}
	main := mainSeaComponent(grid, width, height)
	for i := range tiles {
		if isSea(tiles[i].Terrain) && !main[cell{tiles[i].Q, tiles[i].R}] {
			tiles[i].Terrain = TerrainLake
		}
	}
}
