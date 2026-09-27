package handlers

// R2/R3 (megaron_plan_ordna_passage.md, 3b-3): a ship on a "passage"/
// "passage_wait" mission names the runner's destination in its unit-list
// row, and says plainly once it is actually holding for that runner's
// return leg.

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
)

func TestAttachPassageNotes_NamesDestinationAndWaitingState(t *testing.T) {
	pool := recruitShipTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-pn-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	var owner uuid.UUID
	_ = pool.QueryRow(ctx, `INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"pn-"+uuid.New().String()).Scan(&owner)

	mkSettlement := func(name string, q int) uuid.UUID {
		var prov, id uuid.UUID
		_ = pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
			worldID, q).Scan(&prov)
		_ = pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', $5, 'active', 5000) RETURNING id`,
			worldID, prov, name, owner, q == 0).Scan(&id)
		return id
	}
	home := mkSettlement("Home", 0)
	foreign := mkSettlement("Faraway", 5)

	mkShip := func() uuid.UUID {
		var id uuid.UUID
		_ = pool.QueryRow(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
			 VALUES ($1, $2, 'merchantman', 'naval', 1, 10, 'garrison', $3) RETURNING id`,
			worldID, owner, home).Scan(&id)
		return id
	}
	mkRunner := func(destID uuid.UUID) uuid.UUID {
		var id uuid.UUID
		_ = pool.QueryRow(ctx,
			`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, kind, hex_q, hex_r, arrives_at)
			 VALUES ($1,$2,$3,$4,'hello','outbound','diplomatic',0,0,now()) RETURNING id`,
			worldID, owner, home, destID).Scan(&id)
		return id
	}

	sailingIntent := "passage"
	waitingIntent := "passage_wait"

	sailingShip := mkShip()
	sailingRunner := mkRunner(foreign)
	if _, err := pool.Exec(ctx, `UPDATE units SET passage_messenger_id = $1 WHERE id = $2`, sailingRunner, sailingShip); err != nil {
		t.Fatalf("link sailing ship to its runner: %v", err)
	}

	waitingShip := mkShip()
	waitingRunner := mkRunner(foreign)
	if _, err := pool.Exec(ctx, `UPDATE units SET passage_messenger_id = $1 WHERE id = $2`, waitingRunner, waitingShip); err != nil {
		t.Fatalf("link waiting ship to its runner: %v", err)
	}

	units := []*unit.Unit{
		{ID: sailingShip, MarchIntent: &sailingIntent},
		{ID: waitingShip, MarchIntent: &waitingIntent},
	}
	summaries := []unitSummary{
		{ID: sailingShip, Status: "marching"},
		{ID: waitingShip, Status: "positioned"},
	}
	attachPassageNotes(ctx, pool, worldID, units, summaries)

	if summaries[0].PassageFor == nil || *summaries[0].PassageFor != "Faraway" {
		t.Errorf("sailing ship PassageFor = %v, want \"Faraway\"", summaries[0].PassageFor)
	}
	if summaries[0].WaitingForReturn {
		t.Errorf("sailing ship WaitingForReturn = true, want false — still outbound")
	}
	if summaries[1].PassageFor == nil || *summaries[1].PassageFor != "Faraway" {
		t.Errorf("waiting ship PassageFor = %v, want \"Faraway\"", summaries[1].PassageFor)
	}
	if !summaries[1].WaitingForReturn {
		t.Errorf("waiting ship WaitingForReturn = false, want true")
	}
}
