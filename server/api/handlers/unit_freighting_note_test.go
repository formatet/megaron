package handlers

// R2/R4 (megaron_plan_sjohandel_kraver_skepp.md): a status='freighting' ship's
// unit-list row must say what it's doing — a single transfer's destination,
// or the standing route it's locked to — not just the bare status.

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestAttachFreightingNotes_PrefersInFlightLegOverStandingRoute(t *testing.T) {
	pool := recruitShipTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-fn-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	var owner uuid.UUID
	_ = pool.QueryRow(ctx, `INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"fn-"+uuid.New().String()).Scan(&owner)

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
	dest := mkSettlement("Faraway", 5)
	routeTo := mkSettlement("Colony", 3)

	mkShip := func() uuid.UUID {
		var id uuid.UUID
		_ = pool.QueryRow(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
			 VALUES ($1, $2, 'merchantman', 'naval', 1, 10, 'freighting', $3) RETURNING id`,
			worldID, owner, home).Scan(&id)
		return id
	}

	// Ship A: currently mid-transfer to "Faraway".
	shipA := mkShip()
	if _, err := pool.Exec(ctx,
		`INSERT INTO transports
		   (world_id, owner_id, kind, origin_id, dest_id, category,
		    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, status, interceptable, ship_unit_id)
		 VALUES ($1,$2,'transfer',$3,$4,'naval',0,0,5,0, now(), now() + interval '1 hour', 1, 'in_transit', true, $5)`,
		worldID, owner, home, dest, shipA,
	); err != nil {
		t.Fatalf("create in-flight transport: %v", err)
	}

	// Ship B: idle in port, but locked to a standing route to "Colony".
	shipB := mkShip()
	if _, err := pool.Exec(ctx,
		`INSERT INTO standing_orders (world_id, owner_id, from_settlement_id, to_settlement_id, crewed_by_settlement_id, ship_unit_id)
		 VALUES ($1, $2, $3, $4, $3, $5)`,
		worldID, owner, home, routeTo, shipB,
	); err != nil {
		t.Fatalf("create standing order: %v", err)
	}

	summaries := []unitSummary{
		{ID: shipA, Status: "freighting"},
		{ID: shipB, Status: "freighting"},
	}
	attachFreightingNotes(ctx, pool, worldID, owner, summaries)

	if summaries[0].FreightingNote == nil || *summaries[0].FreightingNote != "carrying goods to Faraway" {
		t.Errorf("ship A note = %v, want \"carrying goods to Faraway\"", summaries[0].FreightingNote)
	}
	if summaries[1].FreightingNote == nil || *summaries[1].FreightingNote != "on standing route Home → Colony" {
		t.Errorf("ship B note = %v, want \"on standing route Home → Colony\"", summaries[1].FreightingNote)
	}
}
