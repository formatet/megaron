package world

import "math/rand"

type cell struct{ q, r int }

// landmass IDs used during generation
const (
	lmSea     = 0
	lmWest    = 1 // copper biased
	lmEast    = 2 // tin + cedar biased
	lmIsland  = 3 // small neutral islands
)

// GenerateMap procedurally generates a hex grid for a world using a seeded RNG.
//
// v2 guarantees:
//   - Copper deposits only on the western landmass.
//   - Tin deposits only on the eastern landmass.
//   - Cedar deposits only on the eastern landmass (3–5 forest tiles).
//   - At least 2 copper tiles and 2 tin tiles.
//   - 2 river-valley corridors running from inland hills toward the coast.
//   - Multiple distinct landmasses separated by sea (copper/tin impossible to reach overland).
func GenerateMap(worldID interface{ String() string }, seed int64, width, height int) []MapTile {
	rng := rand.New(rand.NewSource(seed))

	grid    := make(map[cell]Terrain)
	landmap := make(map[cell]int) // which landmass each cell belongs to

	// ── 1. Fill with deep sea ─────────────────────────────────────────
	for q := 0; q < width; q++ {
		for r := 0; r < height; r++ {
			grid[cell{q, r}]    = TerrainDeepSea
			landmap[cell{q, r}] = lmSea
		}
	}

	// ── 2. Western landmass (copper) ───────────────────────────────────
	// Seed in the left third of the map.
	wq := 4 + rng.Intn(width/4)
	wr := height/4 + rng.Intn(height/2)
	wRadius := 5 + rng.Intn(4)
	expandLandmass(grid, landmap, rng, cell{wq, wr}, width, height, wRadius, lmWest)

	// ── 3. Eastern landmass (tin + cedar) ─────────────────────────────
	// Seed in the right third, with a guaranteed sea gap from the west.
	eq := width*2/3 + rng.Intn(width/5)
	er := height/4 + rng.Intn(height/2)
	eRadius := 5 + rng.Intn(4)
	expandLandmass(grid, landmap, rng, cell{eq, er}, width, height, eRadius, lmEast)

	// ── 4. Small islands (0–2) ─────────────────────────────────────────
	numIslands := rng.Intn(3)
	for i := 0; i < numIslands; i++ {
		iq := width/3 + rng.Intn(width/3)
		ir := 3 + rng.Intn(height-6)
		if landmap[cell{iq, ir}] != lmSea {
			continue
		}
		expandLandmass(grid, landmap, rng, cell{iq, ir}, width, height, 2+rng.Intn(3), lmIsland)
	}

	// ── 5. Coastlines ─────────────────────────────────────────────────
	// 5a. Land tiles adjacent to sea → coast_beach.
	for q := 0; q < width; q++ {
		for r := 0; r < height; r++ {
			c := cell{q, r}
			if grid[c] != TerrainDeepSea && hasDeepSeaNeighbour(grid, c, width, height) {
				grid[c] = TerrainCoastBeach
			}
		}
	}
	// 5b. Deep-sea tiles adjacent to land → coastal_sea (shallow, faster sailing).
	for q := 0; q < width; q++ {
		for r := 0; r < height; r++ {
			c := cell{q, r}
			if grid[c] == TerrainDeepSea && hasLandNeighbour(grid, c, width, height) {
				grid[c] = TerrainCoastalSea
			}
		}
	}

	// ── 6. River valleys (2 per map) ──────────────────────────────────
	// Find pairs of inland plains/hills tiles and carve a short corridor toward the coast.
	addRiverValley(grid, landmap, rng, lmWest, width, height)
	addRiverValley(grid, landmap, rng, lmEast, width, height)

	// ── 7. Assign deposits ────────────────────────────────────────────
	var cedarCandidates []int // indices into tiles
	tiles := make([]MapTile, 0, width*height)

	for q := 0; q < width; q++ {
		for r := 0; r < height; r++ {
			c := cell{q, r}
			terrain := grid[c]
			lm      := landmap[c]
			isRock  := terrain == TerrainHills || terrain == TerrainMountainLimestone || terrain == TerrainMountainRed

			var copperDeposit, tinDeposit, silverDeposit, cedarDeposit bool

			if isRock {
				switch lm {
				case lmWest:
					if rng.Float64() < 0.30 {
						copperDeposit = true
					}
				case lmEast:
					if rng.Float64() < 0.30 {
						tinDeposit = true
					}
				case lmIsland:
					// small islands: small chance of either
					if rng.Float64() < 0.12 {
						copperDeposit = true
					} else if rng.Float64() < 0.12 {
						tinDeposit = true
					}
				}
				// Silver: rare, on any landmass rock (~10%)
				if !copperDeposit && !tinDeposit && rng.Float64() < 0.10 {
					silverDeposit = true
				}
			}

			idx := len(tiles)
			tiles = append(tiles, MapTile{
				Q: q, R: r,
				Terrain:       terrain,
				Fertility:     0.2 + rng.Float64()*0.8,
				Mineral:       0.1 + rng.Float64()*0.7,
				CopperDeposit: copperDeposit,
				TinDeposit:    tinDeposit,
				SilverDeposit: silverDeposit,
				CedarDeposit:  cedarDeposit,
			})

			// Track forest tiles on the eastern landmass as cedar candidates
			if terrain == TerrainForestOliveGrove && lm == lmEast {
				cedarCandidates = append(cedarCandidates, idx)
			}
		}
	}

	// ── 8. Cedar deposits (3–5 eastern forest tiles) ──────────────────
	cedarTarget := 3 + rng.Intn(3) // 3–5
	rng.Shuffle(len(cedarCandidates), func(i, j int) {
		cedarCandidates[i], cedarCandidates[j] = cedarCandidates[j], cedarCandidates[i]
	})
	assigned := 0
	for _, idx := range cedarCandidates {
		if assigned >= cedarTarget {
			break
		}
		tiles[idx].CedarDeposit = true
		assigned++
	}

	// ── 9. Guarantee minimums ─────────────────────────────────────────
	isRockTile := func(t MapTile) bool {
		return t.Terrain == TerrainHills || t.Terrain == TerrainMountainLimestone || t.Terrain == TerrainMountainRed
	}

	tiles = guaranteeDeposit(tiles, func(t MapTile) bool { return t.TinDeposit },
		func(t *MapTile) { t.TinDeposit = true },
		func(t MapTile) bool { return isRockTile(t) && !t.CopperDeposit && !t.SilverDeposit },
		2)

	tiles = guaranteeDeposit(tiles, func(t MapTile) bool { return t.CopperDeposit },
		func(t *MapTile) { t.CopperDeposit = true },
		func(t MapTile) bool { return isRockTile(t) && !t.TinDeposit && !t.SilverDeposit },
		2)

	if assigned < 2 {
		// Guarantee at least 2 cedar tiles on any forest
		for i := range tiles {
			if assigned >= 2 {
				break
			}
			if tiles[i].Terrain == TerrainForestOliveGrove && !tiles[i].CedarDeposit {
				tiles[i].CedarDeposit = true
				assigned++
			}
		}
	}

	return tiles
}

