package kharis

// Manskaps-underhåll (megaron_plan_rekryteringsmodell.md, Timothy
// 2026-08-19) — DB integration tests for the reinforce trickle
// (applyReinforcement, called from applyDecay). Reuses the grain-growth
// fixture (grain_growth_test.go: newGrowthFixture/advanceOneDay/snapshot) —
// the trickle is deliberately hooked into that same daily growth step, so
// exercising it needs the same world/settlement/catchment machinery.
//
// Core invariants under test (plan §Framgångskriterier):
//   - a decimated garrisoned cohort in its OWN origin city refills a few men
//     per day, capped by min(economy.ReinforceMenPerTick, growth-this-tick,
//     100-size);
//   - the origin city's population NEVER shrinks from a refill (it can only
//     slow that day's growth, never reverse it);
//   - the refill costs resources pro-rata, throttled by what the city can
//     actually afford;
//   - a cohort that leaves its origin garrison stops refilling and keeps
//     whatever size it had reached;
//   - the tick handler is safe to run twice for the same event (G2).

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// seedReinforcingUnit inserts a garrisoned land cohort at settlementID, its
// own origin, flagged reinforcing — exactly the state POST .../reinforce
// leaves a unit in (api/handlers/unit.go Reinforce never touches size itself,
// only the flag; kharis/tick.go applyReinforcement does the actual growing).
func seedReinforcingUnit(t *testing.T, pool *pgxpool.Pool, worldID, settlementID uuid.UUID, unitType string, size int) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT owner_id FROM settlements WHERE id = $1`, settlementID,
	).Scan(&ownerID); err != nil {
		t.Fatalf("load settlement owner: %v", err)
	}

	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
		                    settlement_id, support_settlement_id, origin_settlement_id, reinforcing)
		 VALUES ($1, $2, $3, 'land', $4, 0, 'garrison', $5, $5, $5, true)
		 RETURNING id`,
		worldID, ownerID, unitType, size, settlementID,
	).Scan(&unitID); err != nil {
		t.Fatalf("seed reinforcing unit: %v", err)
	}
	return unitID
}

// loadUnitState reads the fields applyReinforcement mutates.
func loadUnitState(t *testing.T, pool *pgxpool.Pool, unitID uuid.UUID) (size int, reinforcing bool) {
	t.Helper()
	ctx := context.Background()
	if err := pool.QueryRow(ctx,
		`SELECT size, reinforcing FROM units WHERE id = $1`, unitID,
	).Scan(&size, &reinforcing); err != nil {
		t.Fatalf("load unit state: %v", err)
	}
	return size, reinforcing
}

// setGood sets a settlement_goods row's raw amount directly (seeding a
// resource stock outside the production loop — bronze/silver aren't produced
// by this fixture's plains/mountain catchment, so RecomputeProduction's
// cap-clamp upsert never touches them; verified empirically by these tests
// passing with the exact deltas asserted below).
func setGood(t *testing.T, pool *pgxpool.Pool, settlementID uuid.UUID, good string, amount float64) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, $2, $3, 0, 100000, current_world_tick())
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET amount = $3, rate = 0, calc_tick = current_world_tick()`,
		settlementID, good, amount,
	); err != nil {
		t.Fatalf("seed good %s: %v", good, err)
	}
}

func goodAmount(t *testing.T, pool *pgxpool.Pool, settlementID uuid.UUID, good string) float64 {
	t.Helper()
	ctx := context.Background()
	var amt float64
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE((SELECT settled(amount, rate, calc_tick) FROM settlement_goods
		                   WHERE settlement_id=$1 AND good_key=$2), 0)`,
		settlementID, good,
	).Scan(&amt); err != nil {
		t.Fatalf("read good %s: %v", good, err)
	}
	return amt
}

// richTerrain gives a comfortably positive daily growth (well above
// economy.ReinforceMenPerTick), so the trickle is capped by the per-tick
// constant rather than by the city's own growth budget in the "happy path"
// tests — the resource/garrison-exit tests below deliberately test the OTHER
// caps instead.
var richTerrain = [6]string{"plains", "plains", "plains", "mountain_limestone", "mountain_limestone", "mountain_limestone"}

