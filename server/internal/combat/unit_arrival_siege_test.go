package combat

// Settlement siege (land march onto an enemy city). KR3 cutover
// (megaron_plan_kr3_stridssystem.md §8): resolveCombat no longer resolves the
// fight itself — it only initiates a persistent battles row via
// initiateOrJoinBattle, sending in EVERY garrison unit as its own defender
// participant (multi-garrison was never a blocker — battle.go already
// carries N participants per side). This mirrors unit_arrival_field_test.go's
// two-phase pattern: assert initiation right after resolve(), then drive
// BattleTickHandler to completion. It does not assert settlement
// capture/ownership — resolveCombat does not perform that anymore (battle.go
// header comment: a deliberate, still-open gap).

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestResolveCombat_SettlementSiege_AllGarrisonUnitsJoinAsDefenders(t *testing.T) {
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
		 VALUES ($1, $2, 'Besieger Home', 'achaean', $3, 'capital', true, 'active', 8000)`,
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
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population, wall_level)
		 VALUES ($1, $2, 'Holdout City', 'khemetiu', $3, 'capital', true, 'active', 9000, 0) RETURNING id`,
		worldID, defProv, defender,
	).Scan(&defSettlement); err != nil {
		t.Fatalf("create defender settlement: %v", err)
	}
	// TWO garrison units — the flergarnison assertion: both must become their
	// own battle_participants row, not just the first one found.
	var garrisonA, garrisonB uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 80, 0, 'garrison', $3) RETURNING id`,
		worldID, defender, defSettlement,
	).Scan(&garrisonA); err != nil {
		t.Fatalf("create garrison A: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'elite_infantry', 'land', 20, 0, 'garrison', $3) RETURNING id`,
		worldID, defender, defSettlement,
	).Scan(&garrisonB); err != nil {
		t.Fatalf("create garrison B: %v", err)
	}

	var attackerUnitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, target_q, target_r, capture_mode)
		 VALUES ($1, $2, 'spearman', 'land', 2000, 0, 'marching', 0, 0, 1, 0, 'sack') RETURNING id`,
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

	// ── Phase 1: initiation only. ──

	var attackerStatus string
	var attackerSettlement *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT status, settlement_id FROM units WHERE id = $1`, attackerUnitID,
	).Scan(&attackerStatus, &attackerSettlement); err != nil {
		t.Fatalf("read attacker unit: %v", err)
	}
	if attackerStatus != "positioned" || attackerSettlement != nil {
		t.Errorf("attacker status=%q settlement=%v, want positioned/nil — holding the contested hex while KR3 resolves it, no capture", attackerStatus, attackerSettlement)
	}

	var battleID uuid.UUID
	var battleStatus string
	if err := pool.QueryRow(ctx,
		`SELECT id, status FROM battles WHERE world_id = $1 AND q = 1 AND r = 0`, worldID,
	).Scan(&battleID, &battleStatus); err != nil {
		t.Fatalf("read battle: %v (no battles row — initiateOrJoinBattle did not run)", err)
	}
	if battleStatus != "active" {
		t.Fatalf("battle status = %q, want active", battleStatus)
	}

	var defenderParticipants int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM battle_participants WHERE battle_id = $1 AND side = 'defender'`, battleID,
	).Scan(&defenderParticipants); err != nil {
		t.Fatalf("count defender participants: %v", err)
	}
	if defenderParticipants != 2 {
		t.Errorf("defender battle_participants = %d, want 2 (both garrison units, flergarnison)", defenderParticipants)
	}
	var attackerParticipants int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM battle_participants WHERE battle_id = $1 AND side = 'attacker'`, battleID,
	).Scan(&attackerParticipants); err != nil {
		t.Fatalf("count attacker participants: %v", err)
	}
	if attackerParticipants != 1 {
		t.Errorf("attacker battle_participants = %d, want 1", attackerParticipants)
	}

	// ── Phase 2: drive the battle to its conclusion. ──

	battleH := NewBattleTickHandler(pool, h.eventStore, h.scheduler, nil)
	runBattleToEnd(t, pool, battleH, worldID, battleID, 30)

	for name, id := range map[string]uuid.UUID{"garrison A": garrisonA, "garrison B": garrisonB} {
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, id).Scan(&status); err != nil {
			t.Fatalf("read %s after battle: %v", name, err)
		}
		if status != "disbanded" {
			t.Errorf("%s status = %q, want disbanded (2000 vs 100 total across two units must annihilate both within 30 battle-ticks)", name, status)
		}
	}

	// No capture: this cutover does not transfer settlement ownership.
	var stillOwner uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id = $1`, defSettlement).Scan(&stillOwner); err != nil {
		t.Fatalf("read settlement owner: %v", err)
	}
	if stillOwner != defender {
		t.Errorf("settlement owner = %s, want unchanged defender %s (capture-on-win is not implemented by this cutover)", stillOwner, defender)
	}
}
