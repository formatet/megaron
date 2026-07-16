package world

// PNG debug export + map metrics for the mapgen CLI (cmd/mapgen-debug).
// Read-only over a generated tile set: nothing here may mutate tiles or feed
// back into generation/validation — validateMap stays the only gatekeeper.
// Stdlib only (image/png); the palette is free-standing (the map canvas is
// already exempt from the CSS-vars rule, see CLAUDE.md §Visual style).

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"sort"
)

// debugTilePx is the square edge per tile. No hex contours — a brick-offset
// grid of squares reads well enough at a glance and keeps the renderer tiny.
const debugTilePx = 8

// Terrain fills — desaturated background, per the palette table in
// temenos_mapgen_arkipelag_plan.md §P0.
var debugTerrainColor = map[Terrain]color.RGBA{
	TerrainDeepSea:           {0x15, 0x38, 0x5C, 0xFF}, // dark blue
	TerrainCoastalSea:        {0x4E, 0x86, 0xB4, 0xFF}, // light blue
	TerrainPlains:            {0x9D, 0xBB, 0x6E, 0xFF}, // light green
	TerrainHills:             {0x8A, 0x8A, 0x50, 0xFF}, // olive
	TerrainScrubMaquis:       {0xAD, 0xB8, 0x64, 0xFF}, // yellow-green
	TerrainSemiDesert:        {0xD6, 0xC0, 0x8A, 0xFF}, // sand
	TerrainMountainRed:       {0xA5, 0x67, 0x4C, 0xFF}, // red-brown
	TerrainMountainLimestone: {0xC9, 0xC6, 0xBC, 0xFF}, // light grey
	TerrainForestOliveGrove:  {0x4D, 0x6B, 0x35, 0xFF}, // dark green
	TerrainRiverValley:       {0x5F, 0xB8, 0xB8, 0xFF}, // cyan
	TerrainRiverDelta:        {0xA5, 0xDE, 0xDE, 0xFF}, // light cyan
}

// Deposit dots — saturated, drawn smaller on top of the terrain fill.
var (
	debugCopperColor = color.RGBA{0xE8, 0x86, 0x2D, 0xFF} // orange
	debugTinColor    = color.RGBA{0xD9, 0x30, 0x25, 0xFF} // red
	debugSilverColor = color.RGBA{0xF5, 0xF5, 0xF5, 0xFF} // white
	debugCedarColor  = color.RGBA{0x14, 0x5A, 0x32, 0xFF} // dark green
)

// debugUnknownColor flags a terrain missing from the palette — loud on purpose.
var debugUnknownColor = color.RGBA{0xFF, 0x00, 0xFF, 0xFF}

// ExportDebugPNG renders the tile set as a brick-offset square grid:
// x from q, y from r - rowOrigin(q, width), with a half-tile y-offset on odd
// columns — the same orientation as the web renderer (y ∝ r + q/2).
func ExportDebugPNG(tiles []MapTile, width, height int, path string) error {
	img := newDebugImage(width, height)
	for _, t := range tiles {
		x, y := debugTileOrigin(t.Q, t.R, width)
		c, ok := debugTerrainColor[t.Terrain]
		if !ok {
			c = debugUnknownColor
		}
		fillRect(img, x, y, debugTilePx, debugTilePx, c)
		if dc, ok := depositColor(t); ok {
			fillRect(img, x+2, y+2, 4, 4, dc)
		}
	}
	return writePNG(img, path)
}

// Overlay palette — saturated, mutually distinguishable colors cycled over
// land-component IDs (IDs are assigned in deterministic tile order).
var debugComponentColors = []color.RGBA{
	{0xE6, 0x19, 0x4B, 0xFF}, {0x3C, 0xB4, 0x4B, 0xFF}, {0xFF, 0xE1, 0x19, 0xFF},
	{0x43, 0x63, 0xD8, 0xFF}, {0xF5, 0x82, 0x31, 0xFF}, {0x91, 0x1E, 0xB4, 0xFF},
	{0x42, 0xD4, 0xF4, 0xFF}, {0xF0, 0x32, 0xE6, 0xFF}, {0xBF, 0xEF, 0x45, 0xFF},
	{0xFA, 0xBE, 0xBE, 0xFF}, {0x46, 0x99, 0x90, 0xFF}, {0x9A, 0x63, 0x24, 0xFF},
}

