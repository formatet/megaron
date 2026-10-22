package economy

// Slice A regression guard (megaron_plan_skeppsreparation.md, §Beslut B1):
// splitting shipbuilding into a new `shipyard` building must NOT move fishing
// off the harbour. This must stay GREEN before and after the slice — the
// harbour still enables deep-water fishing independently of the shipyard.

import (
	"context"
	"testing"
)

func TestHarbourEnablesDeepFishWithoutShipyard(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// pop kept small (AK1's own rationale, recompute_fish_test.go): a single
	// fish hex's production must clearly exceed demand so fishRate nets
	// positive instead of being fully absorbed (AK3's fully-consumed, rate=0
	// case) — this test is about the harbour's capacity boost, not the
	// grain/fish demand-split formula.
	const tick = 200
	const pop = 60
	settlementID := recomputeWaterFixture(t, tick, pop /*grainTiles*/, 0 /*fishTiles*/, 1)

	if _, err := pool.Exec(ctx, `UPDATE map_tiles SET terrain='deep_sea' WHERE world_id=(SELECT world_id FROM settlements WHERE id=$1) AND terrain='coastal_sea'`, settlementID); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'harbour', 1)`,
		settlementID,
	); err != nil {
		t.Fatalf("create harbour building: %v", err)
	}

	// Final rule: a level-one harbour enables one deep-water fisher.
	capBefore, err := LoadHexCapacity(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("LoadHexCapacity: %v", err)
	}
	if capBefore["fish"] != 1 {
		t.Fatalf("fish hex cap with harbour built = %d, want %d (harbour must still boost fish capacity)", capBefore["fish"], 1)
	}

	// Staff the deep-water hex to its level-one cap and confirm production
	// actually flows — the harbour's fish role still functions end-to-end
	// through RecomputeProduction, not just the capacity table.
	placeHexGubbe(t, pool, settlementID, 1, recomputeWaterFixtureOffsets[0], "fish")

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	_, fishRate := readGood(t, settlementID, "fish")
	if fishRate <= 0 {
		t.Errorf("fish rate with harbour + 2 staffed gubbar = %v, want > 0 (harbour's fish role must be unaffected by the shipyard split)", fishRate)
	}
}
