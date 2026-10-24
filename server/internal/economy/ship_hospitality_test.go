package economy

import (
	"context"
	"math"
	"testing"

	"github.com/google/uuid"
)

// Guest-friendship (Timothy 2026-10-24): any town that receives a ship feeds it
// for the way home, for nothing, from food it can spare after its own day's meal.
func TestHospitalityFeedsAForeignShipButNeverItsOwnMeal(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name               string
		grain              float64
		population, needed int
	}{
		{name: "plenty", population: 1000, grain: 100, needed: 20},
		{name: "only the town's own meal", population: 1000, needed: 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFoodFixture(t, pool, "hospitality", tc.population)
			meal := GrainConsumptionPerTick(tc.population)
			grain := tc.grain
			if grain == 0 {
				grain = meal
			}
			seedFoodStock(t, pool, f, grain, 0, 0)
			var guest, ship uuid.UUID
			if err := pool.QueryRow(ctx, `INSERT INTO players (username, password_hash) VALUES ($1,'x') RETURNING id`, "guest-"+uuid.NewString()).Scan(&guest); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,provisions,provision_goods)
 VALUES($1,$2,'galley','naval',1,20,'garrison',$3,0,'{}') RETURNING id`, f.worldID, guest, f.settlementID).Scan(&ship); err != nil {
				t.Fatal(err)
			}
			want := math.Min(float64(tc.needed), math.Max(0, grain-meal))
			for visit := 0; visit < 2; visit++ { // a replayed visit fills no second ration
				tx, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if err = HospitalityFood(ctx, tx, ship, f.settlementID, float64(tc.needed)); err != nil {
					t.Fatal(err)
				}
				if err = tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			goods, err := StoredProvisions(ctx, tx, ship)
			if err != nil {
				t.Fatal(err)
			}
			stock, err := LoadFood(ctx, tx, f.settlementID, false)
			if err != nil {
				t.Fatal(err)
			}
			if got := FoodAmount(goods); math.Abs(got-want) > 1e-6 {
				t.Fatalf("ship got %.3f food, want %.3f", got, want)
			}
			if got := FoodAmount(stock); math.Abs(got-(grain-want)) > 1e-6 || got+1e-6 < meal {
				t.Fatalf("town kept %.3f food, want %.3f and at least its own meal %.3f", got, grain-want, meal)
			}
		})
	}
}
