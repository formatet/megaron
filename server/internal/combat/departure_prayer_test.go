package combat

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestBetaStillnessDamageSnapshotAndReturn(t *testing.T) {
	for _, freight := range []bool{false, true} {
		t.Run(map[bool]string{false: "march", true: "freight"}[freight], func(t *testing.T) {
			pool, world, owner, route := seaStormWorld(t)
			ctx := context.Background()
			ship := uuid.Nil
			if freight {
				ship, _ = mkStormFreight(t, pool, world, owner, route)
			} else {
				ship, _ = mkStormFleet(t, pool, world, owner, route)
			}
			if _, err := pool.Exec(ctx, `UPDATE units SET stillness_depart_tick=0,stillness_arrive_tick=10,stillness_owner_id=$2 WHERE id=$1`, ship, owner); err != nil {
				t.Fatal(err)
			}
			stormsOverHexes(t, pool, world, 1, 2, 4, 5, 7)
			h := NewSeaStormScanHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
			h.Dice = newStormDice(1)
			for day := 0; day <= 6; day++ {
				runStormDay(t, h, world, day)
			}
			if _, hull := shipState(t, pool, ship); hull != 5 {
				t.Fatalf("Stillness failed: hull%d", hull)
			}
			if countShipEvents(t, pool, ship, EventShipStormDamaged) != 0 {
				t.Fatal("protected voyage emitted damage")
			}
			// The same old stamp cannot protect a different voyage.
			if freight {
				_, err := pool.Exec(ctx, `INSERT INTO transports(world_id,owner_id,kind,category,origin_q,origin_r,dest_q,dest_r,departs_at,arrives_at,due_tick,status,interceptable,ship_unit_id,journey,departed_tick) SELECT world_id,owner_id,'ship_return',category,origin_q,origin_r,dest_q,dest_r,departs_at,arrives_at,11,'in_transit',true,ship_unit_id,journey,1 FROM transports WHERE ship_unit_id=$1`, ship)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = pool.Exec(ctx, `UPDATE transports SET status='delivered' WHERE ship_unit_id=$1 AND departed_tick=0`, ship)
			} else {
				route.StartTick = 1
				route.EndTick = 11
				raw, _ := json.Marshal(route)
				_, _ = pool.Exec(ctx, `UPDATE units SET depart_tick=1,arrive_tick=11,march_route=$2 WHERE id=$1`, ship, raw)
			}
			runStormDay(t, h, world, 1)
			if _, hull := shipState(t, pool, ship); hull != 3 {
				t.Fatalf("next leg inherited Stillness: hull%d", hull)
			}
		})
	}
}

