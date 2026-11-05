package economy

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"formatet/megaron/server/internal/ledger"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Storehouse (mig 244): one shared ceiling for every good but food, herd, horses, cult.

func shPut(t *testing.T, pool *pgxpool.Pool, f foodFixture, good string, amount, rate float64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,$2,$3,$4,1000000,$5)
		 ON CONFLICT (settlement_id,good_key) DO UPDATE SET amount=$3,rate=$4,calc_tick=$5`,
		f.settlementID, good, amount, rate, f.tick); err != nil {
		t.Fatal(err)
	}
}

func shTick(t *testing.T, pool *pgxpool.Pool, f foodFixture, tick int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE worlds SET current_tick=$2 WHERE id=$1`, f.worldID, tick); err != nil {
		t.Fatal(err)
	}
}

func shLevel(t *testing.T, pool *pgxpool.Pool, f foodFixture, level int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `DELETE FROM buildings WHERE settlement_id=$1 AND building_type='storehouse'`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if level > 0 {
		if _, err := pool.Exec(context.Background(), `INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'storehouse',$2)`, f.settlementID, level); err != nil {
			t.Fatal(err)
		}
	}
}

func shAmount(t *testing.T, pool *pgxpool.Pool, f foodFixture, good string) float64 {
	t.Helper()
	var a float64
	if err := pool.QueryRow(context.Background(), `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key=$2`, f.settlementID, good).Scan(&a); err != nil {
		t.Fatal(err)
	}
	return a
}

// shRealizeOneByOne is the recompute shape: one write per good, in the given order.
func shRealizeOneByOne(t *testing.T, pool *pgxpool.Pool, f foodFixture, goods ...string) {
	t.Helper()
	for _, g := range goods {
		if _, err := pool.Exec(context.Background(),
			`UPDATE settlement_goods SET amount=storage_capped(settlement_id,good_key,settled(amount,rate,calc_tick),cap,'production'),calc_tick=current_world_tick()
			 WHERE settlement_id=$1 AND good_key=$2`, f.settlementID, g); err != nil {
			t.Fatal(err)
		}
	}
}

var shEvent int64 = 9_100_000