// TestApplyReinforcement_TrickleCappedAndPopulationNeverShrinks is the core
// invariant gate: a 62/100 cohort refills exactly
// economy.ReinforceMenPerTick men per day (spearman costs no bronze, and
// grain/silver are seeded generously so no resource throttle binds here —
// only the per-tick cap does), and the origin settlement's population is
// NEVER lower after a refill day than before it — the refill can only
// cannibalize that day's growth, never the standing population.
func TestApplyReinforcement_TrickleCappedAndPopulationNeverShrinks(t *testing.T) {
	pool, worldID, settlementID := newGrowthFixture(t, richTerrain, 5000)
	setGood(t, pool, settlementID, "grain", 50000)
	setGood(t, pool, settlementID, "silver", 500)
	h := newTestTickHandler(pool)

	unitID := seedReinforcingUnit(t, pool, worldID, settlementID, "spearman", 62)

	wantSize := 62
	for day := 1; day <= 3; day++ {
		// Re-seed grain before every day: RecomputeProduction clamps the stock
		// back down to its (small, fixture-seeded) cap at the end of each
		// applyDecay call, so a one-time seed at the top would only keep
		// growth unthrottled on day 1 — the point of this test is to isolate
		// the flat ReinforceMenPerTick cap, not grain affordability.
		setGood(t, pool, settlementID, "grain", 50000)
		popBefore, _ := snapshot(t, pool, settlementID)
		advanceOneDay(t, h, pool, worldID)
		popAfter, _ := snapshot(t, pool, settlementID)

		if popAfter < popBefore {
			t.Fatalf("day %d: population shrank %d -> %d — a refill must never reduce standing population",
				day, popBefore, popAfter)
		}

		wantSize += 4 // economy.ReinforceMenPerTick
		size, reinforcing := loadUnitState(t, pool, unitID)
		if size != wantSize {
			t.Errorf("day %d: size = %d, want %d (ReinforceMenPerTick=4/day, uncapped by growth or resources here)",
				day, size, wantSize)
		}
		if !reinforcing {
			t.Errorf("day %d: reinforcing = false, want true (still under 100)", day)
		}
	}
}

// TestApplyReinforcement_StopsAndClearsFlagAtFullStrength verifies a cohort
// close to full only takes exactly the room it has left (min(...,
// 100-size)), and reinforcing flips to false the moment it reaches 100 — no
// further trickle, and no over-fill past the cohort atom (economy.MaxUnitSize).
func TestApplyReinforcement_StopsAndClearsFlagAtFullStrength(t *testing.T) {
	pool, worldID, settlementID := newGrowthFixture(t, richTerrain, 5000)
	setGood(t, pool, settlementID, "grain", 50000)
	setGood(t, pool, settlementID, "silver", 500)
	h := newTestTickHandler(pool)

	unitID := seedReinforcingUnit(t, pool, worldID, settlementID, "spearman", 97)
	advanceOneDay(t, h, pool, worldID)

	size, reinforcing := loadUnitState(t, pool, unitID)
	if size != 100 {
		t.Errorf("size = %d, want 100 (97 + min(4, growth, 100-97=3) = 100)", size)
	}
	if reinforcing {
		t.Error("reinforcing = true after reaching 100, want false (cleared at full strength)")
	}

	// A second day must not push past 100 or resurrect the flag.
	setGood(t, pool, settlementID, "grain", 50000)
	advanceOneDay(t, h, pool, worldID)
	size2, reinforcing2 := loadUnitState(t, pool, unitID)
	if size2 != 100 {
		t.Errorf("size after a further day = %d, want still 100 (no over-fill)", size2)
	}
	if reinforcing2 {
		t.Error("reinforcing = true after a further day, want still false")
	}
}

// TestApplyReinforcement_ResourceShortfallThrottlesRefill verifies pro-rata
// resource cost: an elite_infantry cohort (per-man cost includes bronze,
// province.UnitSpecs) refills only as far as its bronze stock allows when
// that is the binding constraint, not the flat ReinforceMenPerTick cap —
// exactly the "elite-refill drar brons; otillräckligt brons -> stannar"
// criterion. grain/silver are seeded generously so bronze alone binds.
func TestApplyReinforcement_ResourceShortfallThrottlesRefill(t *testing.T) {
	pool, worldID, settlementID := newGrowthFixture(t, richTerrain, 5000)
	setGood(t, pool, settlementID, "grain", 50000)
	setGood(t, pool, settlementID, "silver", 500)
	setGood(t, pool, settlementID, "bronze", 0.5) // affords floor(0.5/0.2) = 2 men, not the full 4

	h := newTestTickHandler(pool)
	unitID := seedReinforcingUnit(t, pool, worldID, settlementID, "elite_infantry", 50)

	bronzeBefore := goodAmount(t, pool, settlementID, "bronze")
	advanceOneDay(t, h, pool, worldID)
	bronzeAfter := goodAmount(t, pool, settlementID, "bronze")

	size, reinforcing := loadUnitState(t, pool, unitID)
	if size != 52 {
		t.Errorf("size = %d, want 52 (50 + 2 — bronze at 0.5 only affords 2 men at 0.2/man, below the 4/tick cap)", size)
	}
	if !reinforcing {
		t.Error("reinforcing = false, want true (still under 100, just resource-throttled this tick)")
	}
	wantBronzeDrawn := 2 * 0.2
	if diff := (bronzeBefore - bronzeAfter) - wantBronzeDrawn; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("bronze drawn = %.4f, want %.4f (2 men x 0.2/man)", bronzeBefore-bronzeAfter, wantBronzeDrawn)
	}

	// Bronze now effectively exhausted (0.1 left, buys 0 more men at 0.2/man)
	// — a further day must not throttle-fill a fractional man or go negative.
	// Re-seed grain/silver only (NOT bronze) so bronze stays the sole binding
	// constraint on this second day too.
	setGood(t, pool, settlementID, "grain", 50000)
	setGood(t, pool, settlementID, "silver", 500)
	advanceOneDay(t, h, pool, worldID)
	size2, _ := loadUnitState(t, pool, unitID)
	if size2 != 52 {
		t.Errorf("size after bronze exhausted = %d, want still 52 (0.1 bronze buys 0 whole men)", size2)
	}
	if b := goodAmount(t, pool, settlementID, "bronze"); b < 0 {
		t.Errorf("bronze went negative: %.4f", b)
	}
}

