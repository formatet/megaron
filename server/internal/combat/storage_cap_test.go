package combat

import (
	"context"
	"testing"
)

func TestDefaultGoodStorageCapColony(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	wid, mother, uid := purseFixture(t, pool, ctx, 3000, 700)
	runColonizeArrival(t, pool, ctx, wid, uid)
	var cap float64
	if err := pool.QueryRow(ctx, `SELECT sg.cap FROM settlement_goods sg JOIN settlements s ON s.id=sg.settlement_id WHERE s.world_id=$1 AND s.id<>$2 AND sg.good_key='cedar'`, wid, mother).Scan(&cap); err != nil {
		t.Fatal(err)
	}
	if cap != 1_000_000 {
		t.Fatalf("colony cedar cap=%g want unchanged 1000000", cap)
	}
}
