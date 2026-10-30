package world

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestTinAdditionalBudgetScalesWithArea(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ w, h, want int }{{32, 32, 1}, {56, 40, 1}, {64, 64, 2}, {128, 128, 8}, {230, 230, 26}} {
		if got := tinAdditionalSourceTarget(tc.w, tc.h); got != tc.want {
			t.Fatalf("%dx%d additional source budget=%d want%d", tc.w, tc.h, got, tc.want)
		}
	}
}

// legacyTile is MapTile as it was when testdata/tin-increase-2be784c2.json was
// recorded: without IncenseDeposit (order 32), whose own placement is covered by
// mapgen_incense_test.go. Hashing this shape keeps the recording valid and still
// proves the incense pass moved no other field.
type legacyTile struct {
	WorldID                                                uuid.UUID
	Q, R                                                   int
	Terrain                                                Terrain
	Coastal                                                bool
	Fertility, Mineral                                     float64
	CopperDeposit, TinDeposit, SilverDeposit, CedarDeposit bool
}

// Recorded unmodified 2be784c2 maps, with exact candidate counts measured in
// isolated source copies BEFORE implementation. This checks geography rather
// than comparing two calls through the same new helper.
func TestGenerateMap_TinIncreasePreservesGeography(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/tin-increase-2be784c2.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Size      int      `json:"size"`
		Requested int64    `json:"requested_seed"`
		Effective int64    `json:"effective_seed"`
		Hash      string   `json:"non_tin_sha256"`
		OldTin    [][3]int `json:"tin"`
		Hexes     int      `json:"expected_tin_hexes"`
		Sources   int      `json:"expected_tin_sources"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 40 {
		t.Fatalf("golden maps=%d want40", len(cases))
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%d/seed%d", tc.Size, tc.Requested), func(t *testing.T) {
			tiles, eff := GenerateMap(stubID{}, tc.Requested, tc.Size, tc.Size)
			if eff != tc.Effective {
				t.Fatalf("effective seed%d want%d", eff, tc.Effective)
			}
			if err := validateMap(tiles, tc.Size, tc.Size); err != nil {
				t.Fatal(err)
			}
			// The golden predates lakes (insjöar slice 2): hash the map before
			// MarkLakes relabels enclosed sea, and prove that is the only step.
			pre := generateMapOnce(stubID{}, eff, tc.Size, tc.Size)
			addTinSources(pre, eff, tc.Size, tc.Size)
			AddIncenseSources(pre, eff, tc.Size, tc.Size)
			PopulateRiverMetadata(pre, tc.Size, tc.Size)
			relabelled := append([]MapTile(nil), pre...)
			MarkLakes(relabelled, tc.Size, tc.Size)
			if !reflect.DeepEqual(relabelled, tiles) {
				t.Fatal("GenerateMap differs from the pre-lake map beyond MarkLakes")
			}
			tiles = pre
			h := sha256.New()
			enc := json.NewEncoder(h)
			tin := map[[2]int]bool{}
			hexes := 0
			for _, tile := range tiles {
				if tile.TinDeposit {
					tin[[2]int{tile.Q, tile.R}] = true
					hexes++
					if tile.Terrain != TerrainMountainLimestone {
						t.Fatal("tin on wrong terrain")
					}
				}
				tile.TinDeposit = false
				if err := enc.Encode(legacyTile{tile.WorldID, tile.Q, tile.R, tile.Terrain, tile.Coastal, tile.Fertility, tile.Mineral,
					tile.CopperDeposit, tile.TinDeposit, tile.SilverDeposit, tile.CedarDeposit}); err != nil {
					t.Fatal(err)
				}
			}
			if hash := fmt.Sprintf("%x", h.Sum(nil)); hash != tc.Hash {
				t.Fatal("a non-tin field changed from recorded master")
			}
			for _, old := range tc.OldTin {
				if !tin[[2]int{old[0], old[1]}] {
					t.Fatal("an old tin deposit was moved/removed")
				}
			}
			if hexes != tc.Hexes {
				t.Fatalf("tin hexes=%d want measured%d", hexes, tc.Hexes)
			}
			if sources := depositSourceCount(tiles, func(t MapTile) bool { return t.TinDeposit }); sources != tc.Sources {
				t.Fatalf("tin sources=%d want measured%d", sources, tc.Sources)
			}
		})
	}
}
