package economy

import (
	"context"
	"math"
	"testing"
)

func TestArmyGrain_ConservationAndShortage(t *testing.T) {
	for _, tc := range []struct {
		name                string
		city, reserve, need float64
		paid                bool
		draw                float64
	}{
		{"city first", 2, 10, 1, true, 0}, {"split exact", .25, .25, .5, true, .25},
		{"reserve despite zero cap", 0, 1, .5, true, .5}, {"insufficient grain", .1, .2, .5, false, 0},
		{"fish is not army grain", 0, 0, .5, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			pool := testPool(t)
			_, sid := granaryFixture(t, pool, ctx, 5, 1000, []fixtureGood{{GoodGrain, tc.city, tc.city}, {GoodFish, 1000, 10000}}, map[string]float64{GoodGrain: tc.reserve, GoodFish: 100})
			paid, draw, err := DebitArmyGrain(ctx, pool, sid, tc.need)
			if err != nil {
				t.Fatal(err)
			}
			var city, held, fish float64
			if err = pool.QueryRow(ctx, `SELECT (SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'),(SELECT amount FROM settlement_granary WHERE settlement_id=$1 AND good_key='grain'),(SELECT amount FROM settlement_granary WHERE settlement_id=$1 AND good_key='fish')`, sid).Scan(&city, &held, &fish); err != nil {
				t.Fatal(err)
			}
			consumed := 0.0
			if tc.paid {
				consumed = tc.need
			}
			if paid != tc.paid || math.Abs(draw-tc.draw) > 1e-9 || math.Abs(city+held-(tc.city+tc.reserve-consumed)) > 1e-9 || city < 0 || held < 0 || fish != 100 {
				t.Fatalf("paid%v draw%f city%f reserve%f fish%f", paid, draw, city, held, fish)
			}
		})
	}
}

func TestArmyGrain_ConcurrentDebitsCannotOverdraw(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	_, sid := granaryFixture(t, pool, ctx, 5, 1000, []fixtureGood{{GoodGrain, 0, 0}}, map[string]float64{GoodGrain: 1})
	result := make(chan bool, 8)
	errs := make(chan error, 8)
	for n := 0; n < 8; n++ {
		go func() { paid, _, err := DebitArmyGrain(ctx, pool, sid, .5); result <- paid; errs <- err }()
	}
	paidCount := 0
	for n := 0; n < 8; n++ {
		if <-result {
			paidCount++
		}
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	var left float64
	if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_granary WHERE settlement_id=$1 AND good_key='grain'`, sid).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if paidCount != 2 || left != 0 {
		t.Fatalf("paid%d left%f", paidCount, left)
	}
}
