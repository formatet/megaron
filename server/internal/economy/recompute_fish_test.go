package economy

// DB integration tests for fisk-föder-befolkningen (2026-07-31): AK1-AK3 from
// the slice contract, proved end-to-end through RecomputeProduction against a
// real catchment (not just the pure FoodConsumptionSplit formula — see
// food_consumption_split_test.go for that half). Gated by DATABASE_URL via
// testPool (sitos_conservation_test.go), same as every other recompute test
// in this package.

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// recomputeWaterFixture builds an active world + one settlement whose 7-hex
// catchment mixes terrains freely for the grain/fish invariant tests:
// grainTiles 'plains' (grain field, no building needed — mig 008) and
// fishTiles 'coastal_sea' (fish field, no building needed — mig 101) among
// the 6 neighbour offsets recomputeFixture also uses; remaining neighbour
// slots are filled with inert 'mountain_limestone' (produces nothing but the
// universal timber trickle, matching recomputeFixture's filler terrain). The
// center hex is deliberately left untiled, exactly like recomputeFixture, so
// it never contributes — grainTiles+fishTiles must be <= 6.
func recomputeWaterFixture(t *testing.T, currentTick, pop, grainTiles, fishTiles int) (settlementID uuid.UUID) {
	t.Helper()
	if grainTiles+fishTiles > 6 {
		t.Fatalf("recomputeWaterFixture: grainTiles(%d)+fishTiles(%d) > 6 neighbour slots", grainTiles, fishTiles)
	}
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-recompute-water-%'`,
	); err != nil {
		t.Fatalf("archive leftover test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-recompute-water-"+uuid.New().String(), currentTick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"recompute-water-"+uuid.New().String(), "recompute-water-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'mountain_limestone') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}

	offsets := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
	for i, d := range offsets {
		terrain := "mountain_limestone"
		coastal := false
		switch {
		case i < grainTiles:
			terrain = "plains"
		case i < grainTiles+fishTiles:
			terrain = "coastal_sea"
			coastal = true
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain, coastal) VALUES ($1, $2, $3, $4, $5)`,
			worldID, d[0], d[1], terrain, coastal,
		); err != nil {
			t.Fatalf("seed catchment tile %d (%s): %v", i, terrain, err)
		}
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Halieville', 'achaean', $3, 'capital', true, $4) RETURNING id`,
		worldID, provinceID, ownerID, pop,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	return settlementID
}

// TestRecomputeProduction_AK1_PureFishCatchment_ZeroGrainNotNegative: a
// catchment that is ENTIRELY water (no plains at all) gives fish production
// far above the population's food need. Grain must settle at rate EXACTLY 0
// (never negative — rate=0 is the proof the stock can never be drawn down by
// this settlement, whatever the elapsed ticks), and fish's net rate must be
// its own production minus demand.
func TestRecomputeProduction_AK1_PureFishCatchment_ZeroGrainNotNegative(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	const tick = 200
	const pop = 1000
	settlementID := recomputeWaterFixture(t, tick, pop, /*grainTiles*/ 0, /*fishTiles*/ 6)

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	grainAmount, grainRate := readGood(t, settlementID, "grain")
	if grainRate != 0 {
		t.Errorf("AK1: grain rate must be exactly 0 with zero grain production, got %v", grainRate)
	}
	if grainAmount != 0 {
		t.Errorf("AK1: grain amount should still be 0 (none was ever produced or seeded), got %v", grainAmount)
	}

	fishAmount, fishRate := readGood(t, settlementID, "fish")
	if fishRate <= 0 {
		t.Fatalf("AK1: fish must net-produce (production exceeds demand), got rate=%v", fishRate)
	}
	_ = fishAmount

	// "Grain-lagret sjunker inte": advance the world clock and confirm the
	// grain row's PROJECTED stock (settled(), the same function every read
	// path uses) is unchanged — rate=0 makes this true for any elapsed ticks.
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick = $1 WHERE id = (
		SELECT world_id FROM settlements WHERE id = $2)`, tick+500, settlementID); err != nil {
		t.Fatalf("advance world tick: %v", err)
	}
	var projected float64
	if err := pool.QueryRow(ctx,
		`SELECT settled(amount, rate, calc_tick) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'`,
		settlementID,
	).Scan(&projected); err != nil {
		t.Fatalf("read projected grain: %v", err)
	}
	if projected != 0 {
		t.Errorf("AK1: grain stock must not sink after 500 ticks elapse, got %v", projected)
	}
}

// TestRecomputeProduction_AK2_NoFishBehavesLikePreSlice: a settlement whose
// catchment has plains but NO water at all must reproduce the pre-slice
// formula exactly — grain rate = grainProd - demand, and no fish row is
// written at all (not even a rate=0 one) since fish never appears in the
// potentials set for a landlocked catchment.
func TestRecomputeProduction_AK2_NoFishBehavesLikePreSlice(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	const tick = 200
	const pop = 500
	settlementID := recomputeWaterFixture(t, tick, pop, /*grainTiles*/ 3, /*fishTiles*/ 0)

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	var grainBase float64
	if err := pool.QueryRow(ctx,
		`SELECT SUM(pr.rate_per_tick) FROM map_tiles mt
		 JOIN production_rules pr ON pr.terrain_type = mt.terrain AND pr.good_key = 'grain' AND pr.building_type IS NULL
		 JOIN settlements s ON s.id = $1
		 JOIN provinces p ON p.id = s.province_id
		 WHERE mt.world_id = p.world_id
		   AND ((mt.q = p.map_q AND mt.r = p.map_r) OR
		        (mt.q = p.map_q+1 AND mt.r = p.map_r) OR (mt.q = p.map_q-1 AND mt.r = p.map_r) OR
		        (mt.q = p.map_q AND mt.r = p.map_r+1) OR (mt.q = p.map_q AND mt.r = p.map_r-1) OR
		        (mt.q = p.map_q+1 AND mt.r = p.map_r-1) OR (mt.q = p.map_q-1 AND mt.r = p.map_r+1))`,
		settlementID,
	).Scan(&grainBase); err != nil {
		t.Fatalf("read grain base potential: %v", err)
	}

	// Weight auto-seeds to 1/n across whatever potentials this catchment
	// matches — plains alone also unlocks oil/livestock unconditionally and
	// mountain_limestone unlocks stone/tin unconditionally (plus the
	// universal timber trickle), so n is NOT simply {grain, timber}. Read the
	// actual seeded weight rather than assuming n, so this test only pins the
	// AK2 claim (grain rate = grainProd - demand, exactly like pre-slice) and
	// not an unrelated assumption about the production_rules catalog's shape.
	// LaborCapacity for grain is exempt (always 1.0) so weight is unclamped.
	var grainWeight float64
	if err := pool.QueryRow(ctx,
		`SELECT weight FROM settlement_labor WHERE settlement_id = $1 AND good_key = 'grain'`,
		settlementID,
	).Scan(&grainWeight); err != nil {
		t.Fatalf("read seeded grain weight: %v", err)
	}
	wantGrainProd := (grainBase / REF_LABOR) * grainWeight * float64(pop)
	wantDemand := GrainConsumptionPerTick(pop)
	wantGrainRate := wantGrainProd - wantDemand

	_, grainRate := readGood(t, settlementID, "grain")
	if diff := grainRate - wantGrainRate; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("AK2: grain rate = %v, want %v (pre-slice formula: grainProd - demand)", grainRate, wantGrainRate)
	}

	var fishRowExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'fish')`,
		settlementID,
	).Scan(&fishRowExists); err != nil {
		t.Fatalf("check fish row: %v", err)
	}
	if fishRowExists {
		t.Errorf("AK2: a landlocked catchment must never write a fish row at all")
	}
}

// TestRecomputeProduction_AK3_PartialFishCoverage_SumIsExactlyDemand: a
// catchment with water but no plains, sized so fish production is BELOW
// demand: fish nets exactly 0 (fully consumed, no surplus) and grain nets
// exactly -(demand - fishProd) — and the total drawn from both goods equals
// demand exactly, never more or less.
func TestRecomputeProduction_AK3_PartialFishCoverage_SumIsExactlyDemand(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	const tick = 200
	// A single fish tile shared with 5 inert filler tiles: weight auto-seeds
	// to 1/n across whatever potentials this catchment matches (mountain_
	// limestone also unlocks stone/tin unconditionally, plus the universal
	// timber trickle), and fish is capacity-clamped to LaborCapacity's 0.25
	// terrain-base floor regardless of the exact seeded weight — deliberately
	// picked with a large pop so demand outstrips it either way.
	const pop = 20000
	settlementID := recomputeWaterFixture(t, tick, pop, /*grainTiles*/ 0, /*fishTiles*/ 1)

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	_, fishRate := readGood(t, settlementID, "fish")
	if fishRate != 0 {
		t.Fatalf("AK3: fish must be fully consumed (rate exactly 0) when it falls short of demand, got %v", fishRate)
	}

	_, grainRate := readGood(t, settlementID, "grain")
	if grainRate >= 0 {
		t.Fatalf("AK3: grain must carry the residual shortfall as a negative rate, got %v", grainRate)
	}

	// Recover fishProd independently (same catchment/weight/capacity formula)
	// to check the exact identity grainRate == -(demand - fishProd).
	var fishBase float64
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(pr.rate_per_tick), 0) FROM map_tiles mt
		 JOIN production_rules pr ON pr.terrain_type = mt.terrain AND pr.good_key = 'fish' AND pr.building_type IS NULL
		 JOIN settlements s ON s.id = $1
		 JOIN provinces p ON p.id = s.province_id
		 WHERE mt.world_id = p.world_id
		   AND ((mt.q = p.map_q+1 AND mt.r = p.map_r) OR (mt.q = p.map_q-1 AND mt.r = p.map_r) OR
		        (mt.q = p.map_q AND mt.r = p.map_r+1) OR (mt.q = p.map_q AND mt.r = p.map_r-1) OR
		        (mt.q = p.map_q+1 AND mt.r = p.map_r-1) OR (mt.q = p.map_q-1 AND mt.r = p.map_r+1))`,
		settlementID,
	).Scan(&fishBase); err != nil {
		t.Fatalf("read fish base potential: %v", err)
	}
	var weight float64
	if err := pool.QueryRow(ctx,
		`SELECT weight FROM settlement_labor WHERE settlement_id = $1 AND good_key = 'fish'`,
		settlementID,
	).Scan(&weight); err != nil {
		t.Fatalf("read seeded fish weight: %v", err)
	}
	cap := LaborCapacity("fish", true, 0)
	staffed := weight
	if staffed > cap {
		staffed = cap
	}
	fishProd := (fishBase / REF_LABOR) * staffed * float64(pop)
	if fishProd >= GrainConsumptionPerTick(pop) {
		t.Fatalf("test fixture invariant broken: fishProd (%v) must be BELOW demand (%v) for this scenario",
			fishProd, GrainConsumptionPerTick(pop))
	}

	wantGrainRate := -(GrainConsumptionPerTick(pop) - fishProd)
	if diff := grainRate - wantGrainRate; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("AK3: grainRate = %v, want %v (-(demand - fishProd))", grainRate, wantGrainRate)
	}
}