// ExportDebugOverlayPNG renders the readability overlay: land components
// color-coded, the two permanent sea-channel columns (33 %/67 % of width)
// marked, sea tinted by hemisphere (west = cool blue / east = violet), strait
// hexes marked amber, river-delta tiles outlined cyan, spawn candidates
// dotted white, and each deposit with its 7-hex catchment outlined.
// Harbours and passability are deliberately absent — those concepts do not
// exist in code yet; the overlay draws no fiction.
//
// P0-uppföljning (Timothy 2026-07-16 "var är sunden? och deltat"): straits
// and deltas were already counted into the sidecar JSON but never rendered —
// this is strategic readability #1, so both get a dedicated marker below.
func ExportDebugOverlayPNG(tiles []MapTile, width, height int, path string) error {
	img := newDebugImage(width, height)
	comp := landComponents(tiles)

	// Same channel columns as generateMapOnce §5b.
	chanW := width * 33 / 100
	chanE := width * 67 / 100

	seaWest := color.RGBA{0x1E, 0x3A, 0x50, 0xFF}   // west hemisphere tint (copper side)
	seaMid := color.RGBA{0x22, 0x30, 0x4A, 0xFF}    // neutral centre
	seaEast := color.RGBA{0x35, 0x28, 0x4A, 0xFF}   // east hemisphere tint (tin side)
	chanColor := color.RGBA{0x55, 0xAA, 0xFF, 0xFF} // channel columns — always sea
	spawnDot := color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	straitColor := color.RGBA{0xFF, 0xD5, 0x00, 0xFF} // amber — sund (strait) marker
	deltaColor := color.RGBA{0x00, 0xFF, 0xFF, 0xFF}  // bright cyan — river-delta marker

	// 1. Base fill: component colors on land, hemisphere-tinted sea, channels.
	for _, t := range tiles {
		x, y := debugTileOrigin(t.Q, t.R, width)
		var c color.RGBA
		switch {
		case t.Q == chanW || t.Q == chanE:
			c = chanColor
		case tileIsLand(t.Terrain):
			c = debugComponentColors[comp[[2]int{t.Q, t.R}]%len(debugComponentColors)]
		case t.Q < chanW:
			c = seaWest
		case t.Q > chanE:
			c = seaEast
		default:
			c = seaMid
		}
		fillRect(img, x, y, debugTilePx, debugTilePx, c)
	}

	// 2. Strait hexes: sea tiles flanked by land on an opposing axis pair —
	// same per-tile rule as countStraits (mapgen.go), replicated read-only
	// here (see spawnBuildable below for the same "copy on purpose"
	// reasoning: this file must never become an import target for game logic).
	terrainAt := make(map[[2]int]Terrain, len(tiles))
	for _, t := range tiles {
		terrainAt[[2]int{t.Q, t.R}] = t.Terrain
	}
	straitAxes := [][2][2]int{
		{{1, 0}, {-1, 0}},
		{{0, 1}, {0, -1}},
		{{1, -1}, {-1, 1}},
	}
	for _, t := range tiles {
		if !isSea(t.Terrain) {
			continue
		}
		strait := false
		for _, pair := range straitAxes {
			a := terrainAt[[2]int{t.Q + pair[0][0], t.R + pair[0][1]}]
			b := terrainAt[[2]int{t.Q + pair[1][0], t.R + pair[1][1]}]
			if tileIsLand(a) && tileIsLand(b) {
				strait = true
				break
			}
		}
		if strait {
			x, y := debugTileOrigin(t.Q, t.R, width)
			fillRect(img, x+2, y+2, 4, 4, straitColor)
		}
	}

	// 3. River deltas: outline in a bright marker on top of the component
	// fill, so the mouth is easy to spot without hunting for the (subtle)
	// light-cyan terrain color.
	for _, t := range tiles {
		if t.Terrain != TerrainRiverDelta {
			continue
		}
		x, y := debugTileOrigin(t.Q, t.R, width)
		outlineRect(img, x, y, debugTilePx, debugTilePx, deltaColor)
	}

	// 4. Deposit catchments: outline the deposit tile plus its 6 axial
	// neighbours (the 7-hex catchment RecomputeProduction reads).
	inMapTiles := map[[2]int]bool{}
	for _, t := range tiles {
		inMapTiles[[2]int{t.Q, t.R}] = true
	}
	dirs7 := [7][2]int{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
	for _, t := range tiles {
		dc, ok := depositColor(t)
		if !ok {
			continue
		}
		for _, d := range dirs7 {
			nq, nr := t.Q+d[0], t.R+d[1]
			if !inMapTiles[[2]int{nq, nr}] {
				continue
			}
			x, y := debugTileOrigin(nq, nr, width)
			outlineRect(img, x, y, debugTilePx, debugTilePx, dc)
		}
	}

	// 5. Spawn candidates (buildable terrain, see spawnBuildable) + deposit dots.
	for _, t := range tiles {
		x, y := debugTileOrigin(t.Q, t.R, width)
		if spawnBuildable(t.Terrain) {
			fillRect(img, x+3, y+3, 2, 2, spawnDot)
		}
		if dc, ok := depositColor(t); ok {
			fillRect(img, x+2, y+2, 4, 4, dc)
		}
	}

	return writePNG(img, path)
}

// spawnBuildable replicates — read-only — the terrain exclusion shared by
// validateMap's isBuildable and join.go's spawn query
// (terrain NOT IN coastal_sea, deep_sea, mountain_limestone, mountain_red,
// semi_desert). Kept here as a copy on purpose: this file must never become
// an import target for game logic.
func spawnBuildable(t Terrain) bool {
	switch t {
	case TerrainCoastalSea, TerrainDeepSea,
		TerrainMountainLimestone, TerrainMountainRed, TerrainSemiDesert:
		return false
	}
	return true
}

// EstimatePlayerCapacity greedily packs spawn-eligible hexes (spawnBuildable
// — the same terrain exclusion as join.go's capital-placement query) with a
// minimum hex distance from every already-picked hex, replicating join.go's
// clustering guard (api/handlers/join.go ~line 132-174: "at least 4 hexes
// from any existing settlement", i.e. NOT EXISTS a prior pick with hex
// distance <= 4). join.go itself is untouched — this is a read-only replica
// for CLI/JSON reporting only (plan §P4-B). Candidates are sorted by (q, r)
// before the greedy pass so the same tile set always yields the same count,
// independent of Go's map iteration order.
func EstimatePlayerCapacity(tiles []MapTile, width, height int) int {
	type c struct{ q, r int }
	candidates := make([]c, 0, len(tiles))
	for _, t := range tiles {
		if spawnBuildable(t.Terrain) {
			candidates = append(candidates, c{t.Q, t.R})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].q != candidates[j].q {
			return candidates[i].q < candidates[j].q
		}
		return candidates[i].r < candidates[j].r
	})

	var picked []c
	for _, cand := range candidates {
		tooClose := false
		for _, p := range picked {
			dq := cand.q - p.q
			dr := cand.r - p.r
			dist := (iAbs(dq) + iAbs(dq+dr) + iAbs(dr)) / 2
			if dist <= 4 {
				tooClose = true
				break
			}
		}
		if !tooClose {
			picked = append(picked, cand)
		}
	}
	return len(picked)
}

