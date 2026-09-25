package handlers

// in_battle on the units list: true exactly while the unit is an active
// participant of an active battle — the condition SetStandingOrders checks —
// so the web shows the per-unit retreat control only when it can take.

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestAttachBattleFlags_OnlyActiveParticipants(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`, "test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	var owner uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`, "ib-"+uuid.New().String(),
	).Scan(&owner); err != nil {
		t.Fatalf("create player: %v", err)
	}
	mkUnit := func() uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
			 VALUES ($1, $2, 'spearman', 'land', 50, 0, 'positioned', 1, 0) RETURNING id`, worldID, owner,
		).Scan(&id); err != nil {
			t.Fatalf("create unit: %v", err)
		}
		return id
	}
	fighting, routed, idle, inEndedBattle := mkUnit(), mkUnit(), mkUnit(), mkUnit()

	mkBattle := func(status string) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO battles (world_id, q, r, started_tick, current_tick, status, seed)
			 VALUES ($1, 1, 0, 0, 0, $2, 1) RETURNING id`, worldID, status,
		).Scan(&id); err != nil {
			t.Fatalf("create %s battle: %v", status, err)
		}
		return id
	}
	active, ended := mkBattle("active"), mkBattle("ended")
	join := func(battleID, unitID uuid.UUID, left *int) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO battle_participants (battle_id, unit_id, owner_id, side, joined_tick, initial_size, current_size, left_tick)
			 VALUES ($1, $2, $3, 'defender', 0, 50, 50, $4)`, battleID, unitID, owner, left,
		); err != nil {
			t.Fatalf("join battle: %v", err)
		}
	}
	one := 1
	join(active, fighting, nil)
	join(active, routed, &one)
	join(ended, inEndedBattle, nil)

	summaries := []unitSummary{{ID: fighting}, {ID: routed}, {ID: idle}, {ID: inEndedBattle}}
	attachBattleFlags(ctx, pool, worldID, owner, summaries)
	want := []bool{true, false, false, false}
	for i, s := range summaries {
		if s.InBattle != want[i] {
			t.Errorf("summary %d InBattle = %v, want %v", i, s.InBattle, want[i])
		}
	}
}
