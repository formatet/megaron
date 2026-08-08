package kharis

// Röd-först-test för belägring S3 (megaron_plan_belagring.md §S3): the
// starvation clock bookkeeping applySiegeStarvationClock owns. Tested
// directly (not through the whole applyDecay pipeline) so the fixture can
// set settlements.besieged and settlement_goods.rate('grain') by hand
// instead of engineering a real catchment/production scenario —
// RecomputeProduction is not involved here at all.

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/economy"
	"github.com/google/uuid"
)

// siegeClockFixture builds an active world + one active, owned settlement
// with a controllable grain rate — no catchment, no population growth, just
// the two inputs applySiegeStarvationClock reads: besieged and grain's rate.
func siegeClockFixture(t *testing.T, besieged bool, grainRate float64) (worldID, settlementID uuid.UUID) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover test worlds: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 0) RETURNING id`,
		"test-siegeclock-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"siegeclock-"+uuid.New().String(), "siegeclock-"+uuid.New().String()+"@test.invalid",
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
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, besieged)
		 VALUES ($1, $2, 'Starveburg', 'achaean', $3, 'capital', true, 'active', $4) RETURNING id`,
		worldID, provinceID, ownerID, besieged,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'grain', 0, $2, 1000, 0)`,
		settlementID, grainRate,
	); err != nil {
		t.Fatalf("seed grain row: %v", err)
	}
	return worldID, settlementID
}

func readSiegeStarvationTicks(t *testing.T, worldID, settlementID uuid.UUID) int {
	t.Helper()
	pool := testPool(t)
	var ticks int
	if err := pool.QueryRow(context.Background(),
		`SELECT siege_starvation_ticks FROM settlements WHERE id = $1`, settlementID,
	).Scan(&ticks); err != nil {
		t.Fatalf("read siege_starvation_ticks: %v", err)
	}
	return ticks
}

func countScheduledSiegeCapitulations(t *testing.T, worldID, settlementID uuid.UUID) int {
	t.Helper()
	pool := testPool(t)
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM scheduled_events
		 WHERE world_id = $1 AND event_type = 'SiegeCapitulation'
		   AND payload->>'settlement_id' = $2`,
		worldID, settlementID.String(),
	).Scan(&n); err != nil {
		t.Fatalf("count scheduled SiegeCapitulation events: %v", err)
	}
	return n
}

// TestApplySiegeStarvationClock_IncrementsWhileBesiegedAndStarving is the
// core proof: besieged + grain net rate < 0 (food need uncovered even after
// FoodConsumptionSplit's grain→fish→livestock fallback — the invariant that
// makes grainNet negative, recompute.go) accrues the clock by exactly one
// per call, and does NOT enqueue capitulation before the threshold.
func TestApplySiegeStarvationClock_IncrementsWhileBesiegedAndStarving(t *testing.T) {
	worldID, settlementID := siegeClockFixture(t, true, -5)
	h := newTestTickHandler(testPool(t))

	for day := 1; day <= 3; day++ {
		h.applySiegeStarvationClock(context.Background(), worldID)
		got := readSiegeStarvationTicks(t, worldID, settlementID)
		if got != day {
			t.Fatalf("day %d: siege_starvation_ticks = %d, want %d", day, got, day)
		}
	}
	if n := countScheduledSiegeCapitulations(t, worldID, settlementID); n != 0 {
		t.Errorf("scheduled SiegeCapitulation events = %d, want 0 (threshold not reached)", n)
	}
}

// TestApplySiegeStarvationClock_ResetsWhenFoodCovered: the moment grain's
// net rate is no longer negative (food need covered), the clock must reset
// to 0 even though the settlement is still besieged — this is a
// consecutive-days counter, not cumulative.
func TestApplySiegeStarvationClock_ResetsWhenFoodCovered(t *testing.T) {
	worldID, settlementID := siegeClockFixture(t, true, -5)
	h := newTestTickHandler(testPool(t))
	pool := testPool(t)

	h.applySiegeStarvationClock(context.Background(), worldID)
	h.applySiegeStarvationClock(context.Background(), worldID)
	if got := readSiegeStarvationTicks(t, worldID, settlementID); got != 2 {
		t.Fatalf("after 2 starving days: siege_starvation_ticks = %d, want 2", got)
	}

	if _, err := pool.Exec(context.Background(),
		`UPDATE settlement_goods SET rate = 3 WHERE settlement_id = $1 AND good_key = 'grain'`, settlementID,
	); err != nil {
		t.Fatalf("cover food: %v", err)
	}
	h.applySiegeStarvationClock(context.Background(), worldID)
	if got := readSiegeStarvationTicks(t, worldID, settlementID); got != 0 {
		t.Errorf("after food covered: siege_starvation_ticks = %d, want reset to 0", got)
	}
}

