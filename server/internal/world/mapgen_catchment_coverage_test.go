package world

import (
	"fmt"
	"math"
	"sort"
	"testing"
)

// ── Catchment coverage: the gap TestGenerateMap_TinAndSilverDepositsAreReachable
// leaves open ─────────────────────────────────────────────────────────────────
//
// That test asks whether a deposit has ANY buildable hex within mineableRadius
// — i.e. whether the map could, in principle, be mined by someone. It cannot
// see whether the settlements a world actually gets can reach the metal:
//
//   - a deposit whose only nearby buildable ground is food-dead (Gournia in the
//     drift world: 10 olive grove + 6 mountain limestone, no grain and no fish
//     terrain, permanently stuck at the 101-inhabitant collapse floor,
//     megaron_dagsverkesskalan.md §3c) passes reachability and yields nothing;
//   - two deposits inside ONE catchment pass reachability twice and still make
//     only one silver city;
//   - a metal whose source count is a tenth of the player count passes
//     reachability on every tile and still leaves 90 % of cities without it.
//
// So this file measures SITES, not tiles: how many mutually independent
// settlements (>= 5 hexes apart, join.go's clustering guard) could each have
// the metal inside their 19-hex catchment AND feed themselves. Target:
// megaron_plan_dagsverkesskalan.md §5, "cirka 25-40 % av spelbara städer har
// lokal silvertillgång ... och det finns minst två geografiskt skilda vägar
// till silver". Baseline before the fix (2026-08-27): 14-17 % at 230x230.

// minSilverSiteShare is the floor of §5's 25-40 % band. Sources are placed at
// silverSourceTarget = players/3, so a healthy map lands around a third; the
// gate fires when placement wastes sources rather than when the target is off.
const minSilverSiteShare = 0.25

// foodTerrain lists the terrains that carry a grain or fish production rule
// (grain: plains, hills, river_valley, river_delta — mig 008/027/043/062;
// fish: coastal_sea, deep_sea, river, river_ford — mig 101/108). A site with
// none of them in catchment cannot feed even one gubbe from its own ground.
// Deliberately a terrain list, not a rate lookup: internal/world may not read
// the database, and the question here is "is there any food land at all",
// which survives every rate recalibration.
func foodTerrain(t Terrain) bool {
	switch t {
	case TerrainPlains, TerrainHills, TerrainRiverValley, TerrainRiverDelta,
		TerrainCoastalSea, TerrainDeepSea, TerrainRiver, TerrainRiverFord:
		return true
	}
	return false
}

// coverageMap is one generated world indexed for site questions.
type coverageMap struct {
	terrain map[cell]Terrain
	sites   []cell // spawn-buildable hexes, (q,r)-sorted for determinism
	viable  map[cell]bool
}

func newCoverageMap(tiles []MapTile) *coverageMap {
	cm := &coverageMap{
		terrain: make(map[cell]Terrain, len(tiles)),
		viable:  map[cell]bool{},
	}
	for _, t := range tiles {
		cm.terrain[cell{t.Q, t.R}] = t.Terrain
	}
	for _, t := range tiles {
		if spawnBuildable(t.Terrain) {
			cm.sites = append(cm.sites, cell{t.Q, t.R})
		}
	}
	sort.Slice(cm.sites, func(i, j int) bool {
		if cm.sites[i].q != cm.sites[j].q {
			return cm.sites[i].q < cm.sites[j].q
		}
		return cm.sites[i].r < cm.sites[j].r
	})
	for _, c := range cm.sites {
		for _, n := range catchment(c) {
			if t, ok := cm.terrain[n]; ok && foodTerrain(t) {
				cm.viable[c] = true
				break
			}
		}
	}
	return cm
}

