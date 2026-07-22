package kharis

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
)

// testPool connects to a real Postgres instance — grain-funded growth is pure
// SQL orchestration (settled(), current_world_tick(), the CTE-chained population
// UPDATE) that a mock can't meaningfully stand in for. Skips (not fails) when
// DATABASE_URL isn't set, so `go test ./...` stays green without a database.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// growthFixtureCatchment describes the six catchment tiles to seed around the
// settlement's origin province, expressed as terrain names in the standard
// axial neighbour order used throughout the codebase:
// (+1,0) (-1,0) (0,+1) (0,-1) (+1,-1) (-1,+1).
var catchmentOffsets = [6][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}

// newGrowthFixture builds an active world + one capital settlement whose
// catchment is seeded from terrains, with the standard genesis starter
// buildings (farm/lumbermill/temple/market) and genesis labor weights
// (grain=0.85, cult=0.15 — mirrors api/handlers/join.go), at the given start
// population. Returns pool, worldID, settlementID.
func newGrowthFixture(t *testing.T, terrains [6]string, pop int) (*pgxpool.Pool, uuid.UUID, uuid.UUID) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-graingrowth-%'`,
	); err != nil {
		t.Fatalf("archive leftover test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 0) RETURNING id`,
		"test-graingrowth-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"graingrowth-"+uuid.New().String(), "graingrowth-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}

	for i, off := range catchmentOffsets {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, off[0], off[1], terrains[i],
		); err != nil {
			t.Fatalf("seed catchment tile %d: %v", i, err)
		}
	}

	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Growthville', 'achaean', $3, 'capital', true, $4) RETURNING id`,
		worldID, provinceID, ownerID, pop,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	// Genesis starter buildings (mirrors api/handlers/starter_buildings.go).
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level)
		 SELECT $1, bt, 1 FROM unnest(ARRAY['farm','lumbermill','temple','market']) AS bt`,
		settlementID,
	); err != nil {
		t.Fatalf("seed starter buildings: %v", err)
	}

	// Genesis labor weights (mirrors api/handlers/join.go): grain dominates so
	// the starter city is self-sufficient; cult gets a floor.
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_labor (settlement_id, good_key, weight) VALUES ($1, 'grain', 0.85), ($1, 'cult', 0.15)`,
		settlementID,
	); err != nil {
		t.Fatalf("seed labor weights: %v", err)
	}

	// Seed a zero grain row at tick 0, then let RecomputeProduction derive the
	// real rate from the catchment — exactly the join.go genesis sequence.
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'grain', 0, 0, 1000, 0)`,
		settlementID,
	); err != nil {
		t.Fatalf("seed grain row: %v", err)
	}
	if err := economy.RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("initial RecomputeProduction: %v", err)
	}

	return pool, worldID, settlementID
}

// snapshot reads (population, grain_amount) for a settlement.
func snapshot(t *testing.T, pool *pgxpool.Pool, settlementID uuid.UUID) (pop int, grain float64) {
	t.Helper()
	ctx := context.Background()
	if err := pool.QueryRow(ctx,
		`SELECT s.population,
		        COALESCE((SELECT settled(sg.amount, sg.rate, sg.calc_tick)
		                  FROM settlement_goods sg
		                  WHERE sg.settlement_id = s.id AND sg.good_key = 'grain'), 0)
		 FROM settlements s WHERE s.id = $1`,
		settlementID,
	).Scan(&pop, &grain); err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	return pop, grain
}

// advanceOneDay simulates one game-day passing: bumps worlds.current_tick by
// TicksPerDay (so settled() sees the elapsed production/decay), then runs the
// same decay step the real KharisTick handler runs.
func advanceOneDay(t *testing.T, h *TickHandler, pool *pgxpool.Pool, worldID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET current_tick = current_tick + $1 WHERE id = $2`,
		events.TicksPerDay, worldID,
	); err != nil {
		t.Fatalf("advance tick: %v", err)
	}
	h.applyDecay(ctx, worldID)
}

func newTestTickHandler(pool *pgxpool.Pool) *TickHandler {
	sched := events.NewScheduler(pool, clock.NewTestClock(time.Now()))
	store := events.NewStore(pool)
	return NewTickHandler(pool, sched, store, nil)
}

