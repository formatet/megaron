package transport

import (
	"context"
	"testing"
)

func TestDefaultGoodStorageCapArrival(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	ctx := context.Background()
	dispatchGift(t, pool, f, Manifest{"cedar": 5})
	fireArrival(t, pool, f.worldID, nil)
	var cap, amount float64
	if err := pool.QueryRow(ctx, `SELECT cap,amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='cedar'`, f.destID).Scan(&cap, &amount); err != nil {
		t.Fatal(err)
	}
	if cap != 1_000_000 || amount != 5 {
		t.Fatalf("new arrival row cap=%g amount=%g", cap, amount)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET cap=7 WHERE settlement_id=$1 AND good_key='cedar'`, f.destID); err != nil {
		t.Fatal(err)
	}
	dispatchGift(t, pool, f, Manifest{"cedar": 5})
	fireArrival(t, pool, f.worldID, nil)
	if err := pool.QueryRow(ctx, `SELECT cap,amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='cedar'`, f.destID).Scan(&cap, &amount); err != nil {
		t.Fatal(err)
	}
	if cap != 7 || amount != 7 {
		t.Fatalf("existing arrival row cap=%g amount=%g want 7/7", cap, amount)
	}
}

func TestDefaultGoodStorageCapLoot(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	ctx := context.Background()
	id := dispatchGift(t, pool, f, Manifest{"cedar": 5})
	credit := func() {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err := creditLootToCapital(ctx, tx, f.worldID, f.owner, id); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	credit()
	var cap, amount float64
	if err := pool.QueryRow(ctx, `SELECT cap,amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='cedar'`, f.sourceID).Scan(&cap, &amount); err != nil {
		t.Fatal(err)
	}
	if cap != 1_000_000 || amount != 5 {
		t.Fatalf("new loot row cap=%g amount=%g", cap, amount)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET cap=7 WHERE settlement_id=$1 AND good_key='cedar'`, f.sourceID); err != nil {
		t.Fatal(err)
	}
	credit()
	if err := pool.QueryRow(ctx, `SELECT cap,amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='cedar'`, f.sourceID).Scan(&cap, &amount); err != nil {
		t.Fatal(err)
	}
	if cap != 7 || amount != 7 {
		t.Fatalf("existing loot row cap=%g amount=%g want 7/7", cap, amount)
	}
}
