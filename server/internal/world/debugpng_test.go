package world

import (
	"bytes"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// syntheticBlobMap builds a 9×9 all-sea domain (same rowOrigin layout as the
// generator) with a perfect radius-1 hex blob of plains at the centre — the
// canonical "compactness = 1.0" shape. One deposit of each kind is flagged on
// blob tiles so the counters are exercised too.
func syntheticBlobMap() []MapTile {
	const w, h = 9, 9
	centre := cell{4, rowOrigin(4, w) + 4}
	blob := map[cell]bool{centre: true}
	for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}} {
		blob[cell{centre.q + d[0], centre.r + d[1]}] = true
	}

	var tiles []MapTile
	for q := 0; q < w; q++ {
		base := rowOrigin(q, w)
		for r := base; r < base+h; r++ {
			t := MapTile{Q: q, R: r, Terrain: TerrainDeepSea}
			if blob[cell{q, r}] {
				t.Terrain = TerrainPlains
			}
			tiles = append(tiles, t)
		}
	}
	// Deposits on known blob cells (flags only — terrain metrics unaffected).
	for i := range tiles {
		switch (cell{tiles[i].Q, tiles[i].R}) {
		case centre:
			tiles[i].CopperDeposit = true
		case cell{centre.q + 1, centre.r}:
			tiles[i].TinDeposit = true
		case cell{centre.q - 1, centre.r}:
			tiles[i].SilverDeposit = true
		case cell{centre.q, centre.r + 1}:
			tiles[i].CedarDeposit = true
		}
	}
	return tiles
}

func TestComputeMapMetrics_SyntheticHexBlob(t *testing.T) {
	tiles := syntheticBlobMap()
	m := ComputeMapMetrics(tiles, 9, 9)

	if m.Tiles != 81 {
		t.Fatalf("tiles = %d, want 81", m.Tiles)
	}
	if got, want := m.LandFraction, 7.0/81.0; math.Abs(got-want) > 1e-12 {
		t.Errorf("land_fraction = %v, want %v", got, want)
	}
	if m.LandComponents != 1 {
		t.Errorf("land_components = %d, want 1", m.LandComponents)
	}
	if got, want := m.LargestComponentFraction, 7.0/81.0; math.Abs(got-want) > 1e-12 {
		t.Errorf("largest_component_fraction = %v, want %v", got, want)
	}
	// 7 plains tiles are the only buildable terrain (sea is excluded).
	if m.SpawnValidTiles != 7 {
		t.Errorf("spawn_valid_tiles = %d, want 7", m.SpawnValidTiles)
	}
	if m.CopperDeposits != 1 || m.TinDeposits != 1 || m.SilverDeposits != 1 || m.CedarDeposits != 1 {
		t.Errorf("deposit counts = cu%d sn%d ag%d cedar%d, want 1 each",
			m.CopperDeposits, m.TinDeposits, m.SilverDeposits, m.CedarDeposits)
	}
	if m.DeltaTiles != 0 {
		t.Errorf("delta_tiles = %d, want 0", m.DeltaTiles)
	}
	// A convex radius-1 blob leaves no sea hex flanked by REAL land on an
	// opposing axis pair — but countStraits (existing generator code, reused
	// as-is) treats out-of-domain lookups as land (the zero-value Terrain ""
	// is not sea), so two sheared-corner sea cells at the map boundary count
	// as straits on this 9×9 domain. Documenting existing behaviour here,
	// not endorsing it.
	if m.Straits != 2 {
		t.Errorf("straits = %d, want 2 (boundary artifact of countStraits)", m.Straits)
	}

	// A perfect radius-1 hexagon IS the isoperimetric optimum: quotient 1.0.
	if len(m.CompactnessPerComponent) != 1 {
		t.Fatalf("compactness entries = %d, want 1", len(m.CompactnessPerComponent))
	}
	cc := m.CompactnessPerComponent[0]
	if cc.Tiles != 7 {
		t.Errorf("component tiles = %d, want 7", cc.Tiles)
	}
	if math.Abs(cc.Compactness-1.0) > 1e-12 {
		t.Errorf("compactness = %v, want 1.0", cc.Compactness)
	}

	// Neighbour mix for plains: centre sees 6/6 same, each ring tile 3/6
	// (centre + 2 ring neighbours) → (6 + 6·3) / 42.
	if got, want := m.TerrainNeighbourMix[string(TerrainPlains)], 24.0/42.0; math.Abs(got-want) > 1e-12 {
		t.Errorf("terrain_neighbour_mix[plains] = %v, want %v", got, want)
	}
}

