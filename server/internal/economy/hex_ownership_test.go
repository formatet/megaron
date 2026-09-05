package economy

// Data-layer tests for §2 of megaron_plan_hexagarskap_och_stadsavstand.md:
// GlobalHexOccupancy itself, and — the slice's single most important
// measurement — proof that an ORDINARY, non-overlapping settlement's
// production is untouched by this change. See api/handlers/hex_ownership_test.go
// for the HTTP-level shared-hex-cap and race-guarantee proofs; this file
// stays at the economy package's own level (no HTTP, no auth).

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// TestGlobalHexOccupancy_MatchesSettlementScopedCountWhenAlone is the exact
// claim the plan's §5 demands: "produktionen för en oförändrad,
// icke-överlappande stad är EXAKT densamma före och efter." Before this
// slice, PlaceGubbe/PlacementOptions read occupancy via
// LoadPlacementCounts(settlementID) — settlement-scoped. After, the hex
// branch reads GlobalHexOccupancy(worldID, hexes) instead. For any
// settlement that is the ONLY one touching its own catchment hexes (still
// true of every settlement whose catchment has no neighbour within
// CatchmentClearanceHexes' reach, even after §3 lowered that minimum founding
// distance), the two numbers must be identical for every hex and every good.
// This test proves that arithmetic fact directly, not just via the full test
// suite staying green.
func TestGlobalHexOccupancy_MatchesSettlementScopedCountWhenAlone(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := seedFullRingFixture(t, 100, 1800, "forest_cedar")
	center := hexgrid.Coord{Q: 0, R: 0}
	ring := hexgrid.Ring(center, hexgrid.CatchmentRadius)

	// Fully staff every hex at the no-lumbermill cap (1) — same shape as
	// TestRecomputeProduction_HexSlotCapIsPopulationInvariant (P3).
	placeFullRing(t, pool, settlementID, center, "cedar", 1, 1)

	local, err := LoadPlacementCounts(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("LoadPlacementCounts: %v", err)
	}
	global, err := GlobalHexOccupancy(ctx, pool, mustWorldID(t, pool, settlementID), ring)
	if err != nil {
		t.Fatalf("GlobalHexOccupancy: %v", err)
	}

	if len(local.Hex) != len(global) {
		t.Fatalf("hex count differs: local=%d global=%d", len(local.Hex), len(global))
	}
	for _, hex := range ring {
		localCount := local.Hex[hex]["cedar"]
		globalCount := global[hex]["cedar"]
		if localCount != globalCount {
			t.Errorf("hex (%d,%d): local count=%d, global count=%d — must be identical when this settlement is the only occupant",
				hex.Q, hex.R, localCount, globalCount)
		}
	}
}

// TestRecomputeProduction_UnaffectedByAnotherSettlementElsewhereInTheWorld
// is the production-side half of the same claim: a SECOND settlement
// existing in the same world, fully staffing ITS OWN, entirely separate
// catchment, must not change the first settlement's RecomputeProduction
// output by even a fraction of a unit. This is the exact scenario the task
// calls "den farligaste tänkbara buggen" — this slice changes which rows a
// hex-capacity CHECK counts, and this test proves that change never reaches
// the production FORMULA for a settlement that shares no ground with anyone.
func TestRecomputeProduction_UnaffectedByAnotherSettlementElsewhereInTheWorld(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	settlementID := seedFullRingFixture(t, 100, 1800, "forest_cedar")
	center := hexgrid.Coord{Q: 0, R: 0}
	placeFullRing(t, pool, settlementID, center, "cedar", 1, 1)

	rateBefore := recomputeAndReadRate(t, pool, ctx, settlementID, "cedar")
	if rateBefore <= 0 {
		t.Fatalf("a fully-staffed 18-hex cedar catchment must produce a positive rate before the second settlement exists, got %v", rateBefore)
	}

	// A second settlement, far enough away (500 hexes) that its catchment
	// (radius CatchmentRadius) cannot possibly share a single tile with the
	// first — fully staffed identically, in the SAME world.
	worldID := mustWorldID(t, pool, settlementID)
	otherCenter := hexgrid.Coord{Q: 500, R: 0}
	otherSettlementID := seedSecondSettlement(t, pool, worldID, otherCenter, "forest_cedar", 1800)
	placeFullRing(t, pool, otherSettlementID, otherCenter, "cedar", 1, 1)
	if err := RecomputeProduction(ctx, pool, otherSettlementID); err != nil {
		t.Fatalf("RecomputeProduction (other settlement): %v", err)
	}

	rateAfter := recomputeAndReadRate(t, pool, ctx, settlementID, "cedar")
	if rateAfter != rateBefore {
		t.Errorf("cedar rate for the first settlement changed after an UNRELATED settlement was added to the same world: before=%v after=%v — production for a non-overlapping city must be exactly unchanged", rateBefore, rateAfter)
	}
}

func recomputeAndReadRate(t *testing.T, pool Tx, ctx context.Context, settlementID uuid.UUID, good string) float64 {
	t.Helper()
	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}
	var rate float64
	if err := pool.QueryRow(ctx,
		`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
		settlementID, good,
	).Scan(&rate); err != nil {
		t.Fatalf("read %s rate: %v", good, err)
	}
	return rate
}

func mustWorldID(t *testing.T, pool Tx, settlementID uuid.UUID) uuid.UUID {
	t.Helper()
	var worldID uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`SELECT world_id FROM settlements WHERE id = $1`, settlementID,
	).Scan(&worldID); err != nil {
		t.Fatalf("look up world_id for settlement: %v", err)
	}
	return worldID
}

// seedSecondSettlement adds another settlement (its own owner, province,
// catchment tiles and grain row) to an EXISTING world — seedFullRingFixture's
// sibling for tests that need two settlements sharing one world without
// sharing any ground.
func seedSecondSettlement(t *testing.T, pool Tx, worldID uuid.UUID, center hexgrid.Coord, terrain string, pop int) (settlementID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"p1balance-second-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create second player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, $4) RETURNING id`,
		worldID, center.Q, center.R, terrain,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create second province: %v", err)
	}

	for _, hex := range hexgrid.Ring(center, hexgrid.CatchmentRadius) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, hex.Q, hex.R, terrain,
		); err != nil {
			t.Fatalf("seed second ring tile (%d,%d): %v", hex.Q, hex.R, err)
		}
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Balanceville-2', 'achaean', $3, 'capital', true, $4) RETURNING id`,
		worldID, provinceID, ownerID, pop,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create second settlement: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'grain', 0, 0, 1000000, 100)`,
		settlementID,
	); err != nil {
		t.Fatalf("seed second settlement grain row: %v", err)
	}
	return settlementID
}