func TestBetaShipDepartureSpeedLatePrayerAndActualReturn(t *testing.T) {
	pool, world, owner, city := setupLandMarchWorldWithHome(t)
	ctx := context.Background()
	// Replace the fixture's six tiles with a port and straight sea lane.
	_, err := pool.Exec(ctx, `DELETE FROM map_tiles WHERE world_id=$1`, world)
	if err != nil {
		t.Fatal(err)
	}
	for q := 0; q <= 12; q++ {
		terrain := "coastal_sea"
		if q == 0 {
			terrain = "plains"
		}
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,0,$3)`, world, q, terrain); err != nil {
			t.Fatal(err)
		}
	}
	clk := clock.NewTestClock(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC))
	sched := events.NewScheduler(pool, clk)
	store := events.NewStore(pool)
	var ship uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,settlement_id) VALUES($1,$2,'galley','naval',1,20,'garrison',0,0,$3) RETURNING id`, world, owner, city).Scan(&ship); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO departure_prayers(settlement_id,owner_id,effect) VALUES($1,$2,'next_ship_departure')`, city, owner)
	if err != nil {
		t.Fatal(err)
	}
	order := MarchOrder{WorldID: world, PlayerID: owner, UnitID: ship, TargetQ: 12, Intent: "patrol"}
	preview, err := PreviewMarch(ctx, pool, clk, order, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := StartMarch(ctx, pool, sched, store, clk, order, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.DurationTicks != 3 || preview.DurationTicks != 3 {
		t.Fatalf("Stillness speed/preview got%d/%d want3", result.DurationTicks, preview.DurationTicks)
	} // Eleven entered coastal hexes: 11*(1/3)*.75 rounds to3.
	raw, dep, arr := loadMarchRoute(t, pool, ship)
	// Prayer cast after departure is a pending city gift, never a voyage rewrite.
	_, err = pool.Exec(ctx, `INSERT INTO departure_prayers(settlement_id,owner_id,effect) VALUES($1,$2,'next_ship_departure')`, city, owner)
	if err != nil {
		t.Fatal(err)
	}
	raw2, dep2, arr2 := loadMarchRoute(t, pool, ship)
	if string(raw) != string(raw2) || *dep != *dep2 || *arr != *arr2 {
		t.Fatal("late prayer altered sailing")
	}
	_, err = pool.Exec(ctx, `UPDATE worlds SET current_tick=$2 WHERE id=$1`, world, *arr)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	h := NewUnitArrivalHandler(pool, store, nil, sched, clk, economy.SitosConfig{})
	path := []province.MapPosition{}
	for q := 12; q >= 1; q-- {
		path = append(path, province.MapPosition{Q: q})
	}
	_, ticks, err := h.dispatchLeg(ctx, tx, unitRow{id: ship, ownerID: owner, utype: "galley", category: "naval", crew: 20, homeSettlementID: &city}, 12, 0, 1, 0, path, 11.0/3, "patrol_return", false, world)
	if err != nil {
		t.Fatal(err)
	}
	if ticks != 3+4 {
		t.Fatalf("return duration inherited speed: due%d want7", ticks)
	}
	var protect *int
	if err := tx.QueryRow(ctx, `SELECT stillness_depart_tick FROM units WHERE id=$1`, ship).Scan(&protect); err != nil || protect != nil {
		t.Fatal("return retained storm flag")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestBetaArmyDoesNotConsumeDeparturePrayers(t *testing.T) {
	pool, world, owner, city := setupLandMarchWorldWithHome(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO departure_prayers(settlement_id,owner_id,effect) VALUES($1,$2,'next_land_departure'),($1,$2,'next_ship_departure')`, city, owner); err != nil {
		t.Fatal(err)
	}
	var army uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,settlement_id) VALUES($1,$2,'spearman','land',100,'garrison',0,0,$3) RETURNING id`, world, owner, city).Scan(&army); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC))
	result, err := StartMarch(ctx, pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk, MarchOrder{WorldID: world, PlayerID: owner, UnitID: army, TargetQ: 2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.DurationTicks != 3 {
		t.Fatalf("army affected by travel prayer: ticks%d", result.DurationTicks)
	}
	var pending int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM departure_prayers WHERE settlement_id=$1`, city).Scan(&pending); err != nil || pending != 2 {
		t.Fatalf("army consumed prayer %d %v", pending, err)
	}
}

func TestBetaStandingReturnDoesNotConsumePendingGift(t *testing.T) {
	pool := testPool(t)
	f := newStandingMovementFixture(t, pool, "beta-standing")
	ctx := context.Background()
	seedGoods(t, pool, f.capitalID, f.tick, 1000, 0)
	order := newStandingOrder(t, pool, f.worldID, f.owner, f.capitalID, f.townID, f.capitalID)
	addOutbound(t, pool, order, "grain", 200)
	addReturnGood(t, pool, order, "stone", 20)
	runStandingOrderTick(t, pool, f)
	out, _, _ := latestTransportForOrder(t, pool, order)
	if _, err := pool.Exec(ctx, `UPDATE transports SET status='delivered' WHERE id=$1`, out); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO departure_prayers(settlement_id,owner_id,effect) VALUES($1,$3,'next_land_departure'),($2,$3,'next_land_departure')`, f.capitalID, f.townID, f.owner); err != nil {
		t.Fatal(err)
	}
	runStandingOrderTick(t, pool, f)
	ret, kind, _ := latestTransportForOrder(t, pool, order)
	if kind != "standing_order_return" {
		t.Fatalf("not real return %s", kind)
	}
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT journey FROM transports WHERE id=$1`, ret).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var j province.TradeJourney
	if err := json.Unmarshal(raw, &j); err != nil {
		t.Fatal(err)
	}
	if j.Thread || j.TimeFactorMilli != 0 {
		t.Fatal("standing return inherited Thread")
	}
	var pending int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM departure_prayers WHERE settlement_id IN($1,$2)`, f.capitalID, f.townID).Scan(&pending); err != nil || pending != 2 {
		t.Fatal("standing return consumed pending gift")
	}
}

func TestBetaStillnessDoesNotFollowShipOwner(t *testing.T) {
	pool, world, owner, route := seaStormWorld(t)
	ship, _ := mkStormFleet(t, pool, world, owner, route)
	ctx := context.Background()
	var other uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, uuid.NewString()).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE units SET stillness_depart_tick=0,stillness_arrive_tick=10,stillness_owner_id=$2,owner_id=$3 WHERE id=$1`, ship, owner, other); err != nil {
		t.Fatal(err)
	}
	stormsOverHexes(t, pool, world, 1, 2)
	h := NewSeaStormScanHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.Dice = newStormDice(1)
	runStormDay(t, h, world, 0)
	if _, hull := shipState(t, pool, ship); hull != 3 {
		t.Fatalf("captured owner inherited Stillness: hull%d", hull)
	}
}