func TestComputeMapMetrics_SingleDeltaTile(t *testing.T) {
	const w, h = 5, 5
	var tiles []MapTile
	centre := cell{2, rowOrigin(2, w) + 2}
	for q := 0; q < w; q++ {
		base := rowOrigin(q, w)
		for r := base; r < base+h; r++ {
			terrain := TerrainDeepSea
			if (cell{q, r}) == centre {
				terrain = TerrainRiverDelta
			}
			tiles = append(tiles, MapTile{Q: q, R: r, Terrain: terrain})
		}
	}
	m := ComputeMapMetrics(tiles, w, h)
	if m.DeltaTiles != 1 {
		t.Errorf("delta_tiles = %d, want 1", m.DeltaTiles)
	}
	if m.LandComponents != 1 {
		t.Errorf("land_components = %d, want 1", m.LandComponents)
	}
	// river_delta is buildable terrain (not on the exclusion list).
	if m.SpawnValidTiles != 1 {
		t.Errorf("spawn_valid_tiles = %d, want 1", m.SpawnValidTiles)
	}
	// A single tile is trivially the optimal blob (k=0).
	if len(m.CompactnessPerComponent) != 1 || math.Abs(m.CompactnessPerComponent[0].Compactness-1.0) > 1e-12 {
		t.Errorf("compactness = %+v, want one entry of 1.0", m.CompactnessPerComponent)
	}
}

// Same seed → identical tile slice (guaranteed by TestGenerateMap_Deterministic)
// → byte-identical PNGs, overlays and metrics. This is what makes debug
// artifacts comparable across machines and runs.
func TestDebugExport_Deterministic(t *testing.T) {
	const seed, w, h = int64(7), 30, 20
	tiles := genTiles(seed, w, h)
	dir := t.TempDir()

	render := func(suffix string, f func(string) error) []byte {
		t.Helper()
		path := filepath.Join(dir, "map"+suffix)
		if err := f(path); err != nil {
			t.Fatalf("export %s: %v", suffix, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", suffix, err)
		}
		return data
	}

	a := render("_a.png", func(p string) error { return ExportDebugPNG(tiles, w, h, p) })
	b := render("_b.png", func(p string) error { return ExportDebugPNG(tiles, w, h, p) })
	if !bytes.Equal(a, b) {
		t.Error("ExportDebugPNG is not deterministic for identical tiles")
	}

	oa := render("_oa.png", func(p string) error { return ExportDebugOverlayPNG(tiles, w, h, p) })
	ob := render("_ob.png", func(p string) error { return ExportDebugOverlayPNG(tiles, w, h, p) })
	if !bytes.Equal(oa, ob) {
		t.Error("ExportDebugOverlayPNG is not deterministic for identical tiles")
	}

	// Decoded dimensions match the brick layout: w×8 by h×8 + half tile.
	img, err := png.Decode(bytes.NewReader(a))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if img.Bounds().Dx() != w*debugTilePx || img.Bounds().Dy() != h*debugTilePx+debugTilePx/2 {
		t.Errorf("image = %dx%d, want %dx%d",
			img.Bounds().Dx(), img.Bounds().Dy(), w*debugTilePx, h*debugTilePx+debugTilePx/2)
	}

	m1 := ComputeMapMetrics(tiles, w, h)
	m2 := ComputeMapMetrics(tiles, w, h)
	if !reflect.DeepEqual(m1, m2) {
		t.Error("ComputeMapMetrics is not deterministic for identical tiles")
	}
}
