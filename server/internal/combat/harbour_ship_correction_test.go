package combat

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
)

func TestHarbourShipCorrectionMovesShipAndCohortsIntoPort(t *testing.T) {
	f := newExpeditionFixture(t, -2, 8, nil)
	ctx := context.Background()
	pool := testPool(t)
	var cityQ, cityR int
	if err := pool.QueryRow(ctx, `SELECT p.map_q,p.map_r FROM settlements s JOIN provinces p ON p.id=s.province_id WHERE s.id=$1`, f.capitalID).Scan(&cityQ, &cityR); err != nil {
		t.Fatal(err)
	}
	var ship, cohort, land uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,q,r) VALUES($1,$2,'galley','naval',1,20,'garrison',$3,$4,$5) RETURNING id`, f.worldID, f.ownerID, f.capitalID, cityQ+1, cityR).Scan(&ship); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,settlement_id,q,r,carried_by_ship_id) VALUES($1,$2,'spearman','land',100,'embarked',$3,$4,$5,$6) RETURNING id`, f.worldID, f.ownerID, f.capitalID, cityQ+1, cityR, ship).Scan(&cohort); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,q,r) VALUES($1,$2,'spearman','land',100,'positioned',$3,$4) RETURNING id`, f.worldID, f.ownerID, cityQ+1, cityR).Scan(&land); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("../../db/migrations/220_ships_in_harbour_on_city_hex.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []uuid.UUID{ship, cohort} {
		var q, r int
		if err := pool.QueryRow(ctx, `SELECT q,r FROM units WHERE id=$1`, id).Scan(&q, &r); err != nil {
			t.Fatal(err)
		}
		if q != cityQ || r != cityR {
			t.Fatalf("unit %s at (%d,%d), want port city hex (%d,%d)", id, q, r, cityQ, cityR)
		}
	}
	var q int
	if err := pool.QueryRow(ctx, `SELECT q FROM units WHERE id=$1`, land).Scan(&q); err != nil {
		t.Fatal(err)
	}
	if q != cityQ+1 {
		t.Fatalf("moved a unit that is not a harbour ship")
	}
	var outcomes int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='ShipHarbourPositionCorrectedV1'`, ship).Scan(&outcomes); err != nil {
		t.Fatal(err)
	}
	if outcomes != 1 {
		t.Fatalf("correction outcomes %d, want 1 after rerun", outcomes)
	}
}
