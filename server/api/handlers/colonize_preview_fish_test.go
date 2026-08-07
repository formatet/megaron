package handlers

// AK5 (fisk-föder-befolkningen, 2026-07-31): the founding forecast for a hex
// with water in its catchment must show a higher net than the identical hex
// without water. This exercises the REAL catchment-query path ColonizePreview
// (world.go) uses — economy.CatchmentBasePotentialAt feeding
// economy.FoundingGrainNetPerTick — rather than only the pure-function claim
// already pinned in internal/economy/founding_forecast_test.go
// (TestFoundingGrainNetPerTick_FishRaisesNet).
//
// The second half of AK5 ("/colonize-preview + keryx visar samma tal som
// servern räknar") holds by construction and needs no test: cmd/keryx/
// cmd_unit.go's colonizePreview struct is a pure JSON mirror of this
// endpoint's response (Goods map[string]float64, Grain{BasePerTick,
// EstNetPerTick,...}) with zero independent computation — keryx cannot drift
// from the server because it never re-derives the numbers, only displays
// them.
//
// DB integration test (real Postgres, gated by DATABASE_URL via the shared
// armyDisplayTestPool helper).

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/hexgrid"
)

func TestColonizePreview_AK5_WaterInCatchmentRaisesForecastNet(t *testing.T) {
	pool := armyDisplayTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-preview-fish-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 100) RETURNING id`,
		"test-preview-fish-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	// Two candidate founding sites, far enough apart not to share a catchment.
	// Both have the SAME single plains centre — seeded for realism (a real
	// founding preview shows it) but excluded from the potential call below
	// (P1: the centre hex is not a production tile), so grain potential is 0
	// at both sites and any shortfall falls entirely on fish. The ONLY
	// difference is the 6th ring tile: mountain_limestone (no water) for site
	// A, coastal_sea (water) for site B — isolating the fish contribution
	// exactly as AK5 asks ("samma hex" with vs without water), instead of
	// trading away a grain tile for a water tile.
	// The centre tile is seeded (world.go's real endpoint reads it for
	// display) but deliberately NOT included in the returned hex list: P1
	// (megaron_plan_fysisk_gubbemodell.md §3.2) excludes the settlement's own
	// hex from production potential — hexgrid.Ring, not Disk — so a plains
	// centre must contribute NOTHING to grain here, matching what
	// RecomputeProduction will actually do once this site is founded.
	seedSite := func(centerQ int, sixthNeighborTerrain string) []hexgrid.Coord {
		centerR := 0
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain, coastal) VALUES ($1, $2, $3, 'plains', false)`,
			worldID, centerQ, centerR,
		); err != nil {
			t.Fatalf("seed center tile: %v", err)
		}
		var hexes []hexgrid.Coord
		neighborOffsets := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
		for i, d := range neighborOffsets {
			terrain := "mountain_limestone"
			if i == len(neighborOffsets)-1 {
				terrain = sixthNeighborTerrain
			}
			q, r := centerQ+d[0], centerR+d[1]
			if _, err := pool.Exec(ctx,
				`INSERT INTO map_tiles (world_id, q, r, terrain, coastal) VALUES ($1, $2, $3, $4, $5)`,
				worldID, q, r, terrain, terrain == "coastal_sea",
			); err != nil {
				t.Fatalf("seed ring tile: %v", err)
			}
			hexes = append(hexes, hexgrid.Coord{Q: q, R: r})
		}
		return hexes
	}

	noWaterHexes := seedSite(0, "mountain_limestone")
	withWaterHexes := seedSite(100, "coastal_sea")

	const forecastPop = economy.ColonyBaseFoundingPopulation

	noWaterGoods, err := economy.CatchmentBasePotentialAt(ctx, pool, worldID, noWaterHexes, nil)
	if err != nil {
		t.Fatalf("CatchmentBasePotentialAt (no water): %v", err)
	}
	withWaterGoods, err := economy.CatchmentBasePotentialAt(ctx, pool, worldID, withWaterHexes, nil)
	if err != nil {
		t.Fatalf("CatchmentBasePotentialAt (with water): %v", err)
	}

	if withWaterGoods["fish"] <= 0 {
		t.Fatalf("test fixture invariant broken: the water site must have fish base potential > 0, got %v",
			withWaterGoods["fish"])
	}
	if noWaterGoods["fish"] != 0 {
		t.Fatalf("test fixture invariant broken: the no-water site must have zero fish base potential, got %v",
			noWaterGoods["fish"])
	}

	_, netNoWater := economy.FoundingGrainNetPerTick(
		noWaterGoods["grain"], noWaterGoods["grain"], noWaterGoods["fish"], forecastPop, false)
	_, netWithWater := economy.FoundingGrainNetPerTick(
		withWaterGoods["grain"], withWaterGoods["grain"], withWaterGoods["fish"], forecastPop, false)

	if netWithWater <= netNoWater {
		t.Errorf("AK5: hex with water must forecast a higher net than the identical hex without — "+
			"no-water=%v, with-water=%v", netNoWater, netWithWater)
	}
}
