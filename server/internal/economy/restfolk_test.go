package economy

import (
	"context"
	"math"
	"testing"
)

func TestRestfolk_BestWorkerNotCatchmentSum(t *testing.T) {
	opts := make([]HexOption, 9)
	for i := range opts {
		opts[i] = HexOption{RatePerGood: map[string]float64{GoodGrain: 1}, MultPerGood: map[string]float64{GoodGrain: 1}, PlaceCapPerGood: map[string]int{GoodGrain: 4}}
	}
	a := AutomaticFoodForPopulation(opts, 1042)
	if math.Abs(a.RemainderGrain-.42) > 1e-9 {
		t.Fatalf("42 people must give .42 of ONE best grain worker, not nine hexes: %+v", a)
	}
	if a = AutomaticFoodForPopulation(opts, 1000); a.RemainderGrain != 0 {
		t.Fatalf("whole hundreds %+v", a)
	}
}

func TestRestfolk_EligibleYieldAndBuildingBonus(t *testing.T) {
	for _, tc := range []struct {
		name        string
		options     []HexOption
		grain, fish float64
	}{
		{"fish beats grain", []HexOption{{RatePerGood: map[string]float64{GoodGrain: .5, GoodFish: 1}, MultPerGood: map[string]float64{GoodGrain: 1, GoodFish: 1}, PlaceCapPerGood: map[string]int{GoodGrain: 4, GoodFish: 2}}}, 0, .42},
		{"farm beats fish", []HexOption{{RatePerGood: map[string]float64{GoodGrain: .7, GoodFish: 1}, MultPerGood: map[string]float64{GoodGrain: 1.7, GoodFish: 1}, PlaceCapPerGood: map[string]int{GoodGrain: 4, GoodFish: 2}}}, .42 * .7 * 1.7, 0},
		{"no place no work", []HexOption{{RatePerGood: map[string]float64{GoodGrain: 99}, MultPerGood: map[string]float64{GoodGrain: 3}, PlaceCapPerGood: map[string]int{GoodGrain: 0}}}, 0, 0},
		{"no food", nil, 0, 0},
		{"grain wins tie", []HexOption{{RatePerGood: map[string]float64{GoodGrain: 1, GoodFish: 1}, MultPerGood: map[string]float64{GoodGrain: 1, GoodFish: 1}, PlaceCapPerGood: map[string]int{GoodGrain: 4, GoodFish: 2}}}, .42, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := AutomaticFoodForPopulation(tc.options, 1042)
			if math.Abs(a.RemainderGrain-tc.grain) > 1e-9 || math.Abs(a.RemainderFish-tc.fish) > 1e-9 {
				t.Fatalf("%+v want grain%f fish%f", a, tc.grain, tc.fish)
			}
			whole := AutomaticFoodForPopulation(tc.options, 1000)
			if whole.RemainderGrain != 0 || whole.RemainderFish != 0 {
				t.Fatal(whole)
			}
		})
	}
}

// Forecast runs before placement; actual stored rates, required workers and
// automatic food must agree afterwards, including a best hex already full.
func TestRestfolk_ForecastStartingRequiredRecompute(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	for _, tc := range []struct {
		name, terrain string
		farm          bool
	}{{"fish", "coastal_sea", false}, {"farm", "plains", true}, {"hills", "hills", true}} {
		t.Run(tc.name, func(t *testing.T) {
			wid, center, sid := foundingForecastFixture(t, 1142, tc.terrain)
			if tc.farm {
				c, ok, err := ChooseFarmHex(ctx, pool, wid, center, nil)
				if err != nil || !ok {
					t.Fatalf("farm %v %v", ok, err)
				}
				if _, err = pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level,hex_q,hex_r) VALUES($1,'farm',1,$2,$3)`, sid, c.Q, c.R); err != nil {
					t.Fatal(err)
				}
			}
			forecast, err := FoundingFoodForecastAt(ctx, pool, wid, center, tc.farm, nil, 1142)
			if err != nil {
				t.Fatal(err)
			}
			placed, suff, err := PlaceStartingWorkforce(ctx, pool, sid)
			if err != nil || !suff {
				t.Fatalf("start %d %v %v", placed, suff, err)
			}
			if err = RecomputeProduction(ctx, pool, sid); err != nil {
				t.Fatal(err)
			}
			var grain, fish float64
			if err = pool.QueryRow(ctx, `SELECT COALESCE(sum(rate) FILTER(WHERE good_key='grain'),0),COALESCE(sum(rate) FILTER(WHERE good_key='fish'),0) FROM settlement_goods WHERE settlement_id=$1`, sid).Scan(&grain, &fish); err != nil {
				t.Fatal(err)
			}
			required, achievable, a, err := FoodGubbarRequiredWithAutomatic(ctx, pool, sid)
			if err != nil || !achievable || required != placed || math.Abs(grain-forecast.GrainProduction) > 1e-9 || math.Abs(fish-forecast.FishProduction) > 1e-9 {
				t.Fatalf("forecast %+v actual %f/%f placed%d required%d achievable%v %v", forecast, grain, fish, placed, required, achievable, err)
			}
			if tc.name == "fish" && math.Abs(a.RemainderFish-.42) > 1e-9 {
				t.Fatal(a)
			}
			if tc.farm && a.RemainderGrain <= 0 {
				t.Fatal(a)
			}
			if _, err = pool.Exec(ctx, `UPDATE settlements SET population=1000 WHERE id=$1`, sid); err != nil {
				t.Fatal(err)
			}
			if err = RecomputeProduction(ctx, pool, sid); err != nil {
				t.Fatal(err)
			}
			_, _, whole, err := FoodGubbarRequiredWithAutomatic(ctx, pool, sid)
			if err != nil || whole.RemainderGrain != 0 || whole.RemainderFish != 0 {
				t.Fatalf("whole %+v %v", whole, err)
			}
			var afterGrain, afterFish float64
			if err = pool.QueryRow(ctx, `SELECT COALESCE(sum(rate) FILTER(WHERE good_key='grain'),0),COALESCE(sum(rate) FILTER(WHERE good_key='fish'),0) FROM settlement_goods WHERE settlement_id=$1`, sid).Scan(&afterGrain, &afterFish); err != nil {
				t.Fatal(err)
			}
			if math.Abs((grain-afterGrain)-a.RemainderGrain) > 1e-9 || math.Abs((fish-afterFish)-a.RemainderFish) > 1e-9 {
				t.Fatalf("stale remainder grain%f fish%f", grain-afterGrain, fish-afterFish)
			}
		})
	}
}