// ── Metrics ────────────────────────────────────────────────────────────────

// ComponentCompactness is the isoperimetric quotient of one land component:
// area divided by the area of the ideal hex-metric blob (a hexagon) with the
// same boundary-edge count. 1.0 = perfect hexagon. Today's hexDist blobs sit
// near max — P1's success is measured as this number dropping.
type ComponentCompactness struct {
	Tiles       int     `json:"tiles"`
	Compactness float64 `json:"compactness"`
}

// MapMetrics is the sidecar-JSON artifact contract (stable field set — reviews
// must be comparable across runs; this doubles as P4 calibration data).
// The diagnostics (compactness, neighbour mix) are REPORTED only and must
// never become validation gates.
type MapMetrics struct {
	RequestedSeed int64 `json:"requested_seed"`
	EffectiveSeed int64 `json:"effective_seed"`
	Attempts      int64 `json:"attempts"`
	Width         int   `json:"width"`
	Height        int   `json:"height"`
	Tiles         int   `json:"tiles"`

	LandFraction   float64 `json:"land_fraction"`
	LandComponents int     `json:"land_components"`
	// Largest land component as a fraction of ALL map tiles (not of land) —
	// the P4 gate ("largest landmass ≤ 15 % of map area") is expressed
	// against map area.
	LargestComponentFraction float64 `json:"largest_component_fraction"`

	SpawnValidTiles int `json:"spawn_valid_tiles"`
	CopperDeposits  int `json:"copper_deposits"`
	TinDeposits     int `json:"tin_deposits"`
	SilverDeposits  int `json:"silver_deposits"`
	CedarDeposits   int `json:"cedar_deposits"`
	Straits         int `json:"straits"`
	DeltaTiles      int `json:"delta_tiles"`

	// P4 calibration/capacity fields (plan §P4-B).
	TargetPlayers  int `json:"target_players"`  // playersFor(width, height)
	PlayerCapacity int `json:"player_capacity"` // greedy packing estimate, see EstimatePlayerCapacity
	// *Sources are connected components of same-metal deposit tiles — "how
	// many separate source clusters", not raw tile counts (those are the
	// *Deposits fields above).
	CopperSources int `json:"copper_sources"`
	TinSources    int `json:"tin_sources"`
	SilverSources int `json:"silver_sources"`
	// RiverValleyTiles is the river footprint (river_valley terrain count) —
	// P3 review data: river_valley is extra-fertile, so a bloated footprint
	// is a food-inflation signal even when every river has its delta.
	RiverValleyTiles int `json:"river_valley_tiles"`

	CompactnessPerComponent []ComponentCompactness `json:"compactness_per_component"`
	// Per terrain class: fraction of (tile, in-map neighbour) pairs where the
	// neighbour has the same terrain — captures leftover ring structure.
	TerrainNeighbourMix map[string]float64 `json:"terrain_neighbour_mix"`
}

