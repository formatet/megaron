package economy

import (
	"context"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Timothy 2026-10-31: one animal = one food, and a herd standing at its pen cap
// counts its overflow in food_net (HerdSurplusRate; SQL twin in the growth gate).

func TestHerdSurplusRate(t *testing.T) {
	cases := []struct {
		herd float64
		pen  int
		want float64
	}{
		{49, 0, 0},          // under the cap: the herd is still growing
		{50, 0, 0.25},       // at the cap: 5 calves per 20 game days, one food each
		{60, 0, 0.30},       // over the cap
		{50, 1, 0},          // a pen raises the cap to 150
		{149, 1, 0},         // still under it
		{150, 1, 0.75},      // at it
		{0, 0, 0},           // no herd
		{9, -1, 0},          // nonsense pen level behaves as no pen
		{55.9, 0, 0.25},     // fractional stock: whole tens only
		{100, 0, 10.0 / 20}, // 10 calves per 20 game days
	}
	for _, c := range cases {
		if got := HerdSurplusRate(c.herd, c.pen); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("HerdSurplusRate(%g, pen %d) = %g, want %g", c.herd, c.pen, got, c.want)
		}
	}
}

type growthRow struct {
	foodNet, variety float64
}

func growthOf(t *testing.T, pool *pgxpool.Pool, sql string, sid uuid.UUID) growthRow {
	t.Helper()
	var r growthRow
	if err := pool.QueryRow(context.Background(),
		`SELECT food_net, variety FROM settlements s CROSS JOIN LATERAL (`+sql+`) g WHERE s.id=$1`, sid,
	).Scan(&r.foodNet, &r.variety); err != nil {
		t.Fatal(err)
	}
	return r
}

func seedRate(t *testing.T, pool *pgxpool.Pool, f foodFixture, good string, amount, rate float64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1,$2,$3,$4,1000000,current_world_tick())`, f.settlementID, good, amount, rate); err != nil {
		t.Fatal(err)
	}
}

// (a)+(b): the growth gate's SQL food_net equals the Go balance every surface
// shows (FoodNet + HerdSurplusRate), with and without a herd, under and at the cap.
func TestPopulationGrowthSQL_HerdAtCapRaisesFoodNet(t *testing.T) {
	pool := testPool(t)
	const population = 1000 // eats 5
	cases := []struct {
		name string
		herd float64 // <0: no livestock row
		pen  int
		want float64
	}{
		{"no herd row", -1, 0, 0},
		{"herd under the cap", 49, 0, 0},
		{"herd at the cap", 50, 0, 0.25},
		{"herd over the cap", 80, 0, 0.4},
		{"pen lifts the cap above the herd", 50, 1, 0},
		{"herd at a level-1 pen's cap", 150, 1, 0.75},
	}
	base := math.NaN()
	for i, c := range cases {
		f := newFoodFixture(t, pool, "herd-net-"+string(rune('a'+i)), population)
		seedRate(t, pool, f, GoodGrain, 1000, 5) // production = ration: net 0 without the herd
		if c.herd >= 0 {
			seedRate(t, pool, f, GoodLivestock, c.herd, 0)
		}
		if c.pen > 0 {
			if _, err := pool.Exec(context.Background(), `INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1,'pen',$2)`, f.settlementID, c.pen); err != nil {
				t.Fatal(err)
			}
		}
		got := growthOf(t, pool, PopulationGrowthSQL(), f.settlementID).foodNet
		if math.IsNaN(base) {
			base = got
		}
		herd := math.Max(0, c.herd)
		if want := FoodNet(5+HerdSurplusRate(herd, c.pen), population); math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: SQL food_net=%g, Go surface balance=%g", c.name, got, want)
		}
		if math.Abs(got-base-c.want) > 1e-9 {
			t.Errorf("%s: food_net=%g, want no-herd net %g + %g", c.name, got, base, c.want)
		}
	}
}

// (c)+(d): the variety share is counted in food units. With an animal worth 1
// a tenth of the meal counts; with a value of 3 (the SQL is parameterised so the
// share is proved to be in units, not animals) one animal of 28 eaten is 3/30.
func TestPopulationGrowthSQL_VarietyShareIsInFoodUnits(t *testing.T) {
	pool := testPool(t)
	cases := []struct {
		name        string
		value       int
		grain, herd string
		want        float64
	}{
		{"one animal of ten units, value 1", livestockFoodValue, "9", "1", 1.1},
		{"one animal of eleven units, value 1", livestockFoodValue, "10", "1", 1.0},
		{"one animal worth 3 of thirty units", 3, "27", "1", 1.1},
		{"one animal worth 3 of thirty-one units", 3, "28", "1", 1.0},
	}
	for i, c := range cases {
		f := newFoodFixture(t, pool, "herd-variety-"+string(rune('a'+i)), 1000)
		seedRate(t, pool, f, GoodGrain, 1000, 5)
		if _, err := pool.Exec(context.Background(),
			`UPDATE settlements SET food_consumed=jsonb_build_object('grain',$2::numeric,'livestock',$3::numeric),food_consumed_tick=current_world_tick() WHERE id=$1`,
			f.settlementID, c.grain, c.herd); err != nil {
			t.Fatal(err)
		}
		if got := growthOf(t, pool, populationGrowthSQL(c.value), f.settlementID).variety; math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: variety=%g, want %g", c.name, got, c.want)
		}
	}
}