// catchment is the settlement's own hex plus everything within
// mineableRadius (= hexgrid.CatchmentRadius) — the 19 hexes it produces from.
func catchment(c cell) []cell {
	out := make([]cell, 0, 19)
	for dq := -mineableRadius; dq <= mineableRadius; dq++ {
		for dr := -mineableRadius; dr <= mineableRadius; dr++ {
			n := cell{c.q + dq, c.r + dr}
			if hexDist(c, n) <= mineableRadius {
				out = append(out, n)
			}
		}
	}
	return out
}

func depositCells(tiles []MapTile, has func(MapTile) bool) []cell {
	var out []cell
	for _, t := range tiles {
		if has(t) {
			out = append(out, cell{t.Q, t.R})
		}
	}
	return out
}

func hasInCatchment(c cell, deps []cell) bool {
	for _, d := range deps {
		if hexDist(c, d) <= mineableRadius {
			return true
		}
	}
	return false
}

// metalSites greedily packs the food-viable sites that hold the metal in
// catchment, keeping them >= 5 hexes apart (join.go: a candidate within 4 of
// an existing settlement is rejected). The result is "how many separate
// cities this world could give this metal to", not how many hexes carry it.
func (cm *coverageMap) metalSites(deps []cell) []cell {
	var picked []cell
	for _, c := range cm.sites {
		if !cm.viable[c] || !hasInCatchment(c, deps) {
			continue
		}
		clear := true
		for _, p := range picked {
			if hexDist(c, p) <= 4 {
				clear = false
				break
			}
		}
		if clear {
			picked = append(picked, c)
		}
	}
	return picked
}

func landmassesWithDeposit(tiles []MapTile, has func(MapTile) bool) int {
	comp := landComponents(tiles)
	seen := map[int]bool{}
	for _, t := range tiles {
		if has(t) {
			if lm, ok := comp[[2]int{t.Q, t.R}]; ok {
				seen[lm] = true
			}
		}
	}
	return len(seen)
}

var (
	isSilver = func(t MapTile) bool { return t.SilverDeposit }
	isCopper = func(t MapTile) bool { return t.CopperDeposit }
	isTin    = func(t MapTile) bool { return t.TinDeposit }
)

// coverageSizes are the three plan sizes. 60x60 is the drift world's own size
// (CT126, 2026-08-27) and sits on the 10-player floor.
var coverageSizes = [][2]int{{60, 60}, {120, 84}, {230, 230}}

// distinctSeeds walks requested seeds and yields only maps with distinct
// EFFECTIVE seeds — GenerateMap reseeds until validateMap passes, so
// consecutive requested seeds routinely collapse onto the same world and
// would otherwise be counted as independent samples.
func distinctSeeds(t *testing.T, w, h int, want int, fn func(seed int64, tiles []MapTile)) {
	t.Helper()
	seen := map[int64]bool{}
	for seed := int64(0); seed < 40 && len(seen) < want; seed++ {
		tiles, eff := GenerateMap(stubID{}, seed, w, h)
		if seen[eff] {
			continue
		}
		seen[eff] = true
		fn(eff, tiles)
	}
	if len(seen) < want {
		t.Fatalf("%dx%d: only %d distinct maps out of 40 requested seeds, want %d", w, h, len(seen), want)
	}
}

