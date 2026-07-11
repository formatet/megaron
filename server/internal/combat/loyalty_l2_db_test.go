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

	// Loyalty is a slow points accumulator: one battle moves loyalty_points by a
	// scaled ±1 intent (gain ×1.0, loss ×1.5), not a whole band. Seed points at a
	// band edge so a single event crosses into the next band, proving both the
	// points move AND the integer re-derivation.
	pointsOf := func(id uuid.UUID) float64 {
		var p float64
		if err := pool.QueryRow(ctx, `SELECT loyalty_points FROM settlements WHERE id = $1`, id).Scan(&p); err != nil {
			t.Fatalf("read loyalty_points: %v", err)
		}
		return p
	}
	loyaltyOf := func(id uuid.UUID) int {
		var l int
		if err := pool.QueryRow(ctx, `SELECT loyalty FROM settlements WHERE id = $1`, id).Scan(&l); err != nil {
			t.Fatalf("read loyalty: %v", err)
		}
		return l
	}
	setPoints := func(id uuid.UUID, p float64) {
		if _, err := pool.Exec(ctx, `UPDATE settlements SET loyalty_points = $2 WHERE id = $1`, id, p); err != nil {
			t.Fatalf("set loyalty_points: %v", err)
		}
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
	approx := func(got, want float64) bool { return got-want < 0.001 && want-got < 0.001 }
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

	// ── Attacker wins: +1 gain (×1.0) crosses band-2 ceiling → loyalty 2→3;
	// the (captured) defender is skipped. ──
	setPoints(attCapital, 49.5) // loyalty 2, just under Band2Ceil (50)
	run(OutcomeAttackerWins)
	if got := pointsOf(attCapital); !approx(got, 50.5) {
		t.Errorf("attacker win: capital points want 50.5, got %.3f", got)
	}
	if got := loyaltyOf(attCapital); got != 3 {
		t.Errorf("attacker win: capital loyalty want 3, got %d", got)
	}
	if got := pointsOf(defCity); !approx(got, 37) { // untouched (captured/razed)
		t.Errorf("attacker win: defender points must be untouched (37), got %.3f", got)
	}
	if got := eventCount(attCapital, "shared_victory", "won_battle"); got != 1 {
		t.Errorf("attacker win: want 1 won_battle event, got %d", got)
	}

	// ── Defender wins: attacker's home takes a -1 loss (×1.5 = -1.5) crossing
	// Band1Ceil → loyalty 2→1; the held city gains +1 crossing Band2Ceil → 2→3. ──
	setPoints(attCapital, 25.5) // loyalty 2, just above Band1Ceil (25)
	setPoints(defCity, 49.5)    // loyalty 2, just under Band2Ceil (50)
	run(OutcomeDefenderWins)
	if got := pointsOf(attCapital); !approx(got, 24.0) {
		t.Errorf("defender win: attacker points want 24.0, got %.3f", got)
	}
	if got := loyaltyOf(attCapital); got != 1 {
		t.Errorf("defender win: attacker loyalty want 1, got %d", got)
	}
	if got := pointsOf(defCity); !approx(got, 50.5) {
		t.Errorf("defender win: defender points want 50.5, got %.3f", got)
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