// TestApplyReinforcement_LeavingOriginGarrisonStopsRefillAndHoldsSize proves
// the "lämna garnisonen mitt i -> refill stannar, size bevaras" criterion: a
// cohort ordered to leave its origin city (simulated the same way March
// leaves it — settlement_id cleared, status no longer garrison) stops
// trickling and keeps exactly the size it had reached; the reinforcing flag
// clears (so a later return to garrison, elsewhere or here, doesn't silently
// resume without a fresh POST .../reinforce).
func TestApplyReinforcement_LeavingOriginGarrisonStopsRefillAndHoldsSize(t *testing.T) {
	pool, worldID, settlementID := newGrowthFixture(t, richTerrain, 5000)
	setGood(t, pool, settlementID, "grain", 50000)
	setGood(t, pool, settlementID, "silver", 500)
	h := newTestTickHandler(pool)

	unitID := seedReinforcingUnit(t, pool, worldID, settlementID, "spearman", 62)
	advanceOneDay(t, h, pool, worldID)
	sizeAfterDay1, reinforcingAfterDay1 := loadUnitState(t, pool, unitID)
	if sizeAfterDay1 != 66 || !reinforcingAfterDay1 {
		t.Fatalf("precondition: after day 1 want (66, true), got (%d, %v)", sizeAfterDay1, reinforcingAfterDay1)
	}

	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`UPDATE units SET status = 'marching', settlement_id = NULL WHERE id = $1`, unitID,
	); err != nil {
		t.Fatalf("simulate march away from origin: %v", err)
	}

	advanceOneDay(t, h, pool, worldID)
	sizeAfterDay2, reinforcingAfterDay2 := loadUnitState(t, pool, unitID)
	if sizeAfterDay2 != 66 {
		t.Errorf("size after leaving origin = %d, want still 66 (refill stopped, size held)", sizeAfterDay2)
	}
	if reinforcingAfterDay2 {
		t.Error("reinforcing = true after leaving origin, want false (cleared by the unconditional cleanup)")
	}
}

// TestApplyReinforcement_IdempotentUnderEventReplay directly proves the G2
// claim (processed_tick_claims keyed by (event_id, unit_id)): calling
// applyReinforcement twice with the SAME eventID and the SAME crossings
// (bypassing applyDecay's own outer per-settlement growth claim, which would
// already suppress a naive re-run — this isolates and proves the INNER
// per-unit claim specifically) must only apply the refill once.
func TestApplyReinforcement_IdempotentUnderEventReplay(t *testing.T) {
	pool, worldID, settlementID := newGrowthFixture(t, richTerrain, 5000)
	setGood(t, pool, settlementID, "grain", 50000)
	setGood(t, pool, settlementID, "silver", 500)
	h := newTestTickHandler(pool)

	unitID := seedReinforcingUnit(t, pool, worldID, settlementID, "spearman", 62)

	const fixedEventID int64 = 424242
	crossings := []popCrossing{{id: settlementID, oldPop: 5000, newPop: 5050}} // budget=50, well above the 4/tick cap

	ctx := context.Background()
	h.applyReinforcement(ctx, worldID, fixedEventID, crossings)
	sizeAfterFirst, _ := loadUnitState(t, pool, unitID)
	if sizeAfterFirst != 66 {
		t.Fatalf("size after first applyReinforcement = %d, want 66 (62+4)", sizeAfterFirst)
	}

	// Replay: identical eventID, identical crossings — must be a no-op.
	h.applyReinforcement(ctx, worldID, fixedEventID, crossings)
	sizeAfterReplay, _ := loadUnitState(t, pool, unitID)
	if sizeAfterReplay != 66 {
		t.Errorf("size after replaying the SAME event = %d, want still 66 (claim must block a second refill)", sizeAfterReplay)
	}
}
