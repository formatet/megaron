package economy

import (
	"context"
	"math"
	"os"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Order 33 (hjorden v2): 1 animal = 3 food, the herd breeds itself, no herder.

func TestHerdAnimalIsWorthThreeFood(t *testing.T) {
	// 55 animals, cap 50: the 5 over the cap are 15 food, covering a meal of 15.
	meal := PlanFoodHerd(15, map[string]float64{GoodGrain: 100, GoodLivestock: 55}, true, HerdCap(0))
	if meal.Used[GoodLivestock] != 5 || meal.Used[GoodGrain] != 0 || meal.Unmet != 0 || meal.Lost != 0 {
		t.Fatalf("%+v", meal)
	}
	// Emergency slaughter also pays 3 per animal.
	em := PlanFoodHerd(9, map[string]float64{GoodLivestock: 40}, true, HerdCap(0))
	if em.Used[GoodLivestock] != 3 || em.Unmet != 0 {
		t.Fatalf("%+v", em)
	}
}

func TestHerdBirthsRule(t *testing.T) {
	cases := []struct {
		herd, grain float64
		tick        int
		born, fed   int
	}{
		{40, 100, 4005, 4, 4}, // fed: every 5th tick
		{40, 100, 4001, 0, 0}, // off-beat
		{40, 0, 4005, 0, 0},   // no grain: not on the 5-beat
		{40, 0, 4020, 4, 0},   // no grain: the 20-beat
		{40, 2, 4005, 2, 2},   // grain short: only the fed are born on the 5-beat
		{40, 2, 4020, 4, 2},   // the rest follow the 20-beat
		{9, 100, 4020, 0, 0},  // under ten animals: none
		{55, 100, 4000, 5, 5},
	}
	for _, c := range cases {
		born, fed := HerdBirths(c.herd, c.grain, c.tick)
		if born != c.born || fed != c.fed {
			t.Errorf("HerdBirths(%g,%g,%d)=%d,%d want %d,%d", c.herd, c.grain, c.tick, born, fed, c.born, c.fed)
		}
	}
}

func herdBirthsTick(t *testing.T, pool *pgxpool.Pool, f foodFixture, id int64, tick int) {
	t.Helper()
	ev := events.ScheduledEvent{ID: id, WorldID: f.worldID, DueTick: tick}
	if err := newFoodTickHandler(pool).Handle(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
}

func TestFoodTick_FedHerdBirthsOnTheFifthTick(t *testing.T) {
	pool := testPool(t)
	f := newFoodFixture(t, pool, "births-fed", 100) // demand 0.5
	seedFoodStock(t, pool, f, 100, 0, 40)
	herdBirthsTick(t, pool, f, 283001, 4005)
	if got := foodStockOf(t, pool, f.settlementID, GoodLivestock); got != 44 {
		t.Fatalf("herd=%g want 44", got)
	}
	// 4 grain for the calves + 0.5 for the people
	if got := foodStockOf(t, pool, f.settlementID, GoodGrain); math.Abs(got-95.5) > 1e-6 {
		t.Fatalf("grain=%g want 95.5", got)
	}
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM events WHERE stream_id=$1 AND event_type='HerdBirths' AND (payload->>'born')::int=4 AND (payload->>'fed')::int=4 AND (payload->>'grain_used')::float=4`, f.settlementID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("HerdBirths events=%d err=%v", n, err)
	}
}

func TestFoodTick_UnfedHerdBirthsOnlyOnTheTwentiethTick(t *testing.T) {
	pool := testPool(t)
	f := newFoodFixture(t, pool, "births-unfed", 100)
	seedFoodStock(t, pool, f, 0, 100, 40) // fish feeds the people, no grain for calves
	herdBirthsTick(t, pool, f, 283002, 4005)
	if got := foodStockOf(t, pool, f.settlementID, GoodLivestock); got != 40 {
		t.Fatalf("herd=%g want 40 on a 5-tick without grain", got)
	}
	herdBirthsTick(t, pool, f, 283003, 4020)
	if got := foodStockOf(t, pool, f.settlementID, GoodLivestock); got != 44 {
		t.Fatalf("herd=%g want 44 on the 20-tick", got)
	}
}

func TestFoodTick_CalvesOverTheCapAreEatenFirst(t *testing.T) {
	pool := testPool(t)
	f := newFoodFixture(t, pool, "births-cap", 3000) // demand 15
	seedFoodStock(t, pool, f, 100, 0, 50)
	herdBirthsTick(t, pool, f, 283004, 4005)
	// 5 calves (5 grain), the 5 over the cap are the 15 food of the meal.
	if got := foodStockOf(t, pool, f.settlementID, GoodLivestock); got != 50 {
		t.Fatalf("herd=%g want 50", got)
	}
	if got := foodStockOf(t, pool, f.settlementID, GoodGrain); math.Abs(got-95) > 1e-6 {
		t.Fatalf("grain=%g want 95", got)
	}
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM events WHERE stream_id=$1 AND event_type=$2`, f.settlementID, EventHerdLost).Scan(&n); err != nil || n != 0 {
		t.Fatalf("HerdLost events=%d err=%v", n, err)
	}
}

func TestMigration192LiftsHerders(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFoodFixture(t, pool, "mig192", 300)
	seedFoodStock(t, pool, f, 10, 0, 20)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES($1,1,'hex',1,0,'livestock'),($1,2,'hex',1,0,'grain')`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE settlement_goods SET rate=0.5 WHERE settlement_id=$1 AND good_key='livestock'`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../db/migrations/192_no_herder.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // idempotent
		if _, err := tx.Exec(ctx, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	var herders, others, rules, pop int
	var rate float64
	_ = tx.QueryRow(ctx, `SELECT COUNT(*) FILTER (WHERE good_key='livestock'), COUNT(*) FILTER (WHERE good_key='grain') FROM settlement_placement WHERE settlement_id=$1`, f.settlementID).Scan(&herders, &others)
	_ = tx.QueryRow(ctx, `SELECT rate FROM settlement_goods WHERE settlement_id=$1 AND good_key='livestock'`, f.settlementID).Scan(&rate)
	_ = tx.QueryRow(ctx, `SELECT COUNT(*) FROM production_rules WHERE good_key='livestock'`).Scan(&rules)
	_ = tx.QueryRow(ctx, `SELECT population FROM settlements WHERE id=$1`, f.settlementID).Scan(&pop)
	if herders != 0 || others != 1 || rate != 0 || rules != 0 || pop != 300 {
		t.Fatalf("herders=%d others=%d rate=%g rules=%d pop=%d", herders, others, rate, rules, pop)
	}
}

func TestPlainsHasNoHerderPlace(t *testing.T) {
	for _, r := range hexRules("plains", false, false, false) {
		if r.good == "livestock" {
			t.Fatal("plains still offers a livestock place")
		}
	}
}
