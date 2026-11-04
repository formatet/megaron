package kharis

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testForecaster() combat.StorageForecaster {
	return combat.StorageForecaster{Tithe: func(ctx context.Context, tx pgx.Tx, sid uuid.UUID) (map[string]map[string]float64, error) {
		return ForecastTithe(ctx, tx, sid)
	}}
}

// foodOnly sets the city's food stock to grain only, no production, so the net is the citizens' meal.
func foodOnly(t *testing.T, pool *pgxpool.Pool, sid uuid.UUID, grain float64) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=0, rate=0, calc_tick=current_world_tick() WHERE settlement_id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',$2,0,5000,current_world_tick())
	 ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=$2, rate=0, cap=5000, calc_tick=current_world_tick()`, sid, grain); err != nil {
		t.Fatal(err)
	}
}

func foodLowNotices(t *testing.T, pool *pgxpool.Pool, wid uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind='FoodRunningLow'`, wid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The notice fires once when the net is negative and food lasts under 24 game days,
// stays silent at 24 or more, and returns only after the city has been well again.
func TestFoodRunningLow_OnceSilentAt24AndAgainAfterRecovery(t *testing.T) {
	pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
	ctx := context.Background()
	h := newTestTickHandler(pool)
	h.SetStorageForecaster(testForecaster())
	if _, err := pool.Exec(ctx, `DELETE FROM buildings WHERE settlement_id=$1 AND building_type='temple'`, sid); err != nil {
		t.Fatal(err)
	}
	observe := func() {
		t.Helper()
		if err := h.observeFood(ctx, wid); err != nil {
			t.Fatalf("observeFood: %v", err)
		}
		if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=current_tick+1 WHERE id=$1`, wid); err != nil {
			t.Fatal(err)
		}
	}
	// 1000 people eat 5 food a game day.
	foodOnly(t, pool, sid, 120) // exactly 24 game days: silent
	observe()
	observe()
	if n := foodLowNotices(t, pool, wid); n != 0 {
		t.Fatalf("24 game days of food must stay silent, got %d notices", n)
	}
	foodOnly(t, pool, sid, 119) // 23.8 game days: low
	observe()
	if n := foodLowNotices(t, pool, wid); n != 1 {
		t.Fatalf("under 24 game days: want 1 notice, got %d", n)
	}
	observe()
	observe()
	if n := foodLowNotices(t, pool, wid); n != 1 {
		t.Fatalf("still low: the notice must not repeat every tick, got %d", n)
	}
	foodOnly(t, pool, sid, 500) // recovered
	observe()
	foodOnly(t, pool, sid, 60) // low again
	observe()
	if n := foodLowNotices(t, pool, wid); n != 2 {
		t.Fatalf("after recovery a new notice is due: want 2, got %d", n)
	}
	// A positive net is silent whatever the stock.
	foodOnly(t, pool, sid, 10)
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET rate=9 WHERE settlement_id=$1 AND good_key='grain'`, sid); err != nil {
		t.Fatal(err)
	}
	observe()
	observe()
	if n := foodLowNotices(t, pool, wid); n != 2 {
		t.Fatalf("a growing store must stay silent, got %d", n)
	}
}

// The forecast names the army and the temples, so the notice can say who eats.
func TestCityForecast_NamesArmyTemplesAndCitizens(t *testing.T) {
	pool, wid, sid, h := ledgerCity(t)
	_ = h
	_ = wid
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id, support_settlement_id, origin_settlement_id)
	 SELECT world_id, owner_id, 'spearman','land',100,0,'garrison',id,id,id FROM settlements WHERE id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	fc, err := testForecaster().CityForecast(ctx, tx, sid)
	if err != nil {
		t.Fatal(err)
	}
	cats := map[string]float64{}
	for _, f := range fc.Food.Flows {
		cats[f.Category] = f.Amount
	}
	tithes := 0
	for c, a := range cats {
		if len(c) > 12 && c[:12] == "temple_tithe" && a < 0 {
			tithes++
		}
	}
	if cats["meal"] >= 0 || cats["army_ration"] >= 0 || cats[economyProduction] <= 0 || tithes < 1 {
		t.Fatalf("food flows must name production, citizens, army and temples: %+v", cats)
	}
	var sum float64
	for _, f := range fc.Food.Flows {
		sum += f.Amount
	}
	if d := sum - fc.Food.Net; d > 1e-9 || d < -1e-9 {
		t.Fatalf("food flows sum %.6f != net %.6f", sum, fc.Food.Net)
	}
	_ = economy.FoodLowDays
}

const economyProduction = "production"
