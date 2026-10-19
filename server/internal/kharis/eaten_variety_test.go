package kharis

import (
	"context"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"testing"
)

func TestGrowthVarietyReadsActualMealNotLaterStock(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	var gains []int
	for i, foods := range [][]string{{economy.GoodGrain, economy.GoodFish}, economy.FoodGoods} {
		wid, sid, _ := starvationWarningFixture(t, 0, 0)
		exec := func(q string, args ...any) {
			t.Helper()
			if _, err := pool.Exec(ctx, q, args...); err != nil {
				t.Fatal(err)
			}
		}
		exec(`UPDATE settlements SET population=10000 WHERE id=$1`, sid)
		exec(`UPDATE settlement_goods SET amount=0,rate=0 WHERE settlement_id=$1`, sid)
		for _, good := range foods {
			exec(`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,$2,$3,$4,100000,0) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=$3,rate=$4,cap=100000,calc_tick=0`, sid, good, 10000.0/float64(len(foods)), 100.0/float64(len(foods)))
		}
		if err := newTestFoodTickHandler(pool).Handle(ctx, events.ScheduledEvent{ID: int64(280100 + i), WorldID: wid, DueTick: 0}); err != nil {
			t.Fatal(err)
		}
		var eaten map[string]float64
		if err := pool.QueryRow(ctx, `SELECT food_consumed FROM settlements WHERE id=$1`, sid).Scan(&eaten); err != nil {
			t.Fatal(err)
		}
		if len(eaten) != len(foods) {
			t.Fatalf("meal=%v foods=%v", eaten, foods)
		}
		// Supplies after eating: identical stocks and rates when growth runs, but
		// two meals had two varieties and four meals had four. Stock is no oracle.
		for _, good := range economy.FoodGoods {
			exec(`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,$2,2500,25,100000,0) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=2500,rate=25,cap=100000,calc_tick=0`, sid, good)
		}
		newTestTickHandler(pool).applyDecay(ctx, wid, int64(280200+i))
		var pop int
		if err := pool.QueryRow(ctx, `SELECT population FROM settlements WHERE id=$1`, sid).Scan(&pop); err != nil {
			t.Fatal(err)
		}
		gains = append(gains, pop-10000)
	}
	if gains[0] <= 0 || gains[1] <= gains[0] {
		t.Fatalf("growth from2/4 eaten varieties=%v", gains)
	}
}
