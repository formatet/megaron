package combat

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestSilverTriggerWhenSkipsOtherGoodsBeforeFunction(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	world, sid, _ := purseFixture(t, pool, ctx, 10, 0)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	must := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	// Trace the actual callback BEFORE its early-return guard. This is confined
	// to one rolled-back test transaction; production function remains unchanged.
	must(`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT $1,key,10,2,100,0 FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10,rate=2,cap=100,calc_tick=0`, sid)
	must(`DELETE FROM silver_mining_totals WHERE world_id=$1`, world)
	must(`CREATE TEMP TABLE silver_trigger_probe(good_key text) ON COMMIT DROP`)
	var definition string
	if err := tx.QueryRow(ctx, `SELECT pg_get_functiondef('record_silver_mining_credit()'::regprocedure)`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(definition, "\nBEGIN\n") {
		t.Fatal("probe insertion point missing")
	}
	must(strings.Replace(definition, "\nBEGIN\n", "\nBEGIN\n INSERT INTO pg_temp.silver_trigger_probe VALUES(NEW.good_key);\n", 1))
	must(`UPDATE settlement_goods SET amount=11,rate=3,cap=110,calc_tick=3 WHERE settlement_id=$1`, sid)
	var silver, other int
	if err := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE good_key='silver'),count(*) FILTER(WHERE good_key<>'silver') FROM silver_trigger_probe`).Scan(&silver, &other); err != nil {
		t.Fatal(err)
	}
	if silver != 1 || other != 0 {
		t.Errorf("actual trigger calls silver=%d other=%d want1/0", silver, other)
	}
	var credited float64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(credited_silver),0) FROM silver_mining_totals WHERE world_id=$1`, world).Scan(&credited); err != nil {
		t.Fatal(err)
	}
	if credited != 6 {
		t.Errorf("silver realized%g want6", credited)
	}
	// A same-tick silver write still invokes the unchanged function; its guard
	// prevents new production. A non-silver UPSERT UPDATE skips it altogether.
	must(`UPDATE settlement_goods SET amount=amount WHERE settlement_id=$1`, sid)
	must(`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',8,0,100,3) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=EXCLUDED.amount`, sid)
	if err := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE good_key='silver'),count(*) FILTER(WHERE good_key<>'silver') FROM silver_trigger_probe`).Scan(&silver, &other); err != nil {
		t.Fatal(err)
	}
	if silver != 2 || other != 0 {
		t.Errorf("repeat/UPSERT trigger calls silver=%d other=%d want2/0", silver, other)
	}
	if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(credited_silver),0) FROM silver_mining_totals WHERE world_id=$1`, world).Scan(&credited); err != nil {
		t.Fatal(err)
	}
	if credited != 6 {
		t.Errorf("same-tick write credited twice: %g", credited)
	}
}

func TestSilverTrigger203RoundTripPreservesMiningOutcomes(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	world, sid, _ := purseFixture(t, pool, ctx, 10, 0)
	up, err := os.ReadFile("../../db/migrations/203_silver_trigger_when.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile("../../db/migrations/203_silver_trigger_when.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	must := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO silver_mining_totals(settlement_id,world_id,credited_silver) VALUES($1,$2,12.5) ON CONFLICT(settlement_id) DO UPDATE SET credited_silver=12.5`, sid, world)
	must(`INSERT INTO silver_audit_epochs(world_id) VALUES($1) ON CONFLICT DO NOTHING`, world)
	var fn, epoch string
	if err := tx.QueryRow(ctx, `SELECT pg_get_functiondef('record_silver_mining_credit()'::regprocedure),(SELECT instrumentation_epoch::text FROM silver_audit_epochs WHERE world_id=$1)`, world).Scan(&fn, &epoch); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []struct {
		name     string
		sql      []byte
		filtered bool
	}{{"down", down, false}, {"up", up, true}, {"down_again", down, false}, {"up_again", up, true}} {
		must(string(phase.sql))
		var gotfn, gotepoch string
		var total float64
		var filtered bool
		if err := tx.QueryRow(ctx, `SELECT pg_get_functiondef('record_silver_mining_credit()'::regprocedure),(SELECT instrumentation_epoch::text FROM silver_audit_epochs WHERE world_id=$1),(SELECT credited_silver FROM silver_mining_totals WHERE settlement_id=$2),(SELECT tgqual IS NOT NULL FROM pg_trigger WHERE tgrelid='settlement_goods'::regclass AND tgname='silver_mining_credit')`, world, sid).Scan(&gotfn, &gotepoch, &total, &filtered); err != nil {
			t.Fatal(err)
		}
		if gotfn != fn || gotepoch != epoch || total != 12.5 || filtered != phase.filtered {
			t.Fatalf("%s migration changed function/epoch/counter or wrong filter: %v %v %g %v", phase.name, gotfn == fn, gotepoch == epoch, total, filtered)
		}
		// Actual silver production remains independent of the trigger's dispatch filter.
		for _, tc := range []struct {
			name                             string
			amount, rate, cap                float64
			oldTick, newTick                 int
			newAmount, newRate, newCap, want float64
		}{
			{"production", 10, 2, 100, 0, 3, 16, 2, 100, 6},
			{"clipped", 98, 2, 100, 0, 3, 100, 2, 100, 2},
			{"full", 100, 2, 100, 0, 3, 100, 2, 100, 0},
			{"above_cap", 120, 2, 100, 0, 3, 100, 2, 100, 0},
			{"zero_rate", 10, 0, 100, 0, 3, 10, 0, 100, 0},
			{"negative_rate", 10, -2, 100, 0, 3, 4, -2, 100, 0},
			{"same_tick", 10, 2, 100, 3, 3, 20, 2, 100, 0},
			{"backwards_tick", 10, 2, 100, 5, 3, 8, 2, 100, 0},
			{"payment_not_mining", 10, 2, 100, 0, 3, 1, 2, 100, 6},
			{"old_rate", 10, 2, 100, 0, 3, 16, 10, 100, 6},
			{"old_cap", 98, 2, 100, 0, 3, 0, 2, 200, 2},
		} {
			must(`SAVEPOINT mining_case`)
			// Disable accounting only while resetting an artificial test starting tuple.
			must(`ALTER TABLE settlement_goods DISABLE TRIGGER silver_mining_credit`)
			must(`UPDATE settlement_goods SET amount=$2,rate=$3,cap=$4,calc_tick=$5 WHERE settlement_id=$1 AND good_key='silver'`, sid, tc.amount, tc.rate, tc.cap, tc.oldTick)
			must(`ALTER TABLE settlement_goods ENABLE TRIGGER silver_mining_credit`)
			must(`UPDATE settlement_goods SET amount=$2,rate=$3,cap=$4,calc_tick=$5 WHERE settlement_id=$1 AND good_key='silver'`, sid, tc.newAmount, tc.newRate, tc.newCap, tc.newTick)
			var got float64
			if err := tx.QueryRow(ctx, `SELECT credited_silver-12.5 FROM silver_mining_totals WHERE settlement_id=$1`, sid).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("%s/%s mining%g want%g", phase.name, tc.name, got, tc.want)
			}
			must(`ROLLBACK TO SAVEPOINT mining_case`)
			must(`RELEASE SAVEPOINT mining_case`)
		}
	}
}
