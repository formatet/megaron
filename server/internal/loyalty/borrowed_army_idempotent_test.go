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
// Both branches — king-kharis and lender-loyalty — are now exercised end to end.
// penaliseKingKharis previously failed silently before it could mutate anything,
// for two reasons unrelated to idempotency: (1) it queried kingdom_members.role =
// 'king', but the role check constraint only allows 'basileus'/'member'/'lochagos'/
// 'navarchos' (renamed in 75e02ac/migration 007+046), so the king lookup returned
// pgx.ErrNoRows every time; (2) even past that, its UPDATE targeted
// settlements.kharis_amount/rate/calc_tick, columns migration 029 moved to
// player_world_records. Both are fixed (role → 'basileus', drain the per-Wanax
// player_world_records pool). Kingdoms are POST-MVP and gated off, so this path
// does not run live, but the handler is now correct for when they return.
// TestBorrowedArmyPenaltyHandler_KingKharisDrainsAndIsIdempotent proves the drain
// fires exactly once across a replay; TestBorrowedArmyPenaltyHandler_ReplayIsIdempotent
// covers the lender-loyalty path.

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

// kingKharisStart is the king's seeded realm-pool kharis in the fixture. The
// penalty drains 5 (see penaliseKingKharis), so a single fired penalty leaves
// kingKharisStart - 5.
const kingKharisStart = 50.0

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
			`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
			tag+"-"+role+"-"+uuid.New().String(),
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

	// Seed the per-Wanax kharis pool (player_world_records, migration 029) for
	// both players. rate = 0 so settled() == kharis_amount regardless of tick.
	// The king starts at kingKharisStart so penaliseKingKharis has something to
	// drain and the drain is observable.
	for _, p := range []uuid.UUID{f.kingPlayer, f.lenderPlayer} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO player_world_records (player_id, world_id, kharis_amount, kharis_rate, kharis_calc_tick)
			 VALUES ($1, $2, $3, 0, 0)`,
			p, f.worldID, kingKharisStart,
		); err != nil {
			t.Fatalf("seed player_world_records: %v", err)
		}
	}

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
// the mechanism that prevents a double-drain: the penalty fires exactly once
// across a replay of the same event, both in the claim row AND in the observable
// kharis drain.
func TestBorrowedArmyPenaltyHandler_KingKharisDrainsAndIsIdempotent(t *testing.T) {
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
	kingKharis := func() float64 {
		var v float64
		if err := pool.QueryRow(ctx,
			`SELECT settled(kharis_amount, kharis_rate, kharis_calc_tick)
			 FROM player_world_records WHERE player_id = $1 AND world_id = $2`,
			f.kingPlayer, f.worldID,
		).Scan(&v); err != nil {
			t.Fatalf("read king kharis: %v", err)
		}
		return v
	}

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (first run): %v", err)
	}
	if n := claimCount(); n != 1 {
		t.Fatalf("expected exactly 1 king_kharis claim row after the first run, got %d — fixture does not exercise the branch as intended", n)
	}
	// The drain must actually have fired (proves the role + column fix, not just
	// the claim). 20-day-overdue borrow is past the day-7 king-kharis threshold.
	if got, want := kingKharis(), kingKharisStart-5; got != want {
		t.Fatalf("king kharis after first run = %.4f, want %.4f (start %.0f − 5) — penaliseKingKharis did not drain; role/column fix regressed", got, want, kingKharisStart)
	}

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (replay): %v", err)
	}
	if n := claimCount(); n != 1 {
		t.Errorf("king_kharis claim rows after replay = %d, want unchanged 1 (event_id %d replayed — without the guard, Handle would attempt the work twice)", n, fixedEventID)
	}
	if got, want := kingKharis(), kingKharisStart-5; got != want {
		t.Errorf("king kharis after replay = %.4f, want unchanged %.4f (event %d replayed — a non-idempotent penalty would drain a second 5 to %.0f)", got, want, fixedEventID, kingKharisStart-10)
	}
}
