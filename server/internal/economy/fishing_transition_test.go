package economy

import (
	"context"
	"formatet/megaron/server/internal/hexgrid"
	"math"
	"testing"
)

func TestFishingTransitionPrunesHighestAndRecomputesOnce(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	sid := seedFullRingFixture(t, 100, 1000, "coastal_sea")
	c := hexgrid.Coord{Q: 1, R: 0}
	for i := 1; i <= 6; i++ {
		placeHexGubbe(t, pool, sid, i, c, "fish")
	}
	if _, err := pool.Exec(ctx, `UPDATE fishing_rule_transition SET completed=false;
 `); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'fish',10,2,1000000,99)`, sid); err != nil {
		t.Fatal(err)
	}
	if err := CompleteFishingRuleTransition(ctx, pool); err != nil {
		t.Fatal(err)
	}
	assertState := func() {
		t.Helper()
		var ords []int
		rows, err := pool.Query(ctx, `SELECT gubbe_ordinal FROM settlement_placement WHERE settlement_id=$1 ORDER BY gubbe_ordinal`, sid)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var n int
			if err := rows.Scan(&n); err != nil {
				t.Fatal(err)
			}
			ords = append(ords, n)
		}
		rows.Close()
		if len(ords) != 2 || ords[0] != 1 || ords[1] != 2 {
			t.Fatalf("kept ordinals=%v want1,2", ords)
		}
		var amount, rate float64
		var population int
		if err := pool.QueryRow(ctx, `SELECT amount,rate FROM settlement_goods WHERE settlement_id=$1 AND good_key='fish'`, sid).Scan(&amount, &rate); err != nil {
			t.Fatal(err)
		}
		if math.Abs(amount-12) > 1e-9 || rate != 2 {
			t.Fatalf("food amount/rate=%v/%v want12/2", amount, rate)
		}
		if err := pool.QueryRow(ctx, `SELECT population FROM settlements WHERE id=$1`, sid).Scan(&population); err != nil {
			t.Fatal(err)
		}
		if population != 1000 {
			t.Fatalf("population=%d", population)
		}
	}
	assertState()
	if err := CompleteFishingRuleTransition(ctx, pool); err != nil {
		t.Fatal(err)
	}
	assertState()
}

func TestFishingTransitionRollsBackPruningOnRecomputeFailure(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	sid := seedFullRingFixture(t, 100, 1000, "coastal_sea")
	for i := 1; i <= 3; i++ {
		placeHexGubbe(t, pool, sid, i, hexgrid.Coord{Q: 1, R: 0}, "fish")
	}
	if _, err := pool.Exec(ctx, `UPDATE fishing_rule_transition SET completed=false;
 CREATE FUNCTION fishing_test_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fishing test recompute failure'; END $$;
 CREATE TRIGGER fishing_test_failure BEFORE INSERT OR UPDATE ON settlement_goods FOR EACH ROW WHEN(NEW.good_key='fish') EXECUTE FUNCTION fishing_test_failure();`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DROP TRIGGER fishing_test_failure ON settlement_goods;DROP FUNCTION fishing_test_failure();`)
	})
	if err := CompleteFishingRuleTransition(ctx, pool); err == nil {
		t.Fatal("expected recompute failure")
	}
	var count int
	var done bool
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM settlement_placement WHERE settlement_id=$1`, sid).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT completed FROM fishing_rule_transition`).Scan(&done); err != nil {
		t.Fatal(err)
	}
	if count != 3 || done {
		t.Fatalf("rollback count/done=%d/%v want3/false", count, done)
	}
}
