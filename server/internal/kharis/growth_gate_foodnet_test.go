package kharis

import (
	"context"
	"sync/atomic"
	"testing"

	"formatet/megaron/server/internal/economy"
)

// Pin the real SQL growth gate against the public food balance, independently
// of the day-by-day grain/growth arithmetic in Growth_MatchesGoMirror. FoodTick
// has already fed the city (unmet=0); stocked food and variety cannot substitute
// for a positive daily grain+fish balance. Only population direction is tested.
func TestApplyDecay_GrowthGateMatchesFoodNet(t *testing.T) {
	const population = 1000 // below the soft cap, away from starvation's floor
	need := economy.GrainConsumptionPerTick(population)
	cases := []struct {
		name     string
		rates    map[string]float64
		wantGrow bool
	}{
		{"grain deficit", map[string]float64{economy.GoodGrain: need - 0.25}, false},
		{"grain balanced", map[string]float64{economy.GoodGrain: need}, false},
		{"grain surplus", map[string]float64{economy.GoodGrain: need + 0.25}, true},
		{"fish deficit", map[string]float64{economy.GoodFish: need - 0.25}, false},
		{"fish balanced", map[string]float64{economy.GoodFish: need}, false},
		{"fish surplus", map[string]float64{economy.GoodFish: need + 0.25}, true},
		{"mixed deficit", map[string]float64{economy.GoodGrain: need / 2, economy.GoodFish: need/2 - 0.25}, false},
		{"mixed balanced", map[string]float64{economy.GoodGrain: need / 2, economy.GoodFish: need / 2}, false},
		{"mixed surplus", map[string]float64{economy.GoodGrain: need / 2, economy.GoodFish: need/2 + 0.25}, true},
		{"livestock is emergency food", map[string]float64{economy.GoodLivestock: need * 10}, false},
		{"wine adds variety only", map[string]float64{economy.GoodWine: need * 10}, false},
		{"oil adds variety only", map[string]float64{economy.GoodOil: need * 10}, false},
		{"stocked food without production", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			terrains := [6]string{"plains", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone"}
			pool, worldID, settlementID := newGrowthFixture(t, terrains, population)
			ctx := context.Background()
			// Fix this single tick's already-produced rates as input to the
			// growth gate. We are not testing workplace yield or FoodTick here.
			// All five diet goods are stocked, including deficit cases, so a
			// stock/variety-based replacement gate cannot accidentally pass.
			for _, good := range economy.FoodGoods {
				if _, err := pool.Exec(ctx,
					`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
					 VALUES ($1, $2, 10000, $3, 1000000, current_world_tick())
					 ON CONFLICT (settlement_id, good_key) DO UPDATE
					 SET amount = EXCLUDED.amount, rate = EXCLUDED.rate,
					     cap = EXCLUDED.cap, calc_tick = EXCLUDED.calc_tick`,
					settlementID, good, tc.rates[good]); err != nil {
					t.Fatalf("seed %s balance input: %v", good, err)
				}
			}
			if _, err := pool.Exec(ctx, `UPDATE settlements SET food_unmet_amount = 0 WHERE id = $1`, settlementID); err != nil {
				t.Fatal(err)
			}

			// Read the persisted inputs before applyDecay recomputes production.
			// The oracle uses the canonical Go list/function, never a copied SQL
			// gate, population-growth formula or expected rounded increment.
			rows, err := pool.Query(ctx, `SELECT good_key, rate FROM settlement_goods WHERE settlement_id = $1`, settlementID)
			if err != nil {
				t.Fatal(err)
			}
			rates := map[string]float64{}
			for rows.Next() {
				var good string
				var rate float64
				if err := rows.Scan(&good, &rate); err != nil {
					rows.Close()
					t.Fatal(err)
				}
				rates[good] = rate
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			var foodRate float64
			for _, good := range economy.NetFoodGoods {
				foodRate += rates[good]
			}
			foodNet := economy.FoodNet(foodRate, population)
			if (foodNet > 0) != tc.wantGrow {
				t.Fatalf("fixture/canonical food balance changed: net=%g, want growth=%v", foodNet, tc.wantGrow)
			}

			// The state every surface reports must be the direction the tick takes.
			state := economy.GrowthState(0, foodNet)
			if (state == economy.GrowthGrowing) != tc.wantGrow {
				t.Fatalf("GrowthState(0, %g) = %s, but the gate grows=%v", foodNet, state, tc.wantGrow)
			}

			newTestTickHandler(pool).applyDecay(ctx, worldID, atomic.AddInt64(&advanceOneDayEventID, 1))
			var after int
			if err := pool.QueryRow(ctx, `SELECT population FROM settlements WHERE id = $1`, settlementID).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if (after > population) != (state == economy.GrowthGrowing) || (after == population) != (state == economy.GrowthHolding) {
				t.Errorf("GrowthState %s but population %d->%d", state, population, after)
			}
			if tc.wantGrow {
				if after <= population {
					t.Errorf("FoodNet=%g but population %d->%d did not grow", foodNet, population, after)
				}
			} else if after != population {
				t.Errorf("fed city with FoodNet=%g changed population %d->%d, want hold", foodNet, population, after)
			}
		})
	}
}