// TestApplyDecay_GrainFundedGrowth_MinimalCitySelfSufficient is the hard
// invariant gate (success criterion #1): a start city with the minimal
// guaranteed genesis catchment (exactly one plains tile — the self-sufficiency
// invariant in api/handlers/join.go requires at least one plains/river_valley
// catchment tile, no more), default labor weights, and start population 5000,
// left completely neglected (no Wanax action, no additional buildings), must
// never starve: grain must stay > 0 and population must never shrink below its
// start value over many days. If grainPerCitizen is calibrated too high, this
// test fails and the constant must be lowered.
func TestApplyDecay_GrainFundedGrowth_MinimalCitySelfSufficient(t *testing.T) {
	terrains := [6]string{"plains", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone"}
	pool, worldID, settlementID := newGrowthFixture(t, terrains, 5000)
	h := newTestTickHandler(pool)

	startPop, startGrain := snapshot(t, pool, settlementID)
	t.Logf("day 0: pop=%d grain=%.2f", startPop, startGrain)

	const days = 40
	prevPop := startPop
	minGrain := 1e9
	for day := 1; day <= days; day++ {
		advanceOneDay(t, h, pool, worldID)
		pop, grain := snapshot(t, pool, settlementID)
		t.Logf("day %d: pop=%d grain=%.2f", day, pop, grain)

		if grain <= 0 {
			t.Fatalf("day %d: grain hit %.4f — self-sufficiency invariant violated (starving minimal start city)", day, grain)
		}
		if grain < minGrain {
			minGrain = grain
		}
		if pop < prevPop {
			t.Fatalf("day %d: population shrank %d -> %d — self-sufficiency invariant violated", day, prevPop, pop)
		}
		prevPop = pop
	}
	t.Logf("minimum grain observed over %d days: %.4f", days, minGrain)

	if prevPop <= startPop {
		t.Errorf("expected some net growth over %d days for a grain-positive city, pop stayed at %d", days, prevPop)
	}
}

// rawGrainRow reads the *stored* (unsettled) grain amount and rate — the raw
// columns, not settled() — so the Go mirror below can reproduce the exact
// server-side calculation from the same inputs the SQL sees.
func rawGrainRow(t *testing.T, pool *pgxpool.Pool, settlementID uuid.UUID) (amount, rate float64) {
	t.Helper()
	ctx := context.Background()
	if err := pool.QueryRow(ctx,
		`SELECT amount, rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'`,
		settlementID,
	).Scan(&amount, &rate); err != nil {
		t.Fatalf("read raw grain row: %v", err)
	}
	return amount, rate
}

// expectedDayResult mirrors tick.go's applyDecay formula exactly (decay ×0.99,
// then desired-growth pricing against grainPerCitizen, throttled by
// affordability) so the test can cross-check the SQL's actual output against
// an independent Go computation — a rigorous proof of the atomicity/
// consistency guarantee (pop-added always equals grain-drawn/grainPerCitizen)
// rather than a heuristic bound.
func expectedDayResult(prevPop int, prevAmount, prevRate float64) (newPop int, newGrain float64) {
	grainNow := (prevAmount + prevRate*float64(events.TicksPerDay)) * 0.99
	if grainNow < 0 {
		grainNow = 0
	}
	softcap := 1.0 - float64(prevPop)/30000.0
	if softcap < 0 {
		softcap = 0
	}
	const variety = 1.0 // no fish/oil/wine/livestock in these fixtures
	desired := float64(prevPop) * 0.005 * variety * softcap
	desiredNew := int(desired + 0.5) // ROUND
	if desiredNew < 1 {
		desiredNew = 1
	}

	if grainNow <= 0 {
		// starvation path — not exercised by these fixtures (grain stays positive).
		p := int(float64(prevPop)*0.995 + 0.5)
		if p < 101 {
			p = 101
		}
		return p, prevAmount // grain untouched on starvation path
	}

	var actualNew int
	var draw float64
	cost := float64(desiredNew) * grainPerCitizen
	if grainNow >= cost {
		actualNew = desiredNew
		draw = cost
	} else {
		actualNew = int(grainNow / grainPerCitizen)
		draw = float64(actualNew) * grainPerCitizen
	}

	newPop = prevPop + actualNew
	if newPop < 101 {
		newPop = 101
	}
	if newPop > 30000 {
		newPop = 30000
	}
	newGrain = grainNow - draw
	if newGrain < 0 {
		newGrain = 0
	}
	return newPop, newGrain
}

// TestApplyDecay_GrainFundedGrowth_CapUnpinnedAndConsistent verifies success
// criterion #2 (cap un-pinned: a grain-poor-but-viable city's stock sits
// below cap during growth, not glued at 1000) AND the grain-draw consistency
// guarantee, by mirroring tick.go's exact formula in Go
// (expectedDayResult) and cross-checking it against the real SQL's output
// day by day — not just a heuristic bound.
func TestApplyDecay_GrainFundedGrowth_CapUnpinnedAndConsistent(t *testing.T) {
	// The minimal guaranteed catchment (one plains tile) is the realistic case
	// where grainPerCitizen (300) is calibrated to bind: growth is throttled
	// below the desired amount, so the remainder visibly sits under the 1000
	// cap every day instead of re-saturating.
	terrains := [6]string{"plains", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone"}
	pool, worldID, settlementID := newGrowthFixture(t, terrains, 5000)
	h := newTestTickHandler(pool)

	sawBelowCap := false
	const days = 15
	for day := 1; day <= days; day++ {
		prevPop, _ := snapshot(t, pool, settlementID)
		prevAmount, prevRate := rawGrainRow(t, pool, settlementID)
		wantPop, wantGrain := expectedDayResult(prevPop, prevAmount, prevRate)

		advanceOneDay(t, h, pool, worldID)

		gotPop, gotGrain := snapshot(t, pool, settlementID)
		t.Logf("day %d: pop=%d grain=%.2f (want pop=%d grain=%.2f)", day, gotPop, gotGrain, wantPop, wantGrain)

		if gotPop != wantPop {
			t.Errorf("day %d: population = %d, want %d (Go mirror of tick.go formula)", day, gotPop, wantPop)
		}
		// RecomputeProduction runs after the growth write and re-derives the
		// rate (and re-settles/clamps amount) from the new population — its
		// settle step is a no-op here (elapsed=0) but it does re-derive rate,
		// so allow float slop from that re-derivation, not from the growth math.
		if diff := gotGrain - wantGrain; diff > 5.0 || diff < -5.0 {
			t.Errorf("day %d: grain = %.4f, want %.4f (Go mirror of tick.go formula)", day, gotGrain, wantGrain)
		}
		if gotGrain < 999.0 {
			sawBelowCap = true
		}
		if gotGrain <= 0 {
			t.Fatalf("day %d: grain hit %.4f — self-sufficiency invariant violated", day, gotGrain)
		}
	}

	if !sawBelowCap {
		t.Errorf("grain never dipped below cap (1000) over %d days — growth is not consuming surplus (cap-pinning bug not fixed)", days)
	}
}

// TestApplyDecay_GrainFundedGrowth_GeographyDifferentiates verifies success
// criterion #3: a grain-rich catchment grows faster than a grain-poor one.
func TestApplyDecay_GrainFundedGrowth_GeographyDifferentiates(t *testing.T) {
	// current_world_tick() has no per-world scope — it reads whatever world has
	// status='active' (single-world-enforcement assumption, see mig 067/063).
	// Two fixtures can't run concurrently active; run poor to completion first,
	// then create+run rich (newGrowthFixture archives the previous test world).
	poor := [6]string{"plains", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone"}
	rich := [6]string{"plains", "plains", "plains", "mountain_limestone", "mountain_limestone", "mountain_limestone"}

	const days = 15

	poolPoor, worldPoor, settlementPoor := newGrowthFixture(t, poor, 5000)
	hPoor := newTestTickHandler(poolPoor)
	for day := 1; day <= days; day++ {
		advanceOneDay(t, hPoor, poolPoor, worldPoor)
	}
	popPoor, grainPoor := snapshot(t, poolPoor, settlementPoor)

	poolRich, worldRich, settlementRich := newGrowthFixture(t, rich, 5000)
	hRich := newTestTickHandler(poolRich)
	for day := 1; day <= days; day++ {
		advanceOneDay(t, hRich, poolRich, worldRich)
	}
	popRich, grainRich := snapshot(t, poolRich, settlementRich)

	t.Logf("after %d days — poor: pop=%d grain=%.2f | rich: pop=%d grain=%.2f", days, popPoor, grainPoor, popRich, grainRich)

	if popRich <= popPoor {
		t.Errorf("expected richer catchment to grow faster: rich pop=%d, poor pop=%d", popRich, popPoor)
	}
}

// TestApplyDecay_GrainFundedGrowth_NoOscillation verifies success criterion
// #4: grain/pop evolve smoothly — grain does not sawtooth to (near) zero and
// back up to cap every day, which would indicate grainPerCitizen consumes
// ~100% of surplus instead of a damped fraction.
func TestApplyDecay_GrainFundedGrowth_NoOscillation(t *testing.T) {
	terrains := [6]string{"plains", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone"}
	pool, worldID, settlementID := newGrowthFixture(t, terrains, 5000)
	h := newTestTickHandler(pool)

	const days = 20
	var grains []float64
	for day := 1; day <= days; day++ {
		advanceOneDay(t, h, pool, worldID)
		_, grain := snapshot(t, pool, settlementID)
		grains = append(grains, grain)
	}
	t.Logf("grain trajectory: %v", grains)

	// Look at the tail (after initial fill/settle transient) for near-zero
	// troughs immediately followed by a near-cap peak — the oscillation
	// signature. Skip the first third of the run (transient).
	start := days / 3
	for i := start; i < len(grains)-1; i++ {
		if grains[i] < 5.0 && grains[i+1] > 900.0 {
			t.Errorf("oscillation detected: day %d grain=%.2f -> day %d grain=%.2f (near-zero then slammed back to cap)",
				i+1, grains[i], i+2, grains[i+1])
		}
	}
}
