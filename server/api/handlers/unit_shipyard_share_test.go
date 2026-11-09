package handlers

// The yard's sharing shows on the unit list: two ships forming in one city with 3 workers
// both carry workers=3 sharing=2; a lone ship sharing 1; a garrison unit nothing.

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

func TestAttachShipyardSharing(t *testing.T) {
	pool := recruitShipTestPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatal(err)
	}
	var worldID, owner, prov, city uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`, "test-ys-"+uuid.New().String()).Scan(&worldID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })
	_ = pool.QueryRow(ctx, `INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`, "ys-"+uuid.New().String()).Scan(&owner)
	_ = pool.QueryRow(ctx, `INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`, worldID).Scan(&prov)
	if err := pool.QueryRow(ctx, `INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		VALUES ($1, $2, 'Yard', 'achaean', $3, 'capital', true, 'active', 5000) RETURNING id`, worldID, prov, owner).Scan(&city); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO buildings (settlement_id, building_type, level, hex_q, hex_r) VALUES ($1, 'shipyard', 3, NULL, NULL)`, city); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		if _, err := pool.Exec(ctx, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES($1,$2,'building','shipyard','shipbuilding')`, city, i); err != nil {
			t.Fatal(err)
		}
	}
	mk := func(status string, work bool) uuid.UUID {
		var id uuid.UUID
		q := `INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id%s) VALUES ($1,$2,'galley','naval',1,10,$3,$4%s) RETURNING id`
		if work {
			q = sprintfQ(q, `, shipyard_work_remaining`, `, 600`)
		} else {
			q = sprintfQ(q, ``, ``)
		}
		if err := pool.QueryRow(ctx, q, worldID, owner, status, city).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	a, b, g := mk("forming", true), mk("repairing", true), mk("garrison", false)
	cityID := city
	list := func(ids ...uuid.UUID) []unitSummary {
		var out []unitSummary
		for _, id := range ids {
			s := unitSummary{ID: id, SettlementID: &cityID}
			if id != g {
				w := 600
				s.ShipyardWorkRemaining = &w
			}
			out = append(out, s)
		}
		return out
	}
	both := list(a, b, g)
	attachShipyardSharing(ctx, pool, owner, both)
	for i := 0; i < 2; i++ {
		if both[i].ShipyardWorkers != 3 || both[i].ShipyardSharing != 2 {
			t.Fatalf("ship %d: workers=%d sharing=%d, want 3/2", i, both[i].ShipyardWorkers, both[i].ShipyardSharing)
		}
	}
	if both[2].ShipyardWorkers != 0 || both[2].ShipyardSharing != 0 {
		t.Fatalf("garrison unit carries yard fields: %+v", both[2])
	}
	if _, err := pool.Exec(ctx, `UPDATE units SET status='garrison', shipyard_work_remaining=NULL WHERE id=$1`, b); err != nil {
		t.Fatal(err)
	}
	alone := list(a)
	attachShipyardSharing(ctx, pool, owner, alone)
	if alone[0].ShipyardWorkers != 3 || alone[0].ShipyardSharing != 1 {
		t.Fatalf("lone ship: %d/%d, want 3/1", alone[0].ShipyardWorkers, alone[0].ShipyardSharing)
	}
}

func sprintfQ(q, a, b string) string { return fmt.Sprintf(q, a, b) }
