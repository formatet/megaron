package economy

// P3 (megaron_plan_fysisk_gubbemodell.md §8.3): a catchment hex has a finite
// number of worker stations too — an absolute headcount, not a share of the
// city, exactly like P2 did for buildings. This is the integration-level
// rött-före: RecomputeProduction end-to-end (LoadHexCapacity → LaborCapacity
// → RecomputeProduction), not just the LaborCapacity unit tests in
// labor_capacity_test.go. Cedar via forest_cedar is used because it is
// terrain-gated with no deposit requirement (unlike copper/tin/silver) and no
// building is built in this fixture, isolating the hex term cleanly.

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
)

// TestRecomputeProduction_HexSlotCapIsPopulationInvariant mirrors
// TestRecomputeProduction_BuildingSlotCapIsPopulationInvariant (P2) for the
// hex term: an 18-hex forest_cedar catchment (no lumbermill) caps at
// hexCapacityRule{"cedar",1,2,"lumbermill"}'s capNoBuilding=1 slot PER HEX ×
// 18 hexes = 18 total slots, regardless of population.
func TestRecomputeProduction_HexSlotCapIsPopulationInvariant(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 100

	rateAt := func(pop int) float64 {
		settlementID := seedFullRingFixture(t, tick, pop, "forest_cedar")
		// Full staffing under P4 = one gubbe per hex-slot, not a weight — the
		// no-lumbermill cap is capNoBuilding=1 (hexCapacityRule{"cedar",1,2,...}).
		placeFullRing(t, pool, settlementID, hexgrid.Coord{Q: 0, R: 0}, "cedar", 1, 1)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		defer tx.Rollback(ctx)
		if err := RecomputeProduction(ctx, tx, settlementID); err != nil {
			t.Fatalf("RecomputeProduction: %v", err)
		}
		var rate float64
		if err := tx.QueryRow(ctx,
			`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'cedar'`,
			settlementID,
		).Scan(&rate); err != nil {
			t.Fatalf("read cedar rate: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
		return rate
	}

	small := rateAt(100)
	large := rateAt(5000)

	if small <= 0 {
		t.Fatalf("an 18-hex forest_cedar catchment staffed at full weight must produce SOME cedar, got %v", small)
	}
	// 50x the population, weight and terrain unchanged. Without a hex cap this
	// rate would scale ~50x too (GoodLaborTerrainBase was a flat share of
	// laborPool); with the cap both settlements are bounded at the SAME 18
	// hex-slots, so the rate must be identical regardless of city size.
	if diff := large - small; diff > 1e-6*small || diff < -1e-6*small {
		t.Errorf("cedar rate must be population-invariant once capped by hex slots: "+
			"pop=100 gave %v, pop=5000 gave %v (a %.1fx difference — the hex cap did not hold)",
			small, large, large/small)
	}
}

// TestLoadHexCapacity_BuildingRaisesCap — the "med byggnad" half of §8.3: a
// lumbermill in the settlement must raise cedar's per-hex cap from 1 to 2,
// doubling the total (18 → 36 for an 18-hex forest_cedar catchment).
func TestLoadHexCapacity_BuildingRaisesCap(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 100

	settlementID := seedFullRingFixture(t, tick, 500, "forest_cedar")
	before, err := LoadHexCapacity(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("LoadHexCapacity (no building): %v", err)
	}
	if before["cedar"] != 18 {
		t.Fatalf("18 forest_cedar hexes with no lumbermill should give 18 cedar slots, got %d", before["cedar"])
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'lumbermill', 1)`,
		settlementID,
	); err != nil {
		t.Fatalf("seed lumbermill: %v", err)
	}
	after, err := LoadHexCapacity(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("LoadHexCapacity (with lumbermill): %v", err)
	}
	if after["cedar"] != 36 {
		t.Fatalf("18 forest_cedar hexes WITH a lumbermill should give 36 cedar slots (2 per hex), got %d", after["cedar"])
	}
}
