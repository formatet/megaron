package combat

// Regression test for the r6 legibility audit (2026-07-24, megaron_todo.md):
// resolveFieldCombat (unit_arrival_field.go) supports mixed-owner defender
// stacks — applyFieldDefenderLosses already applies pop-loss per distinct
// owner via totalsByOwner — but the battle NOTIFICATIONS only ever went to
// defenders[0].ownerID, so a hex held by two different Wanax's field units
// left the second owner with no FieldBattleLost/FieldBattleWon at all.
//
// KR3 update (megaron_plan_kr3_stridssystem.md §1/§8): resolveFieldCombat no
// longer sends FieldBattleWon/Lost notifications at all — like the sibling
// avsiktslagret §S3 scan (unit_intercept_scan.go), a notification kind with
// no renderer on the other end is a known anti-pattern here, and the KR3
// stridsrapport payload (megaron_plan_stridsrapport.md) is explicitly a later
// slice. So the "was owner B silently dropped" question this test guards now
// has a structural analogue instead of a notification one: both distinct
// defender owners must be registered as their own battle_participants row
// (not merged into/shadowed by defenders[0]), and both units must actually
// take the battle's outcome once BattleTickHandler resolves it.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// recordingBroadcaster records (playerID, kind) pairs — fakeBroadcaster
// (unit_arrival_notify_test.go) only records kind, which can't distinguish
// which owner among a mixed-owner defender stack was notified.
type recordingBroadcaster struct {
	notified []struct {
		playerID uuid.UUID
		kind     string
	}
}

func (f *recordingBroadcaster) BroadcastEvent(worldID uuid.UUID, kind string, payload any) {}

func (f *recordingBroadcaster) NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error {
	f.notified = append(f.notified, struct {
		playerID uuid.UUID
		kind     string
	}{playerID, kind})
	return nil
}

func TestResolveFieldCombat_NotifiesAllDistinctDefenderOwners(t *testing.T) {
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
	defenderA := mkPlayer("defender-a")
	defenderB := mkPlayer("defender-b")

	// Attacker's capital at (0,0) — needed for the pop-loss / kharis lookups
	// combat touches. Target hex (1,0): an empty province (no settlement row)
	// held by TWO field-positioned units from two DIFFERENT owners.
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

	// Two field-positioned defenders at (1,0), owned by two different players.
	var defenderAUnitID, defenderBUnitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 10, 0, 'positioned', 1, 0) RETURNING id`,
		worldID, defenderA,
	).Scan(&defenderAUnitID); err != nil {
		t.Fatalf("create defender-a field unit: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 10, 0, 'positioned', 1, 0) RETURNING id`,
		worldID, defenderB,
	).Scan(&defenderBUnitID); err != nil {
		t.Fatalf("create defender-b field unit: %v", err)
	}

	// Attacker's overwhelming force, marching (arriving) at (1,0) — forces a
	// deterministic attacker-wins outcome (mirrors unit_arrival_field_test.go).
	var attackerUnitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, target_q, target_r, capture_mode)
		 VALUES ($1, $2, 'spearman', 'land', 1000, 0, 'marching', 0, 0, 1, 0, 'sack') RETURNING id`,
		worldID, attacker,
	).Scan(&attackerUnitID); err != nil {
		t.Fatalf("create attacker unit: %v", err)
	}

	fb := &recordingBroadcaster{}
	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		hub:        fb,
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

	// Both owners must be registered as their OWN battle_participants row —
	// the structural equivalent of "not silently merged into defenders[0]".
	var battleID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM battles WHERE world_id = $1 AND q = 1 AND r = 0`, worldID,
	).Scan(&battleID); err != nil {
		t.Fatalf("read battle: %v", err)
	}
	var registeredOwners []uuid.UUID
	rows, err := pool.Query(ctx,
		`SELECT owner_id FROM battle_participants WHERE battle_id = $1 AND side = 'defender'`, battleID,
	)
	if err != nil {
		t.Fatalf("query defender participants: %v", err)
	}
	for rows.Next() {
		var owner uuid.UUID
		if scanErr := rows.Scan(&owner); scanErr == nil {
			registeredOwners = append(registeredOwners, owner)
		}
	}
	rows.Close()
	seen := map[uuid.UUID]bool{}
	for _, o := range registeredOwners {
		seen[o] = true
	}
	if !seen[defenderA] || !seen[defenderB] {
		t.Fatalf("defender battle_participants owners = %v, want both %s and %s registered", registeredOwners, defenderA, defenderB)
	}

	// Drive the battle to its conclusion — with 100x combined strength, the
	// attacker must annihilate BOTH defenders, not just the first one loaded.
	battleH := NewBattleTickHandler(pool, h.eventStore, h.scheduler)
	runBattleToEnd(t, pool, battleH, worldID, battleID, 20)

	var defenderAStatus, defenderBStatus string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM units WHERE id = $1`, defenderAUnitID,
	).Scan(&defenderAStatus); err != nil {
		t.Fatalf("read defender-a unit: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT status FROM units WHERE id = $1`, defenderBUnitID,
	).Scan(&defenderBStatus); err != nil {
		t.Fatalf("read defender-b unit: %v", err)
	}
	if defenderAStatus != "disbanded" || defenderBStatus != "disbanded" {
		t.Fatalf("defender statuses = (%q, %q), want both disbanded (overwhelming attacker annihilates both, not just defenders[0])", defenderAStatus, defenderBStatus)
	}
}