// ComputeMapMetrics derives the geometric metrics from a tile set. Seed
// fields (RequestedSeed/EffectiveSeed/Attempts) are the caller's to fill —
// they are run parameters, not tile-set properties.
func ComputeMapMetrics(tiles []MapTile, width, height int) MapMetrics {
	m := MapMetrics{
		Width:               width,
		Height:              height,
		Tiles:               len(tiles),
		TerrainNeighbourMix: map[string]float64{},
	}

	terrain := make(map[[2]int]Terrain, len(tiles))
	for _, t := range tiles {
		terrain[[2]int{t.Q, t.R}] = t.Terrain
	}
	comp := landComponents(tiles)
	dirs6 := [6][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}

	land := 0
	compSize := map[int]int{}
	samePairs := map[Terrain]int{}
	totalPairs := map[Terrain]int{}
	for _, t := range tiles {
		k := [2]int{t.Q, t.R}
		if tileIsLand(t.Terrain) {
			land++
			compSize[comp[k]]++
		}
		if spawnBuildable(t.Terrain) {
			m.SpawnValidTiles++
		}
		if t.CopperDeposit {
			m.CopperDeposits++
		}
		if t.TinDeposit {
			m.TinDeposits++
		}
		if t.SilverDeposit {
			m.SilverDeposits++
		}
		if t.CedarDeposit {
			m.CedarDeposits++
		}
		if t.Terrain == TerrainRiverDelta {
			m.DeltaTiles++
		}
		if t.Terrain == TerrainRiverValley {
			m.RiverValleyTiles++
		}
		for _, d := range dirs6 {
			nt, ok := terrain[[2]int{t.Q + d[0], t.R + d[1]}]
			if !ok {
				continue // outside the map domain
			}
			totalPairs[t.Terrain]++
			if nt == t.Terrain {
				samePairs[t.Terrain]++
			}
		}
	}

	if len(tiles) > 0 {
		m.LandFraction = float64(land) / float64(len(tiles))
	}
	m.LandComponents = len(compSize)
	largest := 0
	for _, n := range compSize {
		if n > largest {
			largest = n
		}
	}
	if len(tiles) > 0 {
		m.LargestComponentFraction = float64(largest) / float64(len(tiles))
	}
	m.Straits = countStraits(tiles)

	m.TargetPlayers = playersFor(width, height)
	m.PlayerCapacity = EstimatePlayerCapacity(tiles, width, height)
	m.CopperSources = depositSourceCount(tiles, func(t MapTile) bool { return t.CopperDeposit })
	m.TinSources = depositSourceCount(tiles, func(t MapTile) bool { return t.TinDeposit })
	m.SilverSources = depositSourceCount(tiles, func(t MapTile) bool { return t.SilverDeposit })

	for t, total := range totalPairs {
		m.TerrainNeighbourMix[string(t)] = float64(samePairs[t]) / float64(total)
	}

	// Compactness per component: boundary-edge perimeter, then the ideal
	// hex-blob comparison. A perfect hexagon of radius k has area 3k²+3k+1
	// and perimeter 6(2k+1) boundary edges, so k = (P-6)/12 inverts the
	// perimeter and the quotient is area / idealArea(k). Map-boundary edges
	// count as perimeter (the edge is sea as far as shape reading goes).
	compPerim := map[int]int{}
	for k, id := range comp {
		for _, d := range dirs6 {
			n := [2]int{k[0] + d[0], k[1] + d[1]}
			nid, ok := comp[n]
			if !ok || nid != id {
				// Neighbour is sea, outside the map, or another component
				// (components never touch, but keep the check honest).
				compPerim[id]++
			}
		}
	}
	for id, area := range compSize {
		p := float64(compPerim[id])
		k := (p - 6) / 12
		ideal := 3*k*k + 3*k + 1
		m.CompactnessPerComponent = append(m.CompactnessPerComponent, ComponentCompactness{
			Tiles:       area,
			Compactness: float64(area) / ideal,
		})
	}
	// Deterministic order: biggest first. Equal (tiles, compactness) entries
	// are interchangeable, so this fully determines the JSON output.
	sort.Slice(m.CompactnessPerComponent, func(i, j int) bool {
		a, b := m.CompactnessPerComponent[i], m.CompactnessPerComponent[j]
		if a.Tiles != b.Tiles {
			return a.Tiles > b.Tiles
		}
		return a.Compactness > b.Compactness
	})

	return m
}

