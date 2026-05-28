package world

import "math/rand"

type cell struct{ q, r int }

// GenerateMap procedurally generates a hex grid for a world using a seeded RNG.
// The algorithm:
//  1. Fill with sea.
//  2. Place a handful of "land seeds" based on the seed.
//  3. Expand land tiles using flood-fill with random variance.
//  4. Assign terrain based on distance from land center and fertility/mineral noise.
func GenerateMap(worldID interface{ String() string }, seed int64, width, height int) []MapTile {
	rng := rand.New(rand.NewSource(seed))

	grid := make(map[cell]Terrain)
	fertility := make(map[cell]float64)
	_ = fertility
	mineral := make(map[cell]float64)
	_ = mineral

	// Start with all sea.
	for q := 0; q < width; q++ {
		for r := 0; r < height; r++ {
			grid[cell{q, r}] = TerrainSea
			fertility[cell{q, r}] = 0
			mineral[cell{q, r}] = 0
		}
	}

	// Land seeds — distribute roughly evenly with some noise.
	landSeeds := 4 + rng.Intn(4)
	for i := 0; i < landSeeds; i++ {
		q := 3 + rng.Intn(width-6)
		r := 3 + rng.Intn(height-6)
		// Expand outward from each seed.
		expandLand(grid, rng, cell{q, r}, width, height, 4+rng.Intn(6))
	}

	// Assign terrain types and resource values.
	tiles := make([]MapTile, 0, width*height)
	for q := 0; q < width; q++ {
		for r := 0; r < height; r++ {
			c := cell{q, r}
			terrain := grid[c]

			// Coastal tiles: land cells adjacent to sea.
			if terrain != TerrainSea && hasSeaNeighbour(grid, c, width, height) {
				terrain = TerrainCoast
			}

			f := 0.2 + rng.Float64()*0.8
			m := 0.1 + rng.Float64()*0.7
			if terrain == TerrainMountain || terrain == TerrainHills {
				f = 0.1 + rng.Float64()*0.3
				m = 0.4 + rng.Float64()*0.5
			}
			if terrain == TerrainSea || terrain == TerrainCoast {
				f = 0
				m = 0
			}

			tiles = append(tiles, MapTile{
				Q:         q,
				R:         r,
				Terrain:   terrain,
				Fertility: f,
				Mineral:   m,
			})
		}
	}
	return tiles
}

func expandLand(grid map[cell]Terrain, rng *rand.Rand, seed cell, width, height, radius int) {
	queue := []cell{seed}
	visited := map[cell]bool{seed: true}

	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]

		dist := hexDist(c, seed)
		var terrain Terrain
		switch {
		case dist == 0:
			terrain = TerrainPlains
		case dist <= radius/3:
			terrain = TerrainPlains
		case dist <= radius*2/3:
			if rng.Float64() > 0.5 {
				terrain = TerrainHills
			} else {
				terrain = TerrainForest
			}
		default:
			if rng.Float64() > 0.7 {
				terrain = TerrainMountain
			} else if rng.Float64() > 0.4 {
				terrain = TerrainHills
			} else {
				terrain = TerrainForest
			}
		}
		grid[c] = terrain

		if dist >= radius {
			continue
		}

		for _, n := range hexNeighbours(c, width, height) {
			if !visited[n] && rng.Float64() > 0.2 {
				visited[n] = true
				queue = append(queue, n)
			}
		}
	}
}

func hexDist(a, b cell) int {
	dq := a.q - b.q
	dr := a.r - b.r
	v := (iAbs(dq) + iAbs(dq+dr) + iAbs(dr)) / 2
	return v
}

func hexNeighbours(c cell, w, h int) []cell {
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
	var out []cell
	for _, d := range dirs {
		nq, nr := c.q+d[0], c.r+d[1]
		if nq >= 0 && nq < w && nr >= 0 && nr < h {
			out = append(out, cell{nq, nr})
		}
	}
	return out
}

func hasSeaNeighbour(grid map[cell]Terrain, c cell, w, h int) bool {
	for _, n := range hexNeighbours(c, w, h) {
		if grid[n] == TerrainSea {
			return true
		}
	}
	return false
}

func iAbs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

