package loyalty

// G2 idempotency regression for BorrowedArmyPenaltyHandler (ScheduledBorrowedArmyTick).
//
// Same bug class migration 098 and colony_idempotent_test.go document: Handle fans
// ONE scheduled event out across every overdue borrowed army and mutates directly
// (no FOR UPDATE, no replay-from-log). A crash or G2 handler timeout between the
// first borrow's write and Handle finishing leaves the event unprocessed, so
// events.Worker re-claims and re-runs it from the top — before this fix that would
// double-drain the king's kharis and double-apply the lender's -1 loyalty. The fix
// adds a processed_tick_claims claim per (event_id, scope), scoped off the
// borrowed_armies row id rather than the settlement id directly, because a single
// king or lender can have several overdue borrows resolving to the SAME capital
// settlement within one event pass — a claim keyed on settlement_id alone would let
// the second borrow's claim collide with the first's and silently skip a penalty
// that should fire.
//
// NOTE on coverage: penaliseKingKharis currently fails before it can mutate
// anything, for two reasons unrelated to idempotency and pre-dating this fix —
// (1) it queries kingdom_members.role = 'king', but the role check constraint only
// allows 'basileus'/'member'/'lochagos'/'navarchos' (renamed in 75e02ac, this file
// never updated), so the king-settlement lookup returns pgx.ErrNoRows every time;
// (2) even past that, its UPDATE targets settlements.kharis_amount/kharis_rate/
// kharis_calc_tick, which migration 029 moved to player_world_records — those
// columns no longer exist on settlements. Both are latent, pre-existing bugs
// (kingdoms are POST-MVP and gated off, so this path does not run live) flagged
// separately, not fixed here per this task's scope. Since the king-kharis mutation
// never succeeds either before or after this fix, there is nothing to observe on
// that side beyond the claim row itself — this test proves that in isolation
// (TestBorrowedArmyPenaltyHandler_KingKharisClaimIsIdempotentEvenWhenWorkFails).
// The lender-loyalty path has no such schema drift and is fully exercised end to
// end (TestBorrowedArmyPenaltyHandler_ReplayIsIdempotent).

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type borrowedArmyFixture struct {
	worldID      uuid.UUID
	kingdomID    uuid.UUID
	kingPlayer   uuid.UUID
	lenderPlayer uuid.UUID
	kingCapital  uuid.UUID
	lenderCap    uuid.UUID
	borrowID     uuid.UUID
	tick         int
}

