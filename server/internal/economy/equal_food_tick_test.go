package economy

import (
	"context"
	"formatet/megaron/server/internal/events"
	"math"
	"testing"
)

func TestFourFoodActualMealAndRetryShareOneClaim(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFoodFixture(t, pool, "four-foods", 1000)
	seedFoodStock(t, pool, f, 100, 900, 50)
	for _, good := range []string{GoodOil, GoodWine} {
		if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,$2,500,0,100000,$3)`, f.settlementID, good, f.tick); err != nil {
			t.Fatal(err)
		}
	}
	event := events.ScheduledEvent{ID: 280601, WorldID: f.worldID, DueTick: f.tick + 1} // off the birth beat
	for i := 0; i < 3; i++ {
		if err := newFoodTickHandler(pool).Handle(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	var used map[string]float64
	var observed int
	if err := pool.QueryRow(ctx, `SELECT food_consumed,food_consumed_tick FROM settlements WHERE id=$1`, f.settlementID).Scan(&used, &observed); err != nil {
		t.Fatal(err)
	}
	if len(used) != 4 || observed != f.tick {
		t.Fatalf("meal=%v tick=%d", used, observed)
	}
	for good, want := range map[string]float64{GoodGrain: .25, GoodFish: 2.25, GoodOil: 1.25, GoodWine: 1.25} {
		if math.Abs(used[good]-want) > 1e-9 {
			t.Errorf("%s used=%g want%g", good, used[good], want)
		}
	}
	if got := foodStockOf(t, pool, f.settlementID, GoodGrain); got != 99.75 {
		t.Fatal(got)
	}
	if got := foodStockOf(t, pool, f.settlementID, GoodLivestock); got != 50 {
		t.Fatal(got)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	meal, err := TakeFood(ctx, tx, f.settlementID, 100000, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if meal.Unmet <= 0 {
		t.Fatal("insufficient food accepted")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if got := foodStockOf(t, pool, f.settlementID, GoodGrain); got != 99.75 {
		t.Fatalf("refusal spent %g", got)
	}
}