// TestApplySiegeStarvationClock_ResetsWhenBlockadeLifted: besieged flipping
// false (the blockade lifted — S1/S2's own daily RecomputeProduction call
// already writes this) resets the clock even if grain's rate is still
// negative for some unrelated reason.
func TestApplySiegeStarvationClock_ResetsWhenBlockadeLifted(t *testing.T) {
	worldID, settlementID := siegeClockFixture(t, true, -5)
	h := newTestTickHandler(testPool(t))
	pool := testPool(t)

	h.applySiegeStarvationClock(context.Background(), worldID)
	h.applySiegeStarvationClock(context.Background(), worldID)
	if got := readSiegeStarvationTicks(t, worldID, settlementID); got != 2 {
		t.Fatalf("after 2 starving days: siege_starvation_ticks = %d, want 2", got)
	}

	if _, err := pool.Exec(context.Background(),
		`UPDATE settlements SET besieged = false WHERE id = $1`, settlementID,
	); err != nil {
		t.Fatalf("lift blockade: %v", err)
	}
	h.applySiegeStarvationClock(context.Background(), worldID)
	if got := readSiegeStarvationTicks(t, worldID, settlementID); got != 0 {
		t.Errorf("after blockade lifted: siege_starvation_ticks = %d, want reset to 0", got)
	}
}

// TestApplySiegeStarvationClock_CapitulatesAtThreshold: crossing
// economy.SiegeCapitulationTicks enqueues exactly one ScheduledSiegeCapitulation
// AND resets the clock in the same call (defensive against a delayed
// handler re-triggering the same episode tomorrow).
func TestApplySiegeStarvationClock_CapitulatesAtThreshold(t *testing.T) {
	worldID, settlementID := siegeClockFixture(t, true, -5)
	pool := testPool(t)
	h := newTestTickHandler(pool)

	if _, err := pool.Exec(context.Background(),
		`UPDATE settlements SET siege_starvation_ticks = $2 WHERE id = $1`,
		settlementID, economy.SiegeCapitulationTicks-1,
	); err != nil {
		t.Fatalf("fast-forward clock: %v", err)
	}

	h.applySiegeStarvationClock(context.Background(), worldID)

	if got := readSiegeStarvationTicks(t, worldID, settlementID); got != 0 {
		t.Errorf("siege_starvation_ticks after crossing threshold = %d, want reset to 0", got)
	}
	if n := countScheduledSiegeCapitulations(t, worldID, settlementID); n != 1 {
		t.Errorf("scheduled SiegeCapitulation events = %d, want exactly 1", n)
	}

	// One more starving day must NOT immediately re-enqueue — the clock was
	// reset, so it starts a fresh 30-day count, not a repeat firing.
	h.applySiegeStarvationClock(context.Background(), worldID)
	if n := countScheduledSiegeCapitulations(t, worldID, settlementID); n != 1 {
		t.Errorf("scheduled SiegeCapitulation events after one more day = %d, want still 1", n)
	}
}

// TestApplySiegeStarvationClock_NeverIncrementsWhenNotBesieged: an
// unbesieged settlement whose grain rate happens to be negative for some
// other reason (an ordinary starving city — a pre-existing, unrelated case)
// must never accrue the siege clock at all.
func TestApplySiegeStarvationClock_NeverIncrementsWhenNotBesieged(t *testing.T) {
	worldID, settlementID := siegeClockFixture(t, false, -5)
	h := newTestTickHandler(testPool(t))

	for day := 1; day <= 3; day++ {
		h.applySiegeStarvationClock(context.Background(), worldID)
	}
	if got := readSiegeStarvationTicks(t, worldID, settlementID); got != 0 {
		t.Errorf("siege_starvation_ticks = %d, want 0 — never besieged", got)
	}
}
