package combat

import (
	"context"
	"math"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestFishOnlyTownFeedsArmyAndKeepsGrainAuditHonest(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "fish-army")
	seedGoods(t, pool, f.capitalID, f.tick, 0, 1000)
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'fish',500,0,1000,$2)`, f.capitalID, f.tick); err != nil {
		t.Fatal(err)
	}
	var uid uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,settlement_id,support_settlement_id) VALUES($1,$2,'spearman','land',100,'garrison',$3,$3) RETURNING id`, f.worldID, f.owner, f.capitalID).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	runUpkeep(t, pool, f)
	want := 500 - UnitUpkeep("spearman", "land", 100, "garrison", 0).Grain
	if got := settledAmount(t, pool, f.capitalID, "fish"); got != want {
		t.Fatalf("fish=%g want%g", got, want)
	}
	if got := settledAmount(t, pool, f.capitalID, "grain"); got != 0 {
		t.Fatal(got)
	}
	var grain, food, fish float64
	if err := pool.QueryRow(ctx, `SELECT (payload->>'grain_total')::float8,(payload->>'food_total')::float8,(payload->'food_by_good'->>'fish')::float8 FROM events WHERE world_id=$1 AND stream_id=$2 AND event_type='UpkeepSettled'`, f.worldID, f.capitalID).Scan(&grain, &food, &fish); err != nil {
		t.Fatal(err)
	}
	if grain != 0 || food != 500-want || fish != food {
		t.Fatalf("grain/food/fish audit=%g/%g/%g", grain, food, fish)
	}
}

func TestFishOnlyVoyagePreviewDispatchEatAndReturnSameGood(t *testing.T) {
	pool, world, owner, city := setupLandMarchWorldWithHome(t)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`DELETE FROM map_tiles WHERE world_id=$1`, world)
	for q := 0; q <= 12; q++ {
		terrain := "coastal_sea"
		if q == 0 {
			terrain = "plains"
		}
		exec(`INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,0,$3)`, world, q, terrain)
	}
	for _, good := range economy.FoodGoods {
		n := 0.0
		if good == economy.GoodFish {
			n = 500
		}
		exec(`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,$2,$3,0,1000,0) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=$3,rate=0,calc_tick=0`, city, good, n)
	}
	var ship uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,settlement_id,support_settlement_id) VALUES($1,$2,'galley','naval',1,20,'garrison',0,0,$3,$3) RETURNING id`, world, owner, city).Scan(&ship); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC))
	order := MarchOrder{WorldID: world, PlayerID: owner, UnitID: ship, TargetQ: 12, Intent: "patrol"}
	if _, err := PreviewMarch(ctx, pool, clk, order, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := StartMarch(ctx, pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk, order, nil); err != nil {
		t.Fatal(err)
	}
	var total float64
	var goods map[string]float64
	if err := pool.QueryRow(ctx, `SELECT provisions,provision_goods FROM units WHERE id=$1`, ship).Scan(&total, &goods); err != nil {
		t.Fatal(err)
	}
	if total <= 0 || goods[economy.GoodFish] != total || goods[economy.GoodGrain] != 0 {
		t.Fatalf("loaded=%g/%v", total, goods)
	}
	h := NewUpkeepHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil, nil)
	if fed, err := h.eatProvisions(ctx, ship, 1); err != nil || !fed {
		t.Fatalf("fed=%v err=%v", fed, err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := returnProvisions(ctx, tx, ship, city); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if got := settledAmount(t, pool, city, economy.GoodFish); math.Abs(got-499) > 1e-9 {
		t.Fatalf("returned fish=%g want499", got)
	}
	if got := settledAmount(t, pool, city, economy.GoodGrain); got != 0 {
		t.Fatalf("fish became grain: %g", got)
	}
}

func TestFishOnlyStandingCaravanProtectsFoodReserve(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newStandingMovementFixture(t, pool, "fish-caravan")
	seedGoods(t, pool, f.capitalID, f.tick, 0, 0)
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'fish',1000,0,1000000,$2)`, f.capitalID, f.tick); err != nil {
		t.Fatal(err)
	}
	order := newStandingOrder(t, pool, f.worldID, f.owner, f.capitalID, f.townID, f.capitalID)
	addOutbound(t, pool, order, "fish", 200)
	runStandingOrderTick(t, pool, f)
	if n := transportCountForOrder(t, pool, order); n != 1 {
		t.Fatalf("dispatch count=%d", n)
	}
	if got := settledAmount(t, pool, f.capitalID, "fish"); got != 788 {
		t.Fatalf("fish=%g want788", got)
	}
	if got := settledAmount(t, pool, f.capitalID, "grain"); got != 0 {
		t.Fatal(got)
	}
}
