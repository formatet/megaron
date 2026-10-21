package transport

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestBetaTransportFrozenDeparture(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO departure_prayers(settlement_id,owner_id,effect) VALUES($1,$2,'next_land_departure')`, f.sourceID, f.owner); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC))
	sched := events.NewScheduler(pool, clk)
	send := func(kind string) (province.TradeJourney, int) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		id, err := Dispatch(ctx, tx, sched, DispatchParams{WorldID: f.worldID, OwnerID: f.owner, OriginID: f.sourceID, DestID: f.destID, Kind: kind, Category: "land", DestQ: 3, DepartsAt: clk.Now(), Interceptable: true})
		if err != nil {
			t.Fatal(err)
		}
		var raw []byte
		var due int
		if err := tx.QueryRow(ctx, `SELECT journey,due_tick FROM transports WHERE id=$1`, id).Scan(&raw, &due); err != nil {
			t.Fatal(err)
		}
		var j province.TradeJourney
		if err := json.Unmarshal(raw, &j); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return j, due
	}
	j, due := send("transfer")
	if due != 3 || j.TravelTicks != 3 || !j.Thread {
		t.Fatalf("outbound %+v due%d", j, due)
	} // plains1 x3 x1.5 x.75=3.375
	raw, _ := json.Marshal(j)
	if departureInterceptRadius(inFlightTransport{category: "land", journey: raw}) != 1 {
		t.Fatal("Thread interception radius was not reduced")
	}
	// The next unblessed leg retains the baseline duration and radius.
	other, _ := send("transfer")
	raw, _ = json.Marshal(other)
	if other.TravelTicks != 5 || other.Thread || departureInterceptRadius(inFlightTransport{category: "land", journey: raw}) != 2 {
		t.Fatal("one-shot effect leaked")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO departure_prayers(settlement_id,owner_id,effect) VALUES($1,$2,'next_land_departure')`, f.sourceID, f.owner); err != nil {
		t.Fatal(err)
	}
	ret, _ := send("trade_return")
	if ret.TravelTicks != 5 || ret.Thread {
		t.Fatal("return inherited Thread")
	}
	var n int
	pool.QueryRow(ctx, `SELECT count(*) FROM departure_prayers WHERE settlement_id=$1`, f.sourceID).Scan(&n)
	if n != 1 {
		t.Fatal("return consumed fresh gift")
	}
}

func TestBetaFreightStillnessFrozenOnShip(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'fish',10,0,1000,0)`, f.sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM map_tiles WHERE world_id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	for q := 0; q <= 21; q++ {
		terrain := "coastal_sea"
		if q == 0 || q == 21 {
			terrain = "plains"
		}
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,0,$3)`, f.worldID, q, terrain); err != nil {
			t.Fatal(err)
		}
	}
	_, err := pool.Exec(ctx, `UPDATE provinces SET map_q=21 WHERE id=(SELECT province_id FROM settlements WHERE id=$1)`, f.destID)
	if err != nil {
		t.Fatal(err)
	}
	var ship uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,settlement_id) VALUES($1,$2,'galley','naval',1,20,'freighting',0,0,$3) RETURNING id`, f.worldID, f.owner, f.sourceID).Scan(&ship); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO departure_prayers(settlement_id,owner_id,effect) VALUES($1,$2,'next_ship_departure')`, f.sourceID, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC))
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	id, err := Dispatch(ctx, tx, events.NewScheduler(pool, clk), DispatchParams{WorldID: f.worldID, OwnerID: f.owner, OriginID: f.sourceID, DestID: f.destID, Kind: "trade", Category: "naval", DestQ: 21, DepartsAt: clk.Now(), ShipUnitID: &ship})
	if err != nil {
		t.Fatal(err)
	}
	var due, dep, protect int
	var owner uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT due_tick FROM transports WHERE id=$1`, id).Scan(&due); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT stillness_depart_tick,stillness_arrive_tick,stillness_owner_id FROM units WHERE id=$1`, ship).Scan(&dep, &protect, &owner); err != nil {
		t.Fatal(err)
	}
	if due != 7 || dep != 0 || protect != due || owner != f.owner {
		t.Fatalf("freight blessing not frozen due%d stamp%d/%d/%v", due, dep, protect, owner)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