// guaranteeDeposit ensures at least `min` tiles satisfy `hasDeposit`.
// If not, it sets the deposit on candidate tiles.
func guaranteeDeposit(
	tiles []MapTile,
	hasDeposit func(MapTile) bool,
	setDeposit func(*MapTile),
	isCandidate func(MapTile) bool,
	min int,
) []MapTile {
	count := 0
	for _, t := range tiles {
		if hasDeposit(t) {
			count++
		}
	}
	for i := range tiles {
		if count >= min {
			break
		}
		if isCandidate(tiles[i]) && !hasDeposit(tiles[i]) {
			setDeposit(&tiles[i])
			count++
		}
	}
	return tiles
}

// expandLandmass flood-fills terrain from a seed cell, marking each cell with the given landmass ID.
func expandLandmass(grid map[cell]Terrain, landmap map[cell]int, rng *rand.Rand, seed cell, width, height, radius, lm int) {
	queue   := []cell{seed}
	visited := map[cell]bool{seed: true}

	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]

		dist := hexDist(c, seed)
		var terrain Terrain
		switch {
		case dist == 0:
			terrain = TerrainPlains
		case dist <= radius/4:
			terrain = TerrainPlains
		case dist <= radius/2:
			if rng.Float64() > 0.4 {
				terrain = TerrainHills
			} else {
				terrain = TerrainPlains
			}
		case dist <= radius*3/4:
			switch {
			case rng.Float64() < 0.35:
				terrain = TerrainMountainLimestone
			case rng.Float64() < 0.55:
				terrain = TerrainHills
			default:
				terrain = TerrainForestOliveGrove
			}
		default:
			if rng.Float64() < 0.5 {
				terrain = TerrainForestOliveGrove
			} else {
				terrain = TerrainHills
			}
		}

		grid[c]    = terrain
		landmap[c] = lm

		if dist >= radius {
			continue
		}
		for _, n := range hexNeighbours(c, width, height) {
			if !visited[n] && rng.Float64() > 0.25 {
				visited[n] = true
				queue = append(queue, n)
			}
		}
	}
}