// TestGenerateMap_SilverReachesEnoughCities is the catchment-coverage gate:
// silver must be reachable by a share of the world's intended settlements,
// from at least two separate landmasses, with no two sources spent on one
// city. Reachability on the map (TestGenerateMap_TinAndSilverDepositsAreReachable)
// is necessary but not sufficient — see this file's header.
func TestGenerateMap_SilverReachesEnoughCities(t *testing.T) {
	for _, sz := range coverageSizes {
		w, h := sz[0], sz[1]
		players := playersFor(w, h)
		want := int(math.Ceil(minSilverSiteShare * float64(players)))
		distinctSeeds(t, w, h, 3, func(seed int64, tiles []MapTile) {
			cm := newCoverageMap(tiles)
			deps := depositCells(tiles, isSilver)
			sites := cm.metalSites(deps)
			if len(sites) < want {
				t.Errorf("%dx%d eff seed %d: %d food-viable independent silver city sites (%d silver hexes, %d sources), want >= %d (%.0f%% of %d players)",
					w, h, seed, len(sites), len(deps), depositSourceCount(tiles, isSilver), want, 100*minSilverSiteShare, players)
			}
			if lm := landmassesWithDeposit(tiles, isSilver); lm < 2 {
				t.Errorf("%dx%d eff seed %d: silver on %d landmass(es), want >= 2 — one blocked city must not decide the world's silver",
					w, h, seed, lm)
			}
			// Two silver hexes within 2*mineableRadius of each other fall in
			// one settlement's catchment: one silver city, two sources spent.
			// silverSourceSpacing exists to prevent exactly this, and a silver
			// source is one hex (silverClusterMax), so any such pair is waste
			// rather than a legitimately wide district.
			for i := 0; i < len(deps); i++ {
				for j := i + 1; j < len(deps); j++ {
					if d := hexDist(deps[i], deps[j]); d <= 2*mineableRadius {
						t.Errorf("%dx%d eff seed %d: silver at (%d,%d) and (%d,%d) are %d apart — both fall in one settlement's catchment, spending two sources on one city",
							w, h, seed, deps[i].q, deps[i].r, deps[j].q, deps[j].r, d)
					}
				}
			}
		})
	}
}

// TestGenerateMap_NoCityCanHoldBothCopperAndTin guards the property the drift
// world already has and that makes bronze a good of geography rather than of
// rule (megaron_dagsverkesskalan.md §3c): the two halves of bronze never meet
// inside one catchment, so every bronze chain needs two cities.
func TestGenerateMap_NoCityCanHoldBothCopperAndTin(t *testing.T) {
	for _, sz := range coverageSizes {
		w, h := sz[0], sz[1]
		distinctSeeds(t, w, h, 3, func(seed int64, tiles []MapTile) {
			cm := newCoverageMap(tiles)
			cu := depositCells(tiles, isCopper)
			sn := depositCells(tiles, isTin)
			for _, c := range cm.sites {
				if hasInCatchment(c, cu) && hasInCatchment(c, sn) {
					t.Errorf("%dx%d eff seed %d: site (%d,%d) has both copper and tin in catchment — bronze would need no trade",
						w, h, seed, c.q, c.r)
				}
			}
		})
	}
}

// TestReport_CatchmentDepositCoverage prints the measurement the two gates
// above assert on. Run with -v; it asserts nothing on purpose (arbetssätt §4:
// the tool that produced the baseline lives in the repo, not in a scratchpad).
func TestReport_CatchmentDepositCoverage(t *testing.T) {
	for _, sz := range coverageSizes {
		w, h := sz[0], sz[1]
		players := playersFor(w, h)
		fmt.Printf("\n=== %dx%d  players=%d  källmål: Cu=%d Ag=%d Sn=%d ===\n",
			w, h, players, copperSourceTarget(players), silverSourceTarget(players), tinSourceTarget(players))
		fmt.Printf("%-12s %8s | %-26s | %-26s | %-26s\n",
			"eff seed", "platser", "Ag hexar/källor/land/städer", "Cu hexar/källor/land/städer", "Sn hexar/källor/land/städer")
		distinctSeeds(t, w, h, 4, func(seed int64, tiles []MapTile) {
			cm := newCoverageMap(tiles)
			row := func(has func(MapTile) bool) string {
				deps := depositCells(tiles, has)
				return fmt.Sprintf("%d/%d/%d/%d (%.0f%% av spelarna)", len(deps), depositSourceCount(tiles, has),
					landmassesWithDeposit(tiles, has), len(cm.metalSites(deps)),
					100*float64(len(cm.metalSites(deps)))/float64(players))
			}
			var viable int
			for _, c := range cm.sites {
				if cm.viable[c] {
					viable++
				}
			}
			fmt.Printf("%-12d %8d | %-26s | %-26s | %-26s\n", seed, viable, row(isSilver), row(isCopper), row(isTin))
		})
	}
}
