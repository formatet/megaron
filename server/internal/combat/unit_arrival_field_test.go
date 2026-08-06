package combat

// P2 reproduction (2026-07-18 soak, "Dole mot Eastern Outpost"): before the
// fix in unit_arrival_field.go, a marching unit arriving on a hex where only
// a hostile field-positioned unit sat (no settlement row there) fell through
// resolve()'s hasSettlement gate straight into arriveGarrison — the arriving
// unit simply co-located with the enemy, no combat at all. This test drives
// resolve() (not resolveFieldCombat directly) so it also proves the gate in
// resolve() itself routes to combat instead of a peaceful arrival.
//
// KR3 update (megaron_plan_kr3_stridssystem.md §1): resolve() no longer
// resolves the fight itself — it only initiates a persistent battles row via
// initiateOrJoinBattle. This test now asserts that initiation (a battle +
// both participants exist right after arrival, still at full size), then
// drives BattleTickHandler to completion to reach the same final outcome the
// old one-shot test asserted directly.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestResolve_HostileFieldUnitOnSettlementlessHexTriggersCombat(t *testing.T) {
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
	// no loss on either side, because no battle was even initiated. The fix:
	// resolve() no longer no-ops — the arriving unit is immediately positioned
	// (holding the contested hex) and a battles row now exists for (1,0).
	var attackerStatus string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM units WHERE id = $1`, attackerUnitID,
	).Scan(&attackerStatus); err != nil {
		t.Fatalf("read attacker unit: %v", err)
	}
	if attackerStatus != "positioned" {
		t.Errorf("attacker unit status = %q, want \"positioned\" (holding the contested hex while KR3 resolves it) — no combat occurred, the P2 bug is back", attackerStatus)
	}

	var battleID uuid.UUID
	var battleStatus string
	if err := pool.QueryRow(ctx,
		`SELECT id, status FROM battles WHERE world_id = $1 AND q = 1 AND r = 0`, worldID,
	).Scan(&battleID, &battleStatus); err != nil {
		t.Fatalf("read battle: %v (no battles row — initiateOrJoinBattle did not run)", err)
	}
	if battleStatus != "active" {
		t.Fatalf("battle status = %q, want \"active\" immediately after initiation — no dice have been rolled yet", battleStatus)
	}

	var participantCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM battle_participants WHERE battle_id = $1`, battleID,
	).Scan(&participantCount); err != nil {
		t.Fatalf("count battle participants: %v", err)
	}
	if participantCount != 2 {
		t.Errorf("battle_participants count = %d, want 2 (attacker + defender)", participantCount)
	}

	// Drive the battle to its conclusion (§2's state machine, not resolve()
	// itself) — with 100x the strength, the attacker must annihilate the
	// defender within a handful of battle-ticks.
	battleH := NewBattleTickHandler(pool, h.eventStore, h.scheduler)
	runBattleToEnd(t, pool, battleH, worldID, battleID, 20)

	var defenderStatus string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM units WHERE id = $1`, defenderUnitID,
	).Scan(&defenderStatus); err != nil {
		t.Fatalf("read defender unit: %v", err)
	}
	if defenderStatus != "disbanded" {
		t.Errorf("defender unit status = %q, want \"disbanded\" (overwhelmed attacker must destroy it within 20 battle-ticks)", defenderStatus)
	}

	var attackerSize int
	if err := pool.QueryRow(ctx, `SELECT size FROM units WHERE id = $1`, attackerUnitID).Scan(&attackerSize); err != nil {
		t.Fatalf("read attacker size: %v", err)
	}
	// A resolved KR3 battle rolls discrete T12 dice (§4) rather than a
	// deterministic %-loss formula — an overwhelming 1000-vs-10 win can
	// plausibly cost the winner zero men, so this only asserts it never
	// GAINS men and the battle actually concluded (defender wiped above).
	if attackerSize > 1000 || attackerSize <= 0 {
		t.Errorf("attacker size = %d, want in (0, 1000]", attackerSize)
	}
}