// shCloseDay is the real daily close: ONE statement realizing every producing row.
func shCloseDay(t *testing.T, pool *pgxpool.Pool, f foodFixture) {
	t.Helper()
	shEvent++
	if err := ledger.CloseDay(context.Background(), pool, f.worldID, shEvent); err != nil {
		t.Fatal(err)
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// 10 places left, stone and timber at the same rate -> 5 + 5, whatever the write order,
// and the same through the daily close (one statement) as through one-by-one writes.
func TestStorehouse_LastRoomIsSplitByRate(t *testing.T) {
	pool := testPool(t)
	for name, realize := range map[string]func(*testing.T, *pgxpool.Pool, foodFixture){
		"stone first":  func(t *testing.T, p *pgxpool.Pool, f foodFixture) { shRealizeOneByOne(t, p, f, "stone", "timber") },
		"timber first": func(t *testing.T, p *pgxpool.Pool, f foodFixture) { shRealizeOneByOne(t, p, f, "timber", "stone") },
		"daily close":  shCloseDay,
	} {
		t.Run(name, func(t *testing.T) {
			f := newFoodFixture(t, pool, "sh-prop", 100)
			shPut(t, pool, f, "stone", 245, 10)
			shPut(t, pool, f, "timber", 245, 10)
			shTick(t, pool, f, f.tick+10)
			realize(t, pool, f)
			if s, ti := shAmount(t, pool, f, "stone"), shAmount(t, pool, f, "timber"); !near(s, 250) || !near(ti, 250) {
				t.Fatalf("10 places, equal rate: stone %.4f timber %.4f, want 250 + 250", s, ti)
			}
		})
	}
	t.Run("unequal rates 3:1", func(t *testing.T) {
		f := newFoodFixture(t, pool, "sh-prop31", 100)
		shPut(t, pool, f, "stone", 240, 30)
		shPut(t, pool, f, "timber", 240, 10)
		shTick(t, pool, f, f.tick+10)
		shRealizeOneByOne(t, pool, f, "timber", "stone")
		if s, ti := shAmount(t, pool, f, "stone"), shAmount(t, pool, f, "timber"); !near(s, 255) || !near(ti, 245) {
			t.Fatalf("20 places 3:1: stone %.4f timber %.4f, want 255 + 245", s, ti)
		}
	})
}

// Under the ceiling nothing is clipped; at the ceiling everything stops together.
func TestStorehouse_FillsThenStopsAll(t *testing.T) {
	pool := testPool(t)
	f := newFoodFixture(t, pool, "sh-stop", 100)
	shPut(t, pool, f, "stone", 0, 20)
	shPut(t, pool, f, "timber", 0, 10)
	shTick(t, pool, f, f.tick+5) // 100 + 50 accrued, room 500
	shCloseDay(t, pool, f)
	if s, ti := shAmount(t, pool, f, "stone"), shAmount(t, pool, f, "timber"); !near(s, 100) || !near(ti, 50) {
		t.Fatalf("below the ceiling: stone %.2f timber %.2f, want 100 / 50", s, ti)
	}
	shTick(t, pool, f, f.tick+100)
	shCloseDay(t, pool, f)
	s, ti := shAmount(t, pool, f, "stone"), shAmount(t, pool, f, "timber")
	if !near(s+ti, 500) {
		t.Fatalf("full store: %.2f + %.2f = %.2f, want 500", s, ti, s+ti)
	}
	shTick(t, pool, f, f.tick+200)
	shCloseDay(t, pool, f)
	if s2, t2 := shAmount(t, pool, f, "stone"), shAmount(t, pool, f, "timber"); !near(s2+t2, 500) {
		t.Fatalf("a full store kept growing: %.2f", s2+t2)
	}
}

// Level 1 raises the ceiling to 1000; existing overstock is never cut and only shrinks by use.
func TestStorehouse_LevelCeilingAndOverstock(t *testing.T) {
	pool := testPool(t)
	f := newFoodFixture(t, pool, "sh-level", 100)
	shPut(t, pool, f, "stone", 0, 100)
	shTick(t, pool, f, f.tick+50)
	shCloseDay(t, pool, f)
	if s := shAmount(t, pool, f, "stone"); !near(s, 500) {
		t.Fatalf("no building: %.2f, want 500", s)
	}
	shLevel(t, pool, f, 1)
	shTick(t, pool, f, f.tick+100)
	shCloseDay(t, pool, f)
	if s := shAmount(t, pool, f, "stone"); !near(s, 1000) {
		t.Fatalf("level 1: %.2f, want 1000", s)
	}
	for _, c := range [][2]int{{2, 2500}, {3, 5000}} {
		lvl, want := c[0], float64(c[1])
		shLevel(t, pool, f, lvl)
		shTick(t, pool, f, f.tick+100+lvl*100)
		shCloseDay(t, pool, f)
		if s := shAmount(t, pool, f, "stone"); !near(s, want) {
			t.Fatalf("level %d: %.2f, want %.0f", lvl, s, want)
		}
	}
	// Demolished: 5000 stays (never cut), nothing new is admitted.
	shLevel(t, pool, f, 0)
	shTick(t, pool, f, f.tick+1000)
	shCloseDay(t, pool, f)
	if s := shAmount(t, pool, f, "stone"); !near(s, 5000) {
		t.Fatalf("overstock after demolition: %.2f, want 5000 untouched", s)
	}
	// Spent down to 400: room is 100 again.
	if _, err := pool.Exec(context.Background(), `UPDATE settlement_goods SET amount=400,calc_tick=current_world_tick() WHERE settlement_id=$1 AND good_key='stone'`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	shTick(t, pool, f, f.tick+1100)
	shCloseDay(t, pool, f)
	if s := shAmount(t, pool, f, "stone"); !near(s, 500) {
		t.Fatalf("after use: %.2f, want 500", s)
	}
}

// Food, the herd, horses and cult are outside the store: they neither fill it nor are limited by it.
func TestStorehouse_ExemptGoodsAreNotCounted(t *testing.T) {
	pool := testPool(t)
	f := newFoodFixture(t, pool, "sh-exempt", 100)
	for _, g := range []string{"grain", "fish", "oil", "livestock", "horses"} {
		shPut(t, pool, f, g, 100000, 1000)
	}
	shPut(t, pool, f, "stone", 0, 100)
	shTick(t, pool, f, f.tick+10)
	shCloseDay(t, pool, f)
	if s := shAmount(t, pool, f, "stone"); !near(s, 500) {
		t.Fatalf("stone %.2f, want 500: exempt goods must not count against (or share) the store", s)
	}
	for _, g := range []string{"livestock", "horses"} {
		if a := shAmount(t, pool, f, g); a < 100000+9999 {
			t.Fatalf("%s limited by the Storehouse: %.2f", g, a)
		}
	}
	for _, g := range []string{"grain", "fish", "oil"} { // food keeps its own (silo) ceiling and ratchet
		if a := shAmount(t, pool, f, g); a < 100000 {
			t.Fatalf("%s cut: %.2f", g, a)
		}
	}
	// And silver and wine DO count.
	shPut(t, pool, f, "stone", 250, 0)
	shPut(t, pool, f, "silver", 150, 0)
	shPut(t, pool, f, "wine", 100, 50)
	shTick(t, pool, f, f.tick+20)
	shCloseDay(t, pool, f)
	if w := shAmount(t, pool, f, "wine"); !near(w, 100) {
		t.Fatalf("wine %.2f: stone+silver+wine fill 500, wine must not grow", w)
	}
}

// Deliveries: admitted into the room that is left, the rest lost with an outcome; parallel
// credits of different goods serialize on the city's lock instead of both seeing a free store.
func TestStorehouse_CreditsAdmitRoomAndSerialize(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFoodFixture(t, pool, "sh-credit", 100)
	var got float64
	if err := pool.QueryRow(ctx, `SELECT credit_settlement_good($1,'stone',800,'gift_delivery')`, f.settlementID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if !near(got, 500) {
		t.Fatalf("credit into an empty 500-store admitted %.2f", got)
	}
	var lost float64
	var pool_ string
	if err := pool.QueryRow(ctx, `SELECT (payload->>'lost_quantity')::float8, payload->>'pool' FROM events WHERE stream_id=$1 AND event_type='StorageOverflow'`, f.settlementID).Scan(&lost, &pool_); err != nil {
		t.Fatal(err)
	}
	if !near(lost, 300) || pool_ != "storehouse" {
		t.Fatalf("outcome lost %.2f pool %q, want 300 / storehouse", lost, pool_)
	}

	g := newFoodFixture(t, pool, "sh-parallel", 100)
	txA, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := txA.QueryRow(ctx, `SELECT credit_settlement_good($1,'stone',300,'gift_delivery')`, g.settlementID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	var gotB float64
	go func() {
		defer wg.Done()
		if err := pool.QueryRow(ctx, `SELECT credit_settlement_good($1,'timber',300,'gift_delivery')`, g.settlementID).Scan(&gotB); err != nil {
			t.Error(err)
		}
	}()
	time.Sleep(400 * time.Millisecond)
	if err := txA.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if s, ti := shAmount(t, pool, g, "stone"), shAmount(t, pool, g, "timber"); !near(s+ti, 500) || !near(s, 300) || !near(ti, 200) {
		t.Fatalf("parallel credits: stone %.2f timber %.2f, want 300 + 200", s, ti)
	}
}

// ONE dispatch when production starts to be lost, not one per good and not one per tick;
// a new one only after a quiet spell. Every lost quantity is still an event and, in the
// books, `overflow` - never Other.
func TestStorehouse_OneDispatchPerSpell(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFoodFixture(t, pool, "sh-dispatch", 100)
	shPut(t, pool, f, "stone", 250, 10)
	shPut(t, pool, f, "timber", 250, 10)
	count := func() (n, ev int) {
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE kind='StorageOverflow' AND body_json->>'settlement_id'=$1 AND body_json->>'pool'='storehouse'`, f.settlementID.String()).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='StorageOverflow'`, f.settlementID).Scan(&ev); err != nil {
			t.Fatal(err)
		}
		return
	}
	for i := 1; i <= 5; i++ { // five days of lost production, two goods each
		shTick(t, pool, f, f.tick+i)
		shCloseDay(t, pool, f)
	}
	n, ev := count()
	if n != 1 {
		t.Fatalf("%d dispatches over a five-day spell with two goods, want exactly 1", n)
	}
	if ev < 6 {
		t.Fatalf("only %d StorageOverflow outcomes: every lost quantity must stay an event", ev)
	}
	var other float64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(sum(amount),0) FROM settlement_ledger WHERE settlement_id=$1 AND tick>$2 AND category='unlabelled'`, f.settlementID, f.tick).Scan(&other); err != nil {
		t.Fatal(err)
	}
	var ovf float64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(sum(amount),0) FROM settlement_ledger WHERE settlement_id=$1 AND category='overflow'`, f.settlementID).Scan(&ovf); err != nil {
		t.Fatal(err)
	}
	if !near(other, 0) || ovf >= 0 {
		t.Fatalf("ledger: unlabelled %.4f (want 0), overflow %.4f (want < 0)", other, ovf)
	}
	// Stock used (below the ceiling), then 30 quiet days, then full again -> a second dispatch.
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET rate=0,amount=100,calc_tick=current_world_tick() WHERE settlement_id=$1`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	shTick(t, pool, f, f.tick+40)
	shCloseDay(t, pool, f)
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET rate=10,calc_tick=current_world_tick() WHERE settlement_id=$1`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	shTick(t, pool, f, f.tick+80)
	shCloseDay(t, pool, f)
	if n, _ := count(); n != 2 {
		t.Fatalf("%d dispatches after a second spell, want 2", n)
	}
}

// A payment that realizes accrual with settled() (not through storage_capped) cannot
// smuggle accrual past a full store.
func TestStorehouse_DebitRealizationIsClipped(t *testing.T) {
	pool := testPool(t)
	f := newFoodFixture(t, pool, "sh-debit", 100)
	shPut(t, pool, f, "stone", 300, 10)
	shPut(t, pool, f, "timber", 200, 0)
	shTick(t, pool, f, f.tick+5)
	if _, err := pool.Exec(context.Background(),
		`UPDATE settlement_goods SET amount=settled(amount,rate,calc_tick)-1,calc_tick=current_world_tick() WHERE settlement_id=$1 AND good_key='stone'`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if s := shAmount(t, pool, f, "stone"); !near(s, 299) {
		t.Fatalf("stone %.2f after a 1-stone payment in a full store, want 299 (accrual 50 lost)", s)
	}
}

// Silver mined = what the store admitted, not what the rate would have given.
func TestStorehouse_SilverMiningCountsAdmittedOnly(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFoodFixture(t, pool, "sh-silver", 100)
	shPut(t, pool, f, "silver", 480, 10)
	shTick(t, pool, f, f.tick+10) // 100 accrued, 20 room
	shRealizeOneByOne(t, pool, f, "silver")
	var mined float64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(credited_silver,0) FROM silver_mining_totals WHERE settlement_id=$1`, f.settlementID).Scan(&mined); err != nil {
		t.Fatal(err)
	}
	if !near(mined, 20) || !near(shAmount(t, pool, f, "silver"), 500) {
		t.Fatalf("mined %.4f, held %.4f; want 20 / 500", mined, shAmount(t, pool, f, "silver"))
	}
}

// Go and SQL tell the same ceiling, ladder and membership.
func TestStorehouseSQLMirror(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFoodFixture(t, pool, "sh-mirror", 100)
	for lvl := 0; lvl <= 4; lvl++ {
		shLevel(t, pool, f, min(lvl, 3))
		if lvl == 4 {
			if _, err := pool.Exec(ctx, `UPDATE buildings SET level=4 WHERE settlement_id=$1 AND building_type='storehouse'`, f.settlementID); err != nil {
				t.Fatal(err)
			}
		}
		var c float64
		if err := pool.QueryRow(ctx, `SELECT storehouse_cap($1)`, f.settlementID).Scan(&c); err != nil {
			t.Fatal(err)
		}
		if c != province.StorehouseCap(lvl) {
			t.Errorf("level %d: SQL %.0f, Go %.0f", lvl, c, province.StorehouseCap(lvl))
		}
	}
	for _, g := range []string{"grain", "fish", "oil", "livestock", "horses", "cult", "wine", "silver", "stone", "timber", "bronze", "cedar", "pottery", "incense", "purple"} {
		var in bool
		if err := pool.QueryRow(ctx, `SELECT storehouse_stored_good($1)`, g).Scan(&in); err != nil {
			t.Fatal(err)
		}
		if in != province.StorehouseStoredGood(g) {
			t.Errorf("%s: SQL %v, Go %v", g, in, province.StorehouseStoredGood(g))
		}
	}
	_ = uuid.Nil
}