// WriteJSON writes the metrics as indented JSON — the sidecar artifact next
// to each PNG pair.
func (m MapMetrics) WriteJSON(path string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// ── Drawing helpers ────────────────────────────────────────────────────────

// newDebugImage allocates the canvas: width×height tiles plus a half tile of
// extra height for the odd-column offset, on a near-black background.
func newDebugImage(width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width*debugTilePx, height*debugTilePx+debugTilePx/2))
	fillRect(img, 0, 0, img.Rect.Dx(), img.Rect.Dy(), color.RGBA{0x10, 0x10, 0x10, 0xFF})
	return img
}

// debugTileOrigin maps a tile to its top-left pixel: x from q, y from the
// per-column row (r - rowOrigin), shifted half a tile down on odd columns —
// the brick layout matching the web renderer's y ∝ r + q/2.
func debugTileOrigin(q, r, width int) (x, y int) {
	row := r - rowOrigin(q, width)
	x = q * debugTilePx
	y = row * debugTilePx
	if q%2 != 0 {
		y += debugTilePx / 2
	}
	return x, y
}

// depositColor returns the dot color for a tile's deposit, if any. Deposits
// are mutually exclusive in practice; the order here just fixes a precedence.
func depositColor(t MapTile) (color.RGBA, bool) {
	switch {
	case t.CopperDeposit:
		return debugCopperColor, true
	case t.TinDeposit:
		return debugTinColor, true
	case t.SilverDeposit:
		return debugSilverColor, true
	case t.CedarDeposit:
		return debugCedarColor, true
	}
	return color.RGBA{}, false
}

func fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	for py := y; py < y+h; py++ {
		for px := x; px < x+w; px++ {
			img.SetRGBA(px, py, c)
		}
	}
}

// outlineRect draws a 1-px border just inside the rectangle.
func outlineRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	fillRect(img, x, y, w, 1, c)
	fillRect(img, x, y+h-1, w, 1, c)
	fillRect(img, x, y, 1, h, c)
	fillRect(img, x+w-1, y, 1, h, c)
}

func writePNG(img *image.RGBA, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
