package combat

// P2 reproduction (2026-07-18 soak, "Dole mot Eastern Outpost"): before the
// fix in unit_arrival_field.go, a marching unit arriving on a hex where only
// a hostile field-positioned unit sat (no settlement row there) fell through
// resolve()'s hasSettlement gate straight into arriveGarrison — the arriving
// unit simply co-located with the enemy, no combat at all. This test drives
// resolve() (not resolveFieldCombat directly) so it also proves the gate in
// resolve() itself routes to combat instead of a peaceful arrival.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
)

func TestResolve_HostileFieldUnitOnSettlementlessHexTriggersCombat(t *testing.T) {
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
	attacker := mkPlayer("attacker")
	defender := mkPlayer("defender")

	// Attacker's capital at (0,0) — needed for the pop-loss / kharis lookups
	// combat touches. Target hex (1,0): an empty province (no settlement row)
	// held only by the defender's field-positioned unit — the soak-observed
	// "Eastern Outpost".
	var attCapProv uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&attCapProv); err != nil {
		t.Fatalf("create attacker capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Attacker Home', 'achaean', $3, 'capital', true, 'active', 8000)`,
		worldID, attCapProv, attacker,
	); err != nil {
		t.Fatalf("create attacker capital: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 1, 0, 'plains')`,
		worldID,
	); err != nil {
		t.Fatalf("create target province: %v", err)
	}

	// Defender's field-positioned unit sitting at (1,0) with no settlement.
	var defenderUnitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 10, 0, 'positioned', 1, 0) RETURNING id`,
		worldID, defender,
	).Scan(&defenderUnitID); err != nil {
		t.Fatalf("create defender field unit: %v", err)
	}

	// Attacker's overwhelming force, marching (arriving) at (1,0).
	var attackerUnitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, target_q, target_r, capture_mode)
		 VALUES ($1, $2, 'spearman', 'land', 1000, 0, 'marching', 0, 0, 1, 0, 'sack') RETURNING id`,
		worldID, attacker,
	).Scan(&attackerUnitID); err != nil {
		t.Fatalf("create attacker unit: %v", err)
	}

	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		hub:        &fakeBroadcaster{},
		scheduler:  events.NewScheduler(pool, clock.NewTestClock(time.Now())),
		clk:        clock.NewTestClock(time.Now()),
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := h.resolve(ctx, tx, attackerUnitID, worldID); err != nil {
		tx.Rollback(ctx)
		t.Fatalf("resolve: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// The bug: both units ending up 'positioned' at (1,0) with full size and
	// no loss on either side. The fix: combat resolved — with 100x the
	// strength, the attacker must win, so the defender's unit is disbanded
	// (or reduced) and the attacker is positioned there having taken some
	// losses (not still at its pre-battle size of 1000).
	var defenderStatus string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM units WHERE id = $1`, defenderUnitID,
	).Scan(&defenderStatus); err != nil {
		t.Fatalf("read defender unit: %v", err)
	}
	if defenderStatus != "disbanded" {
		t.Errorf("defender unit status = %q, want \"disbanded\" (overwhelmed attacker must destroy it) — no combat occurred, the P2 bug is back", defenderStatus)
	}

	var attackerStatus string
	var attackerSize int
	if err := pool.QueryRow(ctx,
		`SELECT status, size FROM units WHERE id = $1`, attackerUnitID,
	).Scan(&attackerStatus, &attackerSize); err != nil {
		t.Fatalf("read attacker unit: %v", err)
	}
	if attackerStatus != "positioned" {
		t.Errorf("attacker unit status = %q, want \"positioned\" (victorious field battle)", attackerStatus)
	}
	if attackerSize >= 1000 {
		t.Errorf("attacker size = %d, want < 1000 (a resolved battle applies at least some losses)", attackerSize)
	}
}
