package combat

// Stridsrapport S1 (megaron_plan_stridsrapport.md): a KR3 field battle used to
// end in total silence — resolveFieldCombat only ever initiates the battles
// row (see unit_arrival_field.go's header comment); nothing in
// BattleTickHandler called hub.NotifyPlayer when a battle concluded. This is
// the red-before this slice fixes: BOTH sides' owners should get a
// notification naming the opponent and their own/enemy losses, not just the
// winner (and not q/r-only, per the plan's payload skeleton).

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestBattleTickHandler_NotifiesBothSidesOnBattleEnd(t *testing.T) {
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

	mkPlayer := func(tag string) (uuid.UUID, string) {
		var id uuid.UUID
		wanaxName := tag + "Wanax-" + uuid.New().String()
		if err := pool.QueryRow(ctx,
			`INSERT INTO players (username, wanax_name, email, password_hash) VALUES ($1, $2, $3, 'x') RETURNING id`,
			tag+"-"+uuid.New().String(), wanaxName, tag+"-"+uuid.New().String()+"@test.invalid",
		).Scan(&id); err != nil {
			t.Fatalf("create player %s: %v", tag, err)
		}
		return id, wanaxName
	}
	attacker, attackerName := mkPlayer("attacker")
	defender, defenderName := mkPlayer("defender")

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

	var defenderUnitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 10, 0, 'positioned', 1, 0) RETURNING id`,
		worldID, defender,
	).Scan(&defenderUnitID); err != nil {
		t.Fatalf("create defender field unit: %v", err)
	}

	var attackerUnitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, target_q, target_r, capture_mode)
		 VALUES ($1, $2, 'spearman', 'land', 1000, 0, 'marching', 0, 0, 1, 0, 'sack') RETURNING id`,
		worldID, attacker,
	).Scan(&attackerUnitID); err != nil {
		t.Fatalf("create attacker unit: %v", err)
	}

	arrivalFB := &fakeBroadcaster{}
	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		hub:        arrivalFB,
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

	var battleID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM battles WHERE world_id = $1 AND q = 1 AND r = 0`, worldID,
	).Scan(&battleID); err != nil {
		t.Fatalf("read battle: %v", err)
	}

	battleFB := &fakeBroadcaster{}
	battleH := NewBattleTickHandler(pool, h.eventStore, h.scheduler, battleFB, h.clk)
	runBattleToEnd(t, pool, battleH, worldID, battleID, 20)

	var attackerNotified, defenderNotified map[string]any
	for i, k := range battleFB.notified {
		payload, ok := battleFB.payloads[i].(map[string]any)
		if !ok {
			t.Fatalf("payload %d is not map[string]any: %T", i, battleFB.payloads[i])
		}
		if k != "BattleWon" && k != "BattleLost" {
			t.Fatalf("unexpected notification kind %q", k)
		}
		switch payload["role"] {
		case "attacker":
			attackerNotified = payload
		case "defender":
			defenderNotified = payload
		default:
			t.Fatalf("payload has no recognisable role: %+v", payload)
		}
	}

	if attackerNotified == nil {
		t.Fatal("attacker owner never got a battle-end notification — the bug this slice fixes")
	}
	if defenderNotified == nil {
		t.Fatal("defender owner never got a battle-end notification — the bug this slice fixes (defender used to get NOTHING)")
	}

	if defenderNotified["opponent_name"] != attackerName {
		t.Errorf("defender's opponent_name = %v, want %q (wanax_name, not raw username/UUID)", defenderNotified["opponent_name"], attackerName)
	}
	if attackerNotified["opponent_name"] != defenderName {
		t.Errorf("attacker's opponent_name = %v, want %q", attackerNotified["opponent_name"], defenderName)
	}

	defOwn, ok := defenderNotified["own_unit"].(battleReportUnit)
	if !ok {
		t.Fatalf("defender own_unit has wrong type: %T", defenderNotified["own_unit"])
	}
	if defOwn.SizeBefore != 10 {
		t.Errorf("defender own_unit.size_before = %d, want 10", defOwn.SizeBefore)
	}
	if defOwn.PopLost != defOwn.SizeBefore-defOwn.SizeAfter {
		t.Errorf("defender own_unit.pop_lost = %d, want %d (size_before - size_after)", defOwn.PopLost, defOwn.SizeBefore-defOwn.SizeAfter)
	}

	if attackerNotified["place"] != nil {
		t.Errorf("place = %v, want absent — (1,0) has no settlement, this is open ground", attackerNotified["place"])
	}
}
