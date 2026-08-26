package loyalty

// Recovered from an uncommitted 2026-08-16 test file
// (.claude/worktrees/agent-a75e1ca3749188909/server/internal/loyalty/borrowed_army_test.go)
// found during a 2026-08-26 worktree cleanup. That file had five tests; only
// this one survived triage against today's master — see the triage session
// notes for the other four (two were duplicates of
// borrowed_army_idempotent_test.go's existing coverage, one asserted a
// design master did not implement, one referenced a symbol
// (lenderClaimNamespace) that does not exist in current code).
//
// This is the one gap borrowed_army_idempotent_test.go's fixture doesn't
// cover: its single fixture always seeds a 20-day-overdue borrow (past both
// the day-7 king-kharis and day-14 lender-loyalty thresholds), so nothing in
// master currently proves a borrow held UNDER the lender threshold applies
// no penalty at all.
//
// Its own fixture is intentionally separate from borrowedArmyFixture in
// borrowed_army_idempotent_test.go (same name would collide) and seeds only a
// lender + capital, with no kingdom_members row.
//
// Note what that means at daysHeld = 10: the borrow is past the day-7 king
// threshold, so the king-kharis branch DOES run, finds no 'basileus' member,
// and logs "find king settlement: no rows in result set". That log line is
// expected, not a failure — penaliseKingKharis is deliberately non-fatal so a
// kingless kingdom cannot block the lender-loyalty half. This test asserts
// only the lender half; the king half is covered end-to-end by
// TestBorrowedArmyPenaltyHandler_KingKharisDrainsAndIsIdempotent.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type underThresholdFixture struct {
	pool          *pgxpool.Pool
	worldID       uuid.UUID
	kingdomID     uuid.UUID
	lenderID      uuid.UUID
	lenderCapital uuid.UUID
}

func newUnderThresholdFixture(t *testing.T) *underThresholdFixture {
	t.Helper()
	pool := loyaltyTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 0) RETURNING id`,
		"test-borrowedarmy-underthreshold-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var kingdomID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO kingdoms (world_id, name, state) VALUES ($1, $2, 'active') RETURNING id`,
		worldID, "Kingdom-underthreshold-"+uuid.New().String(),
	).Scan(&kingdomID); err != nil {
		t.Fatalf("create kingdom: %v", err)
	}

	var lenderID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"lender-underthreshold-"+uuid.New().String(), "lender-underthreshold-"+uuid.New().String()+"@test.invalid",
	).Scan(&lenderID); err != nil {
		t.Fatalf("create lender player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}

	var lenderCapital uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'LenderCapital', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, provinceID, lenderID,
	).Scan(&lenderCapital); err != nil {
		t.Fatalf("create lender capital: %v", err)
	}

	return &underThresholdFixture{
		pool:          pool,
		worldID:       worldID,
		kingdomID:     kingdomID,
		lenderID:      lenderID,
		lenderCapital: lenderCapital,
	}
}

func (f *underThresholdFixture) seedBorrow(t *testing.T, daysAgo int) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO borrowed_armies (kingdom_id, lender_id, infantry, borrowed_at)
		 VALUES ($1, $2, 100, now() - make_interval(days => $3))`,
		f.kingdomID, f.lenderID, daysAgo,
	); err != nil {
		t.Fatalf("seed borrow: %v", err)
	}
}

func (f *underThresholdFixture) enqueueEvent(t *testing.T) events.ScheduledEvent {
	t.Helper()
	var id int64
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO scheduled_events (world_id, event_type, payload, process_after, due_tick)
		 VALUES ($1, $2, '{}', now(), 0) RETURNING id`,
		f.worldID, string(events.ScheduledBorrowedArmyTick),
	).Scan(&id); err != nil {
		t.Fatalf("enqueue scheduled event: %v", err)
	}
	return events.ScheduledEvent{ID: id, WorldID: f.worldID, EventType: events.ScheduledBorrowedArmyTick, DueTick: 0}
}

func (f *underThresholdFixture) handler() *BorrowedArmyPenaltyHandler {
	clk := clock.NewTestClock(time.Now())
	store := events.NewStore(f.pool)
	sched := events.NewScheduler(f.pool, clk)
	return NewBorrowedArmyPenaltyHandler(f.pool, sched, store, clk)
}

func (f *underThresholdFixture) loyaltyEventCount(t *testing.T, settlementID uuid.UUID, eventType string) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM loyalty_events WHERE settlement_id = $1 AND event_type = $2`,
		settlementID, eventType,
	).Scan(&n); err != nil {
		t.Fatalf("count loyalty_events: %v", err)
	}
	return n
}

// TestBorrowedArmyPenalty_UnderThresholdAppliesNothing is a sanity check that
// a borrow held under the day-14 lender threshold does not touch lender
// loyalty at all. Unique coverage: borrowed_army_idempotent_test.go's fixture
// always seeds a 20-day-overdue borrow, so nothing else in the package
// exercises the below-threshold no-op path.
func TestBorrowedArmyPenalty_UnderThresholdAppliesNothing(t *testing.T) {
	f := newUnderThresholdFixture(t)
	f.seedBorrow(t, 10) // >= 7 (king threshold) but < 14 (lender threshold)

	h := f.handler()
	e := f.enqueueEvent(t)

	if err := h.Handle(context.Background(), e); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := f.loyaltyEventCount(t, f.lenderCapital, "borrowed_army_penalty"); got != 0 {
		t.Errorf("borrowed_army_penalty events = %d, want 0 (under the 14-day lender threshold)", got)
	}
}
