package kharis

// G2 idempotency regression for TickHandler.applyDecay's growth/grain-draw
// effect (found in the idempotency sweep, sibling of migration 098's already-
// fixed processMaintenance).
//
// Handle fans ONE ScheduledKharisTick event across every player
// (processMaintenance, claimed per event_id+player_id since 098) and THEN
// calls applyDecay(ctx, e.WorldID) unconditionally, unclaimed. applyDecay's
// growth CTE (grain_now/desired_new/actual_new) both GROWS population and
// DRAWS grain from the settlement's stock — a genuine accumulator. A worker
// retry of the SAME event (G2 5s handler timeout, or a crash between commit
// and events.Worker's markDone) re-runs Handle from the top, and an unguarded
// applyDecay would grow and draw a second time for a day that never happened.
//
// This test proves the fix: applyDecay now takes eventID and claims
// (event_id, settlement_id) in processed_tick_claims (same table, same
// pattern as processMaintenance/ColonyPenaltyHandler) before computing
// growth, so a replay of the same event is a no-op.

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/economy"
)

func TestApplyDecay_ReplayIsIdempotent(t *testing.T) {
	// All-plains catchment at a moderate population: a real catchment, not a
	// synthetic one. Warm up several REAL, DISTINCT days first (advanceOneDay: a fresh event
	// ID every call, matching production's one-row-per-due-tick scheduling)
	// so the settlement is actually growing before the replay is exercised.
	terrains := [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}
	pool, worldID, settlementID := newGrowthFixture(t, terrains, 5000)
	h := newTestTickHandler(pool)
	ctx := context.Background()

	const warmupDays = 25
	for i := 0; i < warmupDays; i++ {
		advanceOneDay(t, h, pool, worldID)
	}
	warmPop, warmGrain := snapshot(t, pool, settlementID)
	t.Logf("after %d warm-up game days: pop=%d grain=%.2f", warmupDays, warmPop, warmGrain)
	// This is an explicit player placement in the fixture, not automatic
	// growth behaviour. Keep the replay test on a food-positive growth path.
	for ordinal := 51; ordinal <= warmPop/100; ordinal++ {
		if _, err := economy.PlaceNextGubbeOnBestFoodHex(ctx, pool, settlementID, ordinal); err != nil {
			t.Fatal(err)
		}
	}
	if err := economy.RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatal(err)
	}

	// One more game-day elapses so settled() has fresh accrual to read for the
	// run under test — but from here on both calls share the SAME event ID,
	// since this test simulates a worker RETRY (no further wall/game time
	// passes between the crash and the retry), not two distinct days.
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET current_tick = current_tick + 1 WHERE id = $1`, worldID,
	); err != nil {
		t.Fatalf("advance tick: %v", err)
	}

	startPop, startGrain := snapshot(t, pool, settlementID)
	t.Logf("before: pop=%d grain=%.2f", startPop, startGrain)

	const fixedEventID int64 = 424242

	// First run.
	h.applyDecay(ctx, worldID, fixedEventID)
	firstPop, firstGrain := snapshot(t, pool, settlementID)
	t.Logf("after first run:  pop=%d grain=%.2f", firstPop, firstGrain)

	// Sanity: the fixture must actually exercise POPULATION GROWTH, otherwise
	// this test would pass even without the idempotency guard existing at all.
	if firstPop <= startPop {
		t.Fatalf("first applyDecay run did not grow population (%d -> %d) — fixture does not exercise the growth path this test targets", startPop, firstPop)
	}

	var claimsAfterFirst int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM processed_tick_claims WHERE event_id = $1 AND scope_id = $2`,
		fixedEventID, settlementID,
	).Scan(&claimsAfterFirst); err != nil {
		t.Fatalf("count processed_tick_claims after first run: %v", err)
	}
	if claimsAfterFirst != 1 {
		t.Fatalf("processed_tick_claims rows for (event, settlement) after first run = %d, want 1", claimsAfterFirst)
	}

	// Replay the SAME event — the exact scenario processed_tick_claims
	// (event_id, settlement_id) exists to guard against.
	h.applyDecay(ctx, worldID, fixedEventID)
	replayPop, replayGrain := snapshot(t, pool, settlementID)
	t.Logf("after replay:     pop=%d grain=%.2f", replayPop, replayGrain)

	if replayPop != firstPop {
		t.Errorf("population after replay = %d, want unchanged %d (event %d replayed — a non-idempotent applyDecay would grow it twice)",
			replayPop, firstPop, fixedEventID)
	}
	// Grain must be unchanged on replay: the growth CTE (per-settlement claim)
	// draws no grain and nothing else here touches the stock, so a replay of
	// the same event moves nothing; a non-idempotent growth CTE would
	// additionally grow the population a second time.
	if diff := replayGrain - firstGrain; diff > 0.5 || diff < -0.5 {
		t.Errorf("grain after replay = %.4f, want unchanged %.4f (event %d replayed)",
			replayGrain, firstGrain, fixedEventID)
	}

	var claimsAfterReplay int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM processed_tick_claims WHERE event_id = $1 AND scope_id = $2`,
		fixedEventID, settlementID,
	).Scan(&claimsAfterReplay); err != nil {
		t.Fatalf("count processed_tick_claims after replay: %v", err)
	}
	if claimsAfterReplay != 1 {
		t.Errorf("processed_tick_claims rows for (event, settlement) after replay = %d, want unchanged 1", claimsAfterReplay)
	}
}

// TestApplyDecay_DistinctEventsBothApply is the converse guard: a DIFFERENT
// event ID (the next day's real, distinct ScheduledKharisTick occurrence,
// not a replay of the same one) must NOT be skipped by the claim — the fix
// must not turn applyDecay into a run-once-ever no-op.
func TestApplyDecay_DistinctEventsBothApply(t *testing.T) {
	terrains := [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}
	pool, worldID, settlementID := newGrowthFixture(t, terrains, 5000)
	h := newTestTickHandler(pool)
	ctx := context.Background()

	// This test's subject is the CLAIM, not the economy: it asks whether a
	// distinct event is allowed to do work, and reads population growth as the
	// observable, so the city must be fed on both days — a real stock keeps
	// food_unmet_amount out of the picture.
	if _, err := pool.Exec(ctx,
		`UPDATE settlement_goods SET amount = 10000, calc_tick = (SELECT current_tick FROM worlds WHERE id = $2)
		 WHERE settlement_id = $1 AND good_key = 'grain'`,
		settlementID, worldID,
	); err != nil {
		t.Fatalf("seed grain stock: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET current_tick = current_tick + 1 WHERE id = $1`, worldID,
	); err != nil {
		t.Fatalf("advance tick (game day 1): %v", err)
	}
	h.applyDecay(ctx, worldID, 1001)
	dayOnePop, _ := snapshot(t, pool, settlementID)

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET current_tick = current_tick + 1 WHERE id = $1`, worldID,
	); err != nil {
		t.Fatalf("advance tick (game day 2): %v", err)
	}
	h.applyDecay(ctx, worldID, 1002)
	dayTwoPop, _ := snapshot(t, pool, settlementID)

	if dayTwoPop <= dayOnePop {
		t.Errorf("a distinct event (1002) after event 1001 grew pop %d -> %d, want further growth — the claim must not skip legitimate distinct events",
			dayOnePop, dayTwoPop)
	}
}

// TestApplyDecay_StoresDoNotRot: grain and timber with a positive rate and no
// consumption are, after an applyDecay day, exactly what settled() said before
// it — no 1 % daily shave (abolished 2026-11-02, order ta-bort-lagerrota).
// Cedar is checked alongside as the unchanged control.
func TestApplyDecay_StoresDoNotRot(t *testing.T) {
	terrains := [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}
	pool, worldID, settlementID := newGrowthFixture(t, terrains, 5000)
	h := newTestTickHandler(pool)
	ctx := context.Background()

	settledStock := func(good string) float64 {
		t.Helper()
		var v float64
		if err := pool.QueryRow(ctx,
			`SELECT settled(amount, rate, calc_tick) FROM settlement_goods
			 WHERE settlement_id = $1 AND good_key = $2`, settlementID, good,
		).Scan(&v); err != nil {
			t.Fatalf("read settled %s: %v", good, err)
		}
		return v
	}

	// Timber and cedar share the Storehouse (mig 244); a level-3 one keeps 1 000 well under the ceiling.
	if _, err := pool.Exec(ctx, `INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'storehouse', 3)`, settlementID); err != nil {
		t.Fatalf("storehouse: %v", err)
	}
	// Positive rate, stock well under the cap, calc_tick one day behind so
	// settled() carries real accrual.
	for _, good := range []string{"grain", "timber", "cedar"} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
			 VALUES ($1, $2, 500, 10, 5000, current_world_tick())
			 ON CONFLICT (settlement_id, good_key)
			 DO UPDATE SET amount = 500, rate = 10, cap = 5000, calc_tick = current_world_tick()`,
			settlementID, good,
		); err != nil {
			t.Fatalf("seed %s: %v", good, err)
		}
	}
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET current_tick = current_tick + 1 WHERE id = $1`, worldID,
	); err != nil {
		t.Fatalf("advance tick: %v", err)
	}

	before := map[string]float64{}
	for _, good := range []string{"grain", "timber", "cedar"} {
		before[good] = settledStock(good)
		if before[good] < 509.99 {
			t.Fatalf("%s settled before = %.4f, want 510 — fixture carries no accrual", good, before[good])
		}
	}

	h.applyDecay(ctx, worldID, 616161)

	for _, good := range []string{"grain", "timber", "cedar"} {
		after := settledStock(good)
		if diff := after - before[good]; diff > 0.01 || diff < -0.01 {
			t.Errorf("%s after applyDecay = %.4f, want exactly the settled %.4f — stores must not shrink", good, after, before[good])
		}
	}
}
