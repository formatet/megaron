package combat

import (
	"context"
	"math"
	"testing"
)

// The real upkeep handler books its debits under the army categories: what leaves
// the store as rations and wages is exactly what the ledger says, never "Other".
func TestUpkeep_BooksArmyRationAndWagesInLedger(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "ledger-army")
	seedGoods(t, pool, f.townID, f.tick, 100000, 100000)
	for _, u := range []struct {
		typ, cat string
		size     int
	}{{"spearman", "land", 100}, {"war_chariot", "land", 20}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, support_settlement_id)
			 VALUES ($1, $2, $3, $4, $5, 0, 'positioned', 9, 0, $6)`,
			f.worldID, f.owner, u.typ, u.cat, u.size, f.townID); err != nil {
			t.Fatalf("create %s: %v", u.typ, err)
		}
	}
	grainBefore, silverBefore := settledAmount(t, pool, f.townID, "grain"), settledAmount(t, pool, f.townID, "silver")
	runUpkeep(t, pool, f)
	grainDrop := grainBefore - settledAmount(t, pool, f.townID, "grain")
	silverDrop := silverBefore - settledAmount(t, pool, f.townID, "silver")
	if grainDrop <= 0 || silverDrop <= 0 {
		t.Fatalf("fixture exercises no debit: grain %.2f silver %.2f", grainDrop, silverDrop)
	}
	booked := func(good, cat string) float64 {
		var v float64
		if err := pool.QueryRow(ctx, `SELECT COALESCE(sum(amount),0) FROM settlement_ledger WHERE settlement_id=$1 AND good_key=$2 AND category=$3`,
			f.townID, good, cat).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if got := booked("grain", "army_ration"); math.Abs(got+grainDrop) > 1e-6 {
		t.Errorf("army_ration booked %.4f, store fell %.4f", got, grainDrop)
	}
	if got := booked("silver", "army_wages"); math.Abs(got+silverDrop) > 1e-6 {
		t.Errorf("army_wages booked %.4f, store fell %.4f", got, silverDrop)
	}
}
