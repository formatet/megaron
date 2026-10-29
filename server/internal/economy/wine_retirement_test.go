package economy

import (
	"context"
	"github.com/google/uuid"
	"os"
	"testing"
)

func TestWineProvisionsRetirement(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFoodFixture(t, pool, "wine-retirement", 100)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var owner uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, f.settlementID).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"garrison", "positioned", "disbanded"} {
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,settlement_id,q,r,provisions,provision_goods) VALUES($1,$2,'galley','naval',1,$3,$4,0,0,5,'{"grain":2,"fish":1,"oil":1,"wine":1}') RETURNING id`, f.worldID, owner, status, f.settlementID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := StoredProvisions(ctx, tx, id); err == nil {
			t.Fatal("legacy wine must fail strict ledger")
		}
	}
	raw, err := os.ReadFile("../../db/migrations/226_retire_wine_provisions.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := tx.Exec(ctx, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM units WHERE world_id=$1 AND (provision_goods ? 'wine' OR provisions<>4)`, f.worldID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("bad ledgers: %d", n)
	}
	var wine float64
	if err := tx.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='wine'`, f.settlementID).Scan(&wine); err != nil {
		t.Fatal(err)
	}
	if wine != 1 {
		t.Fatalf("returned wine=%g want 1, only physical port and exactly once", wine)
	}
	var ship uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM units WHERE world_id=$1 AND status='garrison'`, f.worldID).Scan(&ship); err != nil {
		t.Fatal(err)
	}
	if _, err := StoredProvisions(ctx, tx, ship); err != nil {
		t.Fatal(err)
	}
	if err := ReturnProvisions(ctx, tx, ship, f.settlementID); err != nil {
		t.Fatal(err)
	}
}