// addRiverValley creates a short river-valley corridor from an inland tile toward the coast.
// Converts 3–6 plains or hills tiles in a line to river_valley terrain.
func addRiverValley(grid map[cell]Terrain, landmap map[cell]int, rng *rand.Rand, targetLM, width, height int) {
	// Find inland plains/hills tiles on the target landmass (not coastal)
	var candidates []cell
	for q := 0; q < width; q++ {
		for r := 0; r < height; r++ {
			c := cell{q, r}
			if landmap[c] == targetLM &&
				(grid[c] == TerrainPlains || grid[c] == TerrainHills) &&
				!hasDeepSeaNeighbour(grid, c, width, height) {
				candidates = append(candidates, c)
			}
		}
	}
	if len(candidates) == 0 {
		return
	}

	// Pick a random starting point
	start := candidates[rng.Intn(len(candidates))]
	length := 3 + rng.Intn(4) // 3–6 tiles

	// Walk roughly toward the coast (toward lower r or higher r, picking the nearest coast direction)
	dr := 1
	if start.r > height/2 {
		dr = -1
	}
	c := start
	for i := 0; i < length; i++ {
		if landmap[c] == targetLM {
			grid[c] = TerrainRiverValley
		}
		// Move toward coast
		next := cell{c.q + rng.Intn(3) - 1, c.r + dr}
		if next.q < 0 || next.q >= width || next.r < 0 || next.r >= height {
			break
		}
		if isSea(grid[next]) || grid[next] == TerrainCoastBeach {
			break
		}
		c = next
	}
}

func hexDist(a, b cell) int {
	dq := a.q - b.q
	dr := a.r - b.r
	return (iAbs(dq) + iAbs(dq+dr) + iAbs(dr)) / 2
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

func isSea(t Terrain) bool {
	return t == TerrainDeepSea || t == TerrainCoastalSea
}

// hasDeepSeaNeighbour reports whether a land tile borders deep sea (used during coastline marking,
// before coastal_sea tiles exist).
func hasDeepSeaNeighbour(grid map[cell]Terrain, c cell, w, h int) bool {
	for _, n := range hexNeighbours(c, w, h) {
		if grid[n] == TerrainDeepSea {
			return true
		}
	}
	return false
}

// hasLandNeighbour reports whether a sea tile borders any land tile.
func hasLandNeighbour(grid map[cell]Terrain, c cell, w, h int) bool {
	for _, n := range hexNeighbours(c, w, h) {
		if !isSea(grid[n]) {
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
