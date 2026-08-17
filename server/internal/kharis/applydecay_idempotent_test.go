package kharis

// G2 idempotency regression for TickHandler.applyDecay's growth/grain-draw
// effect (found in the idempotency sweep, sibling of migration 098's already-
// fixed processMaintenance and loyalty.ColonyPenaltyHandler).
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
)

func TestApplyDecay_ReplayIsIdempotent(t *testing.T) {
	// All-plains catchment at a moderate population: a real catchment, not a
	// synthetic one. A single tick's grain accrual is nowhere near
	// grainPerCitizen×desired_new, so growth stays throttled to 0 for a while
	// (exactly like the minimal-catchment fixture in grain_growth_test.go) —
	// warm up several REAL, DISTINCT days first (advanceOneDay: a fresh event
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
	t.Logf("after %d warm-up days: pop=%d grain=%.2f", warmupDays, warmPop, warmGrain)

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

	// Sanity: the fixture must actually exercise POPULATION GROWTH (not just
	// the unrelated ×0.99 grain/timber decay, which moves grain every single
	// call and would make this assertion trivially true even without the
	// warm-up), otherwise this test would pass even without the idempotency
	// guard existing at all.
	if firstPop <= startPop {
		t.Fatalf("first applyDecay run did not grow population (%d -> %d) — fixture does not exercise the growth/grain-draw path this test targets", startPop, firstPop)
	}
	if firstGrain == startGrain {
		t.Fatalf("first applyDecay run did not move grain (%.4f -> %.4f) — expected a grain draw alongside growth", startGrain, firstGrain)
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
	// Grain is NOT expected to be bit-for-bit unchanged: the flat ×0.99
	// grain/timber decay a few lines above the claimed growth CTE is its own,
	// separate, pre-existing gap (undocumented before this fix, still
	// unclaimed after it — see the eventID doc comment on applyDecay) and
	// runs every call regardless of replay, so a SINGLE extra ×0.99 pass is
	// expected here. What must NOT happen is a SECOND grain-draw from growth
	// re-firing (~desired_new × grainPerCitizen, hundreds/thousands of
	// grain) stacked on top of that decay. Assert the observed replay grain
	// matches "one more ×0.99 decay pass and nothing else" to a tight
	// tolerance — a re-fired growth draw would blow far past it.
	wantReplayGrain := firstGrain * 0.99
	if diff := replayGrain - wantReplayGrain; diff > 0.5 || diff < -0.5 {
		t.Errorf("grain after replay = %.4f, want %.4f (= firstGrain × 0.99, the known unclaimed decay pass ONLY — event %d replayed, a non-idempotent growth CTE would additionally draw ~desired_new×%.0f grain on top)",
			replayGrain, wantReplayGrain, fixedEventID, grainPerCitizen)
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

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET current_tick = current_tick + 1 WHERE id = $1`, worldID,
	); err != nil {
		t.Fatalf("advance tick (day 1): %v", err)
	}
	h.applyDecay(ctx, worldID, 1001)
	dayOnePop, _ := snapshot(t, pool, settlementID)

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET current_tick = current_tick + 1 WHERE id = $1`, worldID,
	); err != nil {
		t.Fatalf("advance tick (day 2): %v", err)
	}
	h.applyDecay(ctx, worldID, 1002)
	dayTwoPop, _ := snapshot(t, pool, settlementID)

	if dayTwoPop <= dayOnePop {
		t.Errorf("a distinct event (1002) after event 1001 grew pop %d -> %d, want further growth — the claim must not skip legitimate distinct events",
			dayOnePop, dayTwoPop)
	}
}
