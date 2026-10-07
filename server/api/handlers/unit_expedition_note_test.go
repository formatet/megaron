package handlers

// Upptäckarexpeditionen (megaron_plan_upptackarexpeditionen.md): a unit on
// an area expedition carries its mission line in the unit list — and a unit
// whose expedition a recall/redirect ended (arrive_tick moved on) does not.

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestAttachExpeditionNotes_LiveOnly(t *testing.T) {
	pool := recruitShipTestPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover worlds: %v", err)
	}
	var worldID, owner uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-exp-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })
	if err := pool.QueryRow(ctx, `INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"exp-"+uuid.New().String()).Scan(&owner); err != nil {
		t.Fatalf("create player: %v", err)
	}

	mkExpedition := func(unitArrive, legArrive int) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r, target_q, target_r, arrive_tick, march_intent)
			 VALUES ($1, $2, 'infantry', 'land', 100, 'marching', 0, 0, 3, 0, $3, 'explore') RETURNING id`,
			worldID, owner, unitArrive).Scan(&id); err != nil {
			t.Fatalf("create unit: %v", err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO unit_expeditions (unit_id, world_id, area_q, area_r, length_ticks, start_tick, turn_tick, leg_arrive_tick)
			 VALUES ($1, $2, 9, 0, 12, 2, 8, $3)`, id, worldID, legArrive); err != nil {
			t.Fatalf("create expedition: %v", err)
		}
		return id
	}
	live := mkExpedition(5, 5)
	ended := mkExpedition(6, 5) // a redirect moved arrive_tick on

	summaries := []unitSummary{{ID: live}, {ID: ended}}
	attachExpeditionNotes(ctx, pool, worldID, summaries)

	e := summaries[0].Expedition
	if e == nil {
		t.Fatal("live expedition: Expedition = nil")
	}
	if e.AreaQ != 9 || e.AreaR != 0 || e.LengthTicks != 12 || e.TurnTick != 8 || e.HomeByTick != 14 || e.Homeward {
		t.Errorf("live expedition = %+v, want area (9,0), length 12, turn 8, home by 14, outbound", *e)
	}
	if summaries[1].Expedition != nil {
		t.Errorf("ended expedition still shown: %+v", *summaries[1].Expedition)
	}
}

func TestAttachExpeditionNotes_ReturnUsesActualArrival(t *testing.T) {
	pool := recruitShipTestPool(t)
	ctx := context.Background()
	var worldID, owner, id uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status) VALUES($1,'archived') RETURNING id`, "test-return-"+uuid.New().String()).Scan(&worldID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "return-"+uuid.New().String()).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,target_q,target_r,arrive_tick,march_intent) VALUES($1,$2,'infantry','land',100,'marching',1,0,0,0,20,'explore_return') RETURNING id`, worldID, owner).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO unit_expeditions(unit_id,world_id,area_q,area_r,length_ticks,start_tick,turn_tick,leg_arrive_tick,homeward,turn_reason) VALUES($1,$2,4,0,10,2,7,20,true,'half_time')`, id, worldID); err != nil {
		t.Fatal(err)
	}
	summaries := []unitSummary{{ID: id}}
	attachExpeditionNotes(ctx, pool, worldID, summaries)
	if e := summaries[0].Expedition; e == nil || !e.Homeward || e.HomeByTick != 20 {
		t.Fatalf("return mission promises stale duration instead of actual home arrival: %+v", e)
	}
}
