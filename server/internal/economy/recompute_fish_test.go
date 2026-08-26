package economy

// DB integration tests for fisk-föder-befolkningen (2026-07-31): AK1-AK3 from
// the slice contract, proved end-to-end through RecomputeProduction against a
// real catchment (not just the pure FoodConsumptionSplit formula — see
// food_consumption_split_test.go for that half). Gated by DATABASE_URL via
// testPool (sitos_conservation_test.go), same as every other recompute test
// in this package.

import (
	"context"
	"math"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// fishHexOffsets/grainHexOffsets are recomputeWaterFixture's own neighbour
// order (offsets[:fishTiles]/offsets[:grainTiles]) — pulled out so the P4
// placement tests below can address the exact hexes the fixture actually
// seeded, instead of re-deriving them.
var recomputeWaterFixtureOffsets = []hexgrid.Coord{{Q: 1, R: 0}, {Q: -1, R: 0}, {Q: 0, R: 1}, {Q: 0, R: -1}, {Q: 1, R: -1}, {Q: -1, R: 1}}

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

// TestRecomputeProduction_AK1_PureFishCatchment_GrainIsRawNeverNegative: a
// catchment that is ENTIRELY water (no plains at all) gives fish production
// far above the population's food need. Since Utfodringsordningen D1
// (megaron_plan_utfodringsordningen.md, 2026-08-26) RecomputeProduction no
// longer nets EITHER good against consumption — grain must settle at its RAW
// production (the flat nearjord trickle, P1 — never negative, since it has no
// consumption term to go negative FROM any more), and fish must settle at its
// own raw production too, not "production minus demand". The population's
// actual daily meal — grain first, then fish, then livestock — is FoodTick's
// job now (food_tick.go; proved directly in food_tick_test.go), not
// RecomputeProduction's.
//
// pop sized for P3 (megaron_plan_fysisk_gubbemodell.md §8.3): 6 UNENHANCED
// coastal_sea hexes (no harbour built) cap at hexSlots=6 workers regardless of
// population — kustfiske "1 utan byggnad" — so fishProd plateaus at ~31/tick
// no matter how big pop gets.
func TestRecomputeProduction_AK1_PureFishCatchment_GrainIsRawNeverNegative(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	const tick = 200
	const pop = 120
	settlementID := recomputeWaterFixture(t, tick, pop, /*grainTiles*/ 0, /*fishTiles*/ 6)
	// P4: staff every coastal_sea hex to its P3 cap (1, no harbour) — the
	// placement-era equivalent of "weight=1.0, capacity-clamped" pre-P4.
	for i, hex := range recomputeWaterFixtureOffsets {
		placeHexGubbe(t, pool, settlementID, i+1, hex, "fish")
	}

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	grainAmount, grainRate := readGood(t, settlementID, "grain")
	if grainRate != NearjordGrainPerTick {
		t.Errorf("AK1: grain rate must equal the raw nearjord trickle (%v) with zero grain production, got %v", NearjordGrainPerTick, grainRate)
	}
	if grainAmount != 0 {
		t.Errorf("AK1: grain amount should still be 0 (none was ever produced or seeded), got %v", grainAmount)
	}

	fishAmount, fishRate := readGood(t, settlementID, "fish")
	if fishRate <= 0 {
		t.Fatalf("AK1: fish must produce its own raw rate, got rate=%v", fishRate)
	}
	_ = fishAmount

	// Grain's rate can never go negative any more — it has no consumption term
	// left to subtract (D1). Advancing the clock now correctly shows the stock
	// ACCRUING at the nearjord rate (the sawtooth D7 says is the truth, not a
	// regression) — FoodTick, not RecomputeProduction, is what draws it back
	// down once a day (food_tick_test.go).
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
	if wantProjected := NearjordGrainPerTick * 500; projected != wantProjected {
		t.Errorf("AK1: grain stock after 500 elapsed ticks = %v, want %v (raw nearjord accrual, never negative)", projected, wantProjected)
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
	// P4: grain's yield shape is rate × placed, not rate/cap × placed like
	// every other good (placementYield, megaron_plan_grain_cap.md) — so ONE
	// gubbe per plains hex (well within the real per-hex cap of 4, no farm
	// built) already yields that hex's FULL rate_per_tick. pop=500 is an
	// exact multiple of 100 so the population-remainder term (§1.1) is 0 and
	// doesn't need accounting for below.
	for i, hex := range recomputeWaterFixtureOffsets[:3] {
		placeHexGubbe(t, pool, settlementID, i+1, hex, "grain")
	}

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

	// P4: every plains hex is staffed to EXACTLY its cap (2/hex, 3 hexes), so
	// each hex's own yield_per_worker (rate/cap) × cap reproduces that hex's
	// full rate_per_tick — summed across the 3 hexes, that's exactly grainBase
	// (queried above). + NearjordGrainPerTick: P1 (megaron_plan_fysisk_gubbemodell.md
	// §3.2) adds the settlement's own-hex flat grain trickle on top of the
	// catchment-ring potential — unconditional, not scaled by placement.
	//
	// Utfodringsordningen D1 (megaron_plan_utfodringsordningen.md, 2026-08-26):
	// grain's rate is RAW production now — no demand subtracted. pop is still
	// what this fixture needs to size the population-remainder term (§1.1),
	// but the population's food no longer shows up in this rate at all.
	wantGrainRate := grainBase + NearjordGrainPerTick

	_, grainRate := readGood(t, settlementID, "grain")
	// Relative, not absolute, tolerance: migration 109 (2026-08-06, tick-is-the-
	// day) scaled production_rules ×24, so these figures now run ~24× larger
	// than when the 1e-6 absolute epsilon here was calibrated, and the DB-side
	// float8 aggregation (SUM/multiply across several rows) accumulates rounding
	// proportional to magnitude, not a fixed absolute amount. A fixed absolute
	// epsilon that held at the old scale spuriously fails at the new one even
	// though the relative precision is unchanged.
	if diff := math.Abs(grainRate - wantGrainRate); diff > 1e-6*math.Abs(wantGrainRate) {
		t.Errorf("AK2: grain rate = %v, want %v (raw production, D1 — no demand term)", grainRate, wantGrainRate)
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

// TestRecomputeProduction_AK3_FishAndGrainRatesAreIndependentOfDemand: a
// catchment with water but no plains, sized so fish production is far BELOW
// the population's food need. Before Utfodringsordningen D1
// (megaron_plan_utfodringsordningen.md, 2026-08-26) RecomputeProduction ran
// the grain→fish fallback against these PRODUCTION rates and clamped fish to
// 0 / drained grain negative for the residual — that fallback now runs once a
// day, against STOCK, in FoodTick (food_tick_test.go). RecomputeProduction
// itself must write each good's RAW rate, with no reference to demand at all:
// fish keeps its own full production regardless of how far short it falls,
// and grain keeps its flat nearjord trickle (P1) regardless of population size.
func TestRecomputeProduction_AK3_FishAndGrainRatesAreIndependentOfDemand(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	const tick = 200
	// A single fish tile shared with 5 inert filler tiles: weight auto-seeds
	// to 1/n across whatever potentials this catchment matches (mountain_
	// limestone also unlocks stone/tin unconditionally, plus the universal
	// timber trickle), and fish is capacity-clamped to LaborCapacity's 0.25
	// terrain-base floor regardless of the exact seeded weight — deliberately
	// picked with a large pop so demand outstrips it either way (proving the
	// rate does NOT react to that at all any more).
	const pop = 20000
	settlementID := recomputeWaterFixture(t, tick, pop, /*grainTiles*/ 0, /*fishTiles*/ 1)
	// P4: staff the one coastal_sea hex to its P3 cap (1, no harbour) — fully
	// staffed regardless of pop is the whole point (P3's population-invariant
	// cap), so a huge pop is still fed by exactly this one hex's yield.
	placeHexGubbe(t, pool, settlementID, 1, recomputeWaterFixtureOffsets[0], "fish")

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	// Recover fishProd independently (same catchment/weight/capacity formula)
	// to check the exact identity fishRate == fishProd (no demand term at all).
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
	// P4: the one fish hex is staffed to EXACTLY its cap (1), so its
	// yield_per_worker (rate/cap) × cap reproduces the hex's full
	// rate_per_tick — i.e. fishProd == fishBase, regardless of pop.
	fishProd := fishBase
	if fishProd >= GrainConsumptionPerTick(pop) {
		t.Fatalf("test fixture invariant broken: fishProd (%v) must be BELOW demand (%v) for this scenario",
			fishProd, GrainConsumptionPerTick(pop))
	}

	_, fishRate := readGood(t, settlementID, "fish")
	if fishRate != fishProd {
		t.Errorf("AK3: fish rate = %v, want %v (raw production — falling short of demand no longer clamps it to 0)", fishRate, fishProd)
	}

	_, grainRate := readGood(t, settlementID, "grain")
	if grainRate != NearjordGrainPerTick {
		t.Errorf("AK3: grain rate = %v, want %v (flat nearjord trickle — a huge population's demand leaves no mark on it)", grainRate, NearjordGrainPerTick)
	}
}