// newBorrowedArmyFixture creates one active world, a kingdom with a basileus and
// one member, both players' capitals, and one borrowed_armies row 20 days
// overdue — past both the day-7 (king kharis) and day-14 (lender loyalty)
// thresholds, so a single Handle() call exercises both branches.
func newBorrowedArmyFixture(t *testing.T, pool *pgxpool.Pool, tag string) borrowedArmyFixture {
	t.Helper()
	ctx := context.Background()
	const worldTick = 500

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}

	f := borrowedArmyFixture{tick: worldTick}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-"+tag+"-"+uuid.New().String(), worldTick,
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})

	mkPlayer := func(role string) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
			tag+"-"+role+"-"+uuid.New().String(), tag+"-"+role+"-"+uuid.New().String()+"@test.invalid",
		).Scan(&id); err != nil {
			t.Fatalf("create player (%s): %v", role, err)
		}
		return id
	}
	f.kingPlayer = mkPlayer("king")
	f.lenderPlayer = mkPlayer("lender")

	if err := pool.QueryRow(ctx,
		`INSERT INTO kingdoms (world_id, name, state) VALUES ($1, $2, 'active') RETURNING id`,
		f.worldID, tag+"-kingdom-"+uuid.New().String(),
	).Scan(&f.kingdomID); err != nil {
		t.Fatalf("create kingdom: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO kingdom_members (kingdom_id, player_id, role) VALUES ($1, $2, 'basileus')`,
		f.kingdomID, f.kingPlayer,
	); err != nil {
		t.Fatalf("add king member: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO kingdom_members (kingdom_id, player_id, role) VALUES ($1, $2, 'member')`,
		f.kingdomID, f.lenderPlayer,
	); err != nil {
		t.Fatalf("add lender member: %v", err)
	}

	mkCapital := func(q int, name string, owner uuid.UUID) uuid.UUID {
		var prov, sid uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
			f.worldID, q,
		).Scan(&prov); err != nil {
			t.Fatalf("create province: %v", err)
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true, 'active', 1000) RETURNING id`,
			f.worldID, prov, name, owner,
		).Scan(&sid); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return sid
	}
	f.kingCapital = mkCapital(0, "King-Capital", f.kingPlayer)
	f.lenderCap = mkCapital(1, "Lender-Capital", f.lenderPlayer)

	if err := pool.QueryRow(ctx,
		`INSERT INTO borrowed_armies (kingdom_id, lender_id, infantry, chariot, ship, borrowed_at)
		 VALUES ($1, $2, 10, 0, 0, now() - interval '20 days') RETURNING id`,
		f.kingdomID, f.lenderPlayer,
	).Scan(&f.borrowID); err != nil {
		t.Fatalf("create borrowed army: %v", err)
	}

	return f
}

// TestBorrowedArmyPenaltyHandler_ReplayIsIdempotent proves the lender-loyalty
// branch (daysHeld >= 14) is exactly-once across a replay of the SAME scheduled
// event — this branch has no schema drift and runs end to end.
func TestBorrowedArmyPenaltyHandler_ReplayIsIdempotent(t *testing.T) {
	pool := loyaltyTestPool(t)
	ctx := context.Background()
	f := newBorrowedArmyFixture(t, pool, "ba-idem")

	h := NewBorrowedArmyPenaltyHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())), events.NewStore(pool), clock.NewTestClock(time.Now()))
	// Fixed event ID so the second Handle call is a genuine replay of the SAME
	// scheduled event, not a fresh one — that is the exact scenario the
	// processed_tick_claims guard exists for.
	const fixedEventID int64 = 991001
	evt := events.ScheduledEvent{ID: fixedEventID, WorldID: f.worldID, DueTick: f.tick}

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (first run): %v", err)
	}

	var pointsAfterFirst float64
	if err := pool.QueryRow(ctx, `SELECT loyalty_points FROM settlements WHERE id = $1`, f.lenderCap).Scan(&pointsAfterFirst); err != nil {
		t.Fatalf("read loyalty_points after first run: %v", err)
	}
	var countAfterFirst int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM loyalty_events WHERE settlement_id = $1 AND event_type = 'borrowed_army_penalty'`,
		f.lenderCap,
	).Scan(&countAfterFirst); err != nil {
		t.Fatalf("count borrowed_army_penalty events after first run: %v", err)
	}

	// Sanity: the penalty must actually have fired once (fixture is 20 days
	// overdue, past the day-14 threshold), otherwise this test would trivially
	// pass even without the idempotency guard.
	if countAfterFirst != 1 {
		t.Fatalf("expected exactly 1 borrowed_army_penalty event after the first run, got %d — fixture does not exercise the handler as intended", countAfterFirst)
	}

	// Replay the SAME event.
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (replay): %v", err)
	}

	var pointsAfterReplay float64
	if err := pool.QueryRow(ctx, `SELECT loyalty_points FROM settlements WHERE id = $1`, f.lenderCap).Scan(&pointsAfterReplay); err != nil {
		t.Fatalf("read loyalty_points after replay: %v", err)
	}
	if pointsAfterReplay != pointsAfterFirst {
		t.Errorf("lender loyalty_points after replay = %v, want unchanged %v (event_id %d replayed — a non-idempotent handler would move it twice)",
			pointsAfterReplay, pointsAfterFirst, fixedEventID)
	}

	var countAfterReplay int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM loyalty_events WHERE settlement_id = $1 AND event_type = 'borrowed_army_penalty'`,
		f.lenderCap,
	).Scan(&countAfterReplay); err != nil {
		t.Fatalf("count borrowed_army_penalty events after replay: %v", err)
	}
	if countAfterReplay != countAfterFirst {
		t.Errorf("borrowed_army_penalty loyalty_events rows after replay = %d, want unchanged %d", countAfterReplay, countAfterFirst)
	}
}

// TestBorrowedArmyPenaltyHandler_KingKharisClaimIsIdempotentEvenWhenWorkFails
// proves the king-kharis claim itself (scope derived from the borrow row, suffix
// "king_kharis") is taken at most once per event, independent of whether the
// downstream mutation succeeds. See the file-level NOTE: penaliseKingKharis's
// own work currently always errors (pre-existing, unrelated schema drift — role
// 'king' vs 'basileus', and settlements no longer has kharis_amount), so this is
// the only observable proof available for that branch today, but it is exactly
// the mechanism that would prevent a double-drain once the downstream bugs are
// fixed and the mutation starts succeeding.
func TestBorrowedArmyPenaltyHandler_KingKharisClaimIsIdempotentEvenWhenWorkFails(t *testing.T) {
	pool := loyaltyTestPool(t)
	ctx := context.Background()
	f := newBorrowedArmyFixture(t, pool, "ba-kharis-claim")

	h := NewBorrowedArmyPenaltyHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())), events.NewStore(pool), clock.NewTestClock(time.Now()))
	const fixedEventID int64 = 991002
	evt := events.ScheduledEvent{ID: fixedEventID, WorldID: f.worldID, DueTick: f.tick}

	scope := uuid.NewSHA1(f.borrowID, []byte("king_kharis"))
	claimCount := func() int {
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM processed_tick_claims WHERE event_id = $1 AND scope_id = $2`,
			fixedEventID, scope,
		).Scan(&n); err != nil {
			t.Fatalf("count processed_tick_claims: %v", err)
		}
		return n
	}

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (first run): %v", err)
	}
	if n := claimCount(); n != 1 {
		t.Fatalf("expected exactly 1 king_kharis claim row after the first run, got %d — fixture does not exercise the branch as intended", n)
	}

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (replay): %v", err)
	}
	if n := claimCount(); n != 1 {
		t.Errorf("king_kharis claim rows after replay = %d, want unchanged 1 (event_id %d replayed — without the guard, Handle would attempt the work twice)", n, fixedEventID)
	}
}
