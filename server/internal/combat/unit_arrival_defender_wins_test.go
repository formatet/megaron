package combat

// Regression test for the r6 legibility audit (2026-07-24, megaron_todo.md):
// when a besieging unit is beaten back by a settlement's garrison
// (applyDefenderWins), NEITHER side got a notification — the attacker's unit
// vanished/routed/bled and the defender's garrison took losses
// (applyDefenderUnitLosses) with nothing connecting either to "a siege
// happened here". Mirrors FieldBattleWon/FieldBattleLost (unit_arrival_field.go)
// and SettlementCaptured's shared-kind-with-role convention.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestApplyDefenderWins_NotifiesBothSides(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
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
	attacker := mkPlayer("besieger")
	defender := mkPlayer("holdout")

	var attCapProv uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&attCapProv); err != nil {
		t.Fatalf("create attacker capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Besieger Home', 'achaean', $3, 'capital', true, 'active', 5000)`,
		worldID, attCapProv, attacker,
	); err != nil {
		t.Fatalf("create attacker capital: %v", err)
	}

	var defProv uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 1, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&defProv); err != nil {
		t.Fatalf("create defender province: %v", err)
	}
	var defSettlement uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Holdout City', 'khemetiu', $3, 'capital', true, 'active', 4000) RETURNING id`,
		worldID, defProv, defender,
	).Scan(&defSettlement); err != nil {
		t.Fatalf("create defender settlement: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 200, 0, 'garrison', $3)`,
		worldID, defender, defSettlement,
	); err != nil {
		t.Fatalf("create defender garrison: %v", err)
	}

	var attackerUnitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, capture_mode)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'marching', 0, 0, 'annex') RETURNING id`,
		worldID, attacker,
	).Scan(&attackerUnitID); err != nil {
		t.Fatalf("create attacker unit: %v", err)
	}

	fb := &fakeBroadcaster{}
	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		hub:        fb,
		scheduler:  events.NewScheduler(pool, clock.NewTestClock(time.Now())),
		clk:        clock.NewTestClock(time.Now()),
	}

	u := unitRow{
		id: attackerUnitID, ownerID: attacker, utype: "spearman", category: "land",
		size: 100, status: "marching", q: 0, r: 0, captureMode: "annex",
	}
	destOwnerID := defender
	dest := destSettlement{
		provinceID: defProv, settlementID: &defSettlement, ownerID: &destOwnerID,
		wallLevel: 0, terrain: "plains",
	}
	// Attacker wiped out (attSizeAfter=0), no rout — the simplest, deterministic
	// "destroyed" branch.
	result := CombatResult{Outcome: OutcomeDefenderWins, DefenderLosses: 0.1, AttackerRouted: false}
	const attSizeAfter, attPopLost = 0, 100

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := h.applyDefenderWins(ctx, tx, u, dest, attSizeAfter, attPopLost, result, 1, 0, worldID); err != nil {
		tx.Rollback(ctx)
		t.Fatalf("applyDefenderWins: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var attackerStatus string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM units WHERE id = $1`, attackerUnitID,
	).Scan(&attackerStatus); err != nil {
		t.Fatalf("read attacker unit after defeat: %v", err)
	}
	if attackerStatus != "disbanded" {
		t.Errorf("attacker status = %q, want disbanded", attackerStatus)
	}

	count := 0
	for _, kind := range fb.notified {
		if kind == "SettlementDefended" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 SettlementDefended notifications (attacker + defender), got %d in %v", count, fb.notified)
	}
}
