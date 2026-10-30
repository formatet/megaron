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
	for i, foods := range [][]string{{economy.GoodGrain, economy.GoodFish}, economy.FoodGoods, economy.FoodGoods} {
		wid, sid, _ := starvationWarningFixture(t, 0, 0)
		exec := func(q string, args ...any) {
			t.Helper()
			if _, err := pool.Exec(ctx, q, args...); err != nil {
				t.Fatal(err)
			}
		}
		exec(`UPDATE settlements SET population=10000, wall_level=3 WHERE id=$1`, sid)
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
		if i == 2 {
			exec(`UPDATE worlds SET current_tick=1 WHERE id=$1`, wid)
		}
		newTestTickHandler(pool).applyDecay(ctx, wid, int64(280200+i))
		var pop int
		if err := pool.QueryRow(ctx, `SELECT population FROM settlements WHERE id=$1`, sid).Scan(&pop); err != nil {
			t.Fatal(err)
		}
		gains = append(gains, pop-10000)
	}
	if gains[0] <= 0 || gains[1] <= gains[0] || gains[2] >= gains[0] {
		t.Fatalf("growth from2/4 eaten varieties=%v", gains)
	}
}

// Exercise the actual proportional meal and persisted outcome, not a hand-built
// variety projection. In particular a tiny oil stock must not buy a full bonus.
func TestGrowthVarietyRequiresSubstantialMealShare(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	cases := []struct {
		name       string
		oil, grain float64
		wantBonus  bool
	}{
		{"grain only", 0, 1000, false},
		{"one oil among thousand grain", 1, 1000, false},
		{"just below tenth", 99, 901, false},
		{"exactly tenth", 100, 900, true},
		{"three tenths", 300, 700, true},
	}
	var plainGain int
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wid, sid, _ := starvationWarningFixture(t, 0, 0)
			exec := func(q string, args ...any) {
				t.Helper()
				if _, err := pool.Exec(ctx, q, args...); err != nil {
					t.Fatal(err)
				}
			}
			exec(`UPDATE settlements SET population=10000, wall_level=3 WHERE id=$1`, sid)
			exec(`UPDATE settlement_goods SET amount=0,rate=0,calc_tick=0 WHERE settlement_id=$1`, sid)
			for good, amount := range map[string]float64{economy.GoodGrain: tc.grain, economy.GoodOil: tc.oil} {
				exec(`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,$2,$3,0,100000,0) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=$3,rate=0,cap=100000,calc_tick=0`, sid, good, amount)
			}
			if err := newTestFoodTickHandler(pool).Handle(ctx, events.ScheduledEvent{ID: int64(280300 + i), WorldID: wid, DueTick: 0}); err != nil {
				t.Fatal(err)
			}
			var meal map[string]float64
			if err := pool.QueryRow(ctx, `SELECT food_consumed FROM settlements WHERE id=$1`, sid).Scan(&meal); err != nil {
				t.Fatal(err)
			}
			if tc.oil > 0 && meal[economy.GoodOil] <= 0 {
				t.Fatalf("real oil meal absent: %v", meal)
			}
			// Give every case identical subsequent stocks/rates. Only the actual meal
			// above determines variety; later oil availability cannot affect the bonus.
			for _, good := range economy.FoodGoods {
				exec(`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,$2,2500,25,100000,0) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=2500,rate=25,cap=100000,calc_tick=0`, sid, good)
			}
			newTestTickHandler(pool).applyDecay(ctx, wid, int64(280400+i))
			var population int
			if err := pool.QueryRow(ctx, `SELECT population FROM settlements WHERE id=$1`, sid).Scan(&population); err != nil {
				t.Fatal(err)
			}
			gain := population - 10000
			if i == 0 {
				plainGain = gain
				if gain <= 0 {
					t.Fatalf("no baseline growth: %d", gain)
				}
			}
			if tc.wantBonus && gain <= plainGain {
				t.Fatalf("substantial meal %v: gain%d must exceed plain%d", meal, gain, plainGain)
			}
			if !tc.wantBonus && gain != plainGain {
				t.Fatalf("crumbs meal %v: gain%d must equal plain%d", meal, gain, plainGain)
			}
		})
	}
}
