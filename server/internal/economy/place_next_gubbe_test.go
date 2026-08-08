package economy

import (
	"context"
	"testing"
)

// TestPlaceNextGubbeOnBestFoodHex_PicksHighestYieldThenFallsToPool covers
// P0-UI answer 5 (LÅST 2026-08-07): the growth-time default places on the
// single best-yielding food slot with room, and once every food slot is full
// it correctly falls through to the pool (placed=false) instead of erroring
// or silently placing somewhere wrong.
func TestPlaceNextGubbeOnBestFoodHex_PicksHighestYieldThenFallsToPool(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// A settlement with ONE coastal_sea hex (fish, cap 1 without harbour) and
	// the rest mountain_limestone (no food at all) — a tightly bounded
	// scenario so "falls to pool once full" is reachable in one test.
	settlementID := recomputeWaterFixture(t, 100, 500, /*grainTiles*/ 0, /*fishTiles*/ 1)

	placed, err := PlaceNextGubbeOnBestFoodHex(ctx, pool, settlementID, 1)
	if err != nil {
		t.Fatalf("PlaceNextGubbeOnBestFoodHex: %v", err)
	}
	if !placed {
		t.Fatal("expected the first gubbe to be placed on the one fish hex, got placed=false")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM settlement_placement WHERE settlement_id = $1`, settlementID).Scan(&count); err != nil {
		t.Fatalf("count placements: %v", err)
	}
	if count != 1 {
		t.Fatalf("placements after gubbe 1 = %d, want 1", count)
	}

	// The fish hex's cap (1, no harbour) is now full — gubbe 2 must fall to
	// the pool (grain never applies here: recomputeWaterFixture's
	// grainTiles=0 means no grain-capable hex exists at all).
	placed2, err := PlaceNextGubbeOnBestFoodHex(ctx, pool, settlementID, 2)
	if err != nil {
		t.Fatalf("PlaceNextGubbeOnBestFoodHex (2nd): %v", err)
	}
	if placed2 {
		t.Error("expected the second gubbe to fall to the pool (fish hex already full, no grain hex exists), got placed=true")
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM settlement_placement WHERE settlement_id = $1`, settlementID).Scan(&count); err != nil {
		t.Fatalf("count placements: %v", err)
	}
	if count != 1 {
		t.Errorf("placements after gubbe 2 (should have fallen to pool) = %d, want still 1", count)
	}
}

// TestPlaceNextGubbeOnBestFoodHex_GrainNeverBlocksOnCapacity: with BOTH a
// grain hex and a fish hex available, grain's uncapped placementYield
// (placementYield's grain exemption) means an unbounded run of gubbe
// placements never falls to the pool via the grain hex — this is the fix for
// "a 1-gubbe city crossing 199→200 loses its 99 invisible auto-farmers."
func TestPlaceNextGubbeOnBestFoodHex_GrainNeverBlocksOnCapacity(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	settlementID := seedFullRingFixture(t, 100, 500, "plains")
	for ordinal := 1; ordinal <= 10; ordinal++ {
		placed, err := PlaceNextGubbeOnBestFoodHex(ctx, pool, settlementID, ordinal)
		if err != nil {
			t.Fatalf("PlaceNextGubbeOnBestFoodHex(%d): %v", ordinal, err)
		}
		if !placed {
			t.Fatalf("gubbe %d fell to the pool despite an uncapped grain catchment (18 plains hexes)", ordinal)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM settlement_placement WHERE settlement_id = $1 AND good_key = 'grain'`, settlementID).Scan(&count); err != nil {
		t.Fatalf("count grain placements: %v", err)
	}
	if count != 10 {
		t.Errorf("grain placements = %d, want 10 (every gubbe went to grain, uncapped)", count)
	}
}
