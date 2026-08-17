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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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
	// Grain must now be bit-for-bit unchanged on replay: BOTH the growth CTE
	// (per-settlement claim) AND the flat ×0.99 grain/timber decay (world-scoped
	// claim, closed alongside this test) are guarded, so a replay of the same
	// event moves nothing. Before the decay claim landed, replay left grain at
	// firstGrain × 0.99 (one extra shave); a non-idempotent growth CTE would
	// additionally draw ~desired_new × grainPerCitizen on top. Either regression
	// blows past this tolerance.
	if diff := replayGrain - firstGrain; diff > 0.5 || diff < -0.5 {
		t.Errorf("grain after replay = %.4f, want unchanged %.4f (event %d replayed — a re-shaved ×0.99 decay would leave %.4f, a re-fired growth CTE would draw ~desired_new×%.0f grain further)",
			replayGrain, firstGrain, fixedEventID, firstGrain*0.99, grainPerCitizen)
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

// readTimber reads the settled timber stock for a settlement.
func readTimber(t *testing.T, pool *pgxpool.Pool, settlementID uuid.UUID) float64 {
	t.Helper()
	var amount float64
	if err := pool.QueryRow(context.Background(),
		`SELECT COALESCE((SELECT settled(sg.amount, sg.rate, sg.calc_tick)
		                  FROM settlement_goods sg
		                  WHERE sg.settlement_id = $1 AND sg.good_key = 'timber'), 0)`,
		settlementID,
	).Scan(&amount); err != nil {
		t.Fatalf("read timber: %v", err)
	}
	return amount
}

// TestApplyDecay_GoodsDecayReplayIsIdempotent isolates the flat ×0.99
// grain/timber decay from the growth CTE. Timber has no growth interaction, so
// it is a clean probe of the decay claim alone: a replay of the SAME event must
// shave ×0.99 exactly ONCE (leaving amount×0.99), never twice (amount×0.99²).
func TestApplyDecay_GoodsDecayReplayIsIdempotent(t *testing.T) {
	terrains := [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}
	pool, worldID, settlementID := newGrowthFixture(t, terrains, 5000)
	h := newTestTickHandler(pool)
	ctx := context.Background()

	// Seed a fixed timber stock with rate 0 so settled() == amount (no accrual
	// muddies the decay reading). calc_tick = current so settled() is stable.
	const startTimber = 500.0
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'timber', $2, 0, 1000, current_world_tick())
		 ON CONFLICT (settlement_id, good_key)
		 DO UPDATE SET amount = $2, rate = 0, calc_tick = current_world_tick()`,
		settlementID, startTimber,
	); err != nil {
		t.Fatalf("seed timber row: %v", err)
	}

	const fixedEventID int64 = 515151

	h.applyDecay(ctx, worldID, fixedEventID)
	afterFirst := readTimber(t, pool, settlementID)
	wantFirst := startTimber * 0.99
	if diff := afterFirst - wantFirst; diff > 0.01 || diff < -0.01 {
		t.Fatalf("timber after first run = %.4f, want %.4f (= 500 × 0.99) — fixture does not exercise the decay path as intended", afterFirst, wantFirst)
	}

	// Replay the SAME event: the decay claim must make this a no-op.
	h.applyDecay(ctx, worldID, fixedEventID)
	afterReplay := readTimber(t, pool, settlementID)
	if diff := afterReplay - afterFirst; diff > 0.01 || diff < -0.01 {
		t.Errorf("timber after replay = %.4f, want unchanged %.4f (event %d replayed — an unclaimed decay would shave it to %.4f)",
			afterReplay, afterFirst, fixedEventID, afterFirst*0.99)
	}

	// The converse: a DISTINCT event must shave again (the claim must not turn
	// decay into a run-once-ever no-op).
	h.applyDecay(ctx, worldID, fixedEventID+1)
	afterDistinct := readTimber(t, pool, settlementID)
	wantDistinct := afterFirst * 0.99
	if diff := afterDistinct - wantDistinct; diff > 0.01 || diff < -0.01 {
		t.Errorf("timber after distinct event = %.4f, want %.4f (= previous × 0.99) — the world-scoped claim must not block a legitimately new event", afterDistinct, wantDistinct)
	}
}
