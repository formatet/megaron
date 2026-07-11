package combat

// L2 DB integration: a resolved battle moves the supplying settlements' loyalty
// through applyBattleLoyalty's tx path (AppendLoyaltyEventTx). Skips without
// DATABASE_URL, like the other combat integration tests.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
)

func TestApplyBattleLoyalty_MovesSettlementLoyalty(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	mkPlayer := func(tag string) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
			tag+"-"+uuid.New().String(), tag+"-"+uuid.New().String()+"@test.invalid",
		).Scan(&id); err != nil {
			t.Fatalf("create player %s: %v", tag, err)
		}
		return id
	}
	attacker := mkPlayer("winner")
	defender := mkPlayer("holder")

	mkSettlement := func(owner uuid.UUID, name string, q int) uuid.UUID {
		var prov uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
			worldID, q,
		).Scan(&prov); err != nil {
			t.Fatalf("create province for %s: %v", name, err)
		}
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population, loyalty)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true, 'active', 5000, 2) RETURNING id`,
			worldID, prov, name, owner,
		).Scan(&id); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return id
	}
	attCapital := mkSettlement(attacker, "Winner Home", 0)
	defCity := mkSettlement(defender, "Held City", 2)

	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		scheduler:  events.NewScheduler(pool, clock.NewTestClock(time.Now())),
		clk:        clock.NewTestClock(time.Now()),
	}

	loyaltyOf := func(id uuid.UUID) int {
		var l int
		if err := pool.QueryRow(ctx, `SELECT loyalty FROM settlements WHERE id = $1`, id).Scan(&l); err != nil {
			t.Fatalf("read loyalty: %v", err)
		}
		return l
	}
	eventCount := func(id uuid.UUID, eventType, reason string) int {
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM loyalty_events WHERE settlement_id = $1 AND event_type = $2 AND reason = $3`,
			id, eventType, reason,
		).Scan(&n); err != nil {
			t.Fatalf("count loyalty_events: %v", err)
		}
		return n
	}
	run := func(outcome Outcome) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		h.applyBattleLoyalty(ctx, tx, outcome, attCapital, true, &defCity, worldID)
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit tx: %v", err)
		}
	}

	// ── Attacker wins: its home gains loyalty; the (captured) defender is skipped. ──
	run(OutcomeAttackerWins)
	if got := loyaltyOf(attCapital); got != 3 {
		t.Errorf("attacker win: capital loyalty want 3, got %d", got)
	}
	if got := loyaltyOf(defCity); got != 2 {
		t.Errorf("attacker win: defender loyalty must be untouched (captured/razed), want 2, got %d", got)
	}
	if got := eventCount(attCapital, "shared_victory", "won_battle"); got != 1 {
		t.Errorf("attacker win: want 1 won_battle event, got %d", got)
	}

	// ── Defender wins: attacker's home loses loyalty; the held city gains it. ──
	if _, err := pool.Exec(ctx, `UPDATE settlements SET loyalty = 2 WHERE id = $1`, attCapital); err != nil {
		t.Fatalf("reset attacker loyalty: %v", err)
	}
	run(OutcomeDefenderWins)
	if got := loyaltyOf(attCapital); got != 1 {
		t.Errorf("defender win: attacker loyalty want 1, got %d", got)
	}
	if got := loyaltyOf(defCity); got != 3 {
		t.Errorf("defender win: defender loyalty want 3, got %d", got)
	}
	if got := eventCount(attCapital, "battle_lost", "lost_battle"); got != 1 {
		t.Errorf("defender win: want 1 lost_battle event, got %d", got)
	}
	if got := eventCount(defCity, "shared_victory", "defended_settlement"); got != 1 {
		t.Errorf("defender win: want 1 defended_settlement event, got %d", got)
	}
}
