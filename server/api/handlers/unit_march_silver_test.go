package handlers

import (
	"context"
	"math"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestMarchSilverForecastMatchesActualTick(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()
	t.Setenv("UPKEEP_SOLD_SHARE", "0.7")
	_, err := pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`)
	if err != nil {
		t.Fatal(err)
	}
	var world, owner uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick) VALUES($1,'active',10) RETURNING id`, "march-silver-"+uuid.NewString()).Scan(&world); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE id=$1`, world) })
	if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, uuid.NewString()).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	mkCity := func(q int, stock, rate float64) uuid.UUID {
		t.Helper()
		var prov, sid uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,$2,0,'plains') RETURNING id`, world, q).Scan(&prov); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,state,population) VALUES($1,$2,'Paying city','achaean',$3,'direct','active',1000) RETURNING id`, world, prov, owner).Scan(&sid); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'silver',$2,$3,1000,8),($1,'grain',1000,0,1000,10)`, sid, stock, rate); err != nil {
			t.Fatal(err)
		}
		return sid
	}
	mkUnit := func(city uuid.UUID, typ, cat, status string, size int, location *uuid.UUID) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,status,size,settlement_id,support_settlement_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, world, owner, typ, cat, status, size, location, city).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if cat == "naval" {
			if _, err := pool.Exec(ctx, `UPDATE units SET provisions=100,provision_goods='{"grain":100}'::jsonb,crew=CASE type WHEN 'galley' THEN 20 WHEN 'war_galley' THEN 50 ELSE 10 END WHERE id=$1`, id); err != nil {
				t.Fatal(err)
			}
		}
		return id
	}
	a, b, c := mkCity(0, 100, 5), mkCity(4, 12, 0), mkCity(8, 20, 0)
	elite := mkUnit(a, "elite_infantry", "land", "garrison", 100, &a)
	spear := mkUnit(c, "spearman", "land", "garrison", 100, &c)
	ship := mkUnit(a, "galley", "naval", "garrison", 1, &a)
	cargo := mkUnit(a, "elite_infantry", "land", "embarked", 50, nil)
	_, err = pool.Exec(ctx, `UPDATE units SET cargo_unit_id=$1 WHERE id=$2`, cargo, ship)
	if err != nil {
		t.Fatal(err)
	}
	mkUnit(a, "war_galley", "naval", "repairing", 1, &a)
	mkUnit(a, "merchantman", "naval", "freighting", 1, nil)
	other := mkUnit(b, "elite_infantry", "land", "garrison", 100, &b)
	clk := clock.NewTestClock(time.Now())
	h := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	forecast := func(id uuid.UUID, ids string) map[string]any {
		t.Helper()
		got, err := h.marchSilverForecast(ctx, world, owner, id, ids)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	near := func(got any, want float64) {
		t.Helper()
		number, ok := got.(float64)
		if !ok || math.Abs(number-want) > 1e-8 {
			t.Fatalf("got %v, want %.9f", got, want)
		}
	}
	single := forecast(elite, "")
	near(single["upkeep_silver_home"], .75)
	near(single["upkeep_silver_field"], 1.5)
	// Remaining home galley .45, embarked elite 1, repair .75, freight 1.
	near(single["silver_runway_ticks"], 110/2.7)
	free := forecast(spear, "")
	near(free["upkeep_silver_home"], 0)
	near(free["upkeep_silver_field"], 0)
	if free["silver_runway_ticks"] != nil {
		t.Fatal("zero drain must have null runway")
	}
	// A free cohort at a city paying other troops still has a finite runway.
	_, err = pool.Exec(ctx, `UPDATE units SET support_settlement_id=$1,settlement_id=$1 WHERE id=$2`, a, spear)
	if err != nil {
		t.Fatal(err)
	}
	near(forecast(spear, "")["silver_runway_ticks"], 110/1.95)
	all := elite.String() + "," + ship.String() + "," + cargo.String() + "," + elite.String()
	batch := forecast(elite, all)
	near(batch["upkeep_silver_home"], 1.6)
	near(batch["upkeep_silver_field"], 2.45)
	near(batch["silver_runway_ticks"], 110/2.8)
	auto := forecast(elite, elite.String()+","+ship.String())
	near(auto["upkeep_silver_field"], 2.45)
	near(auto["silver_runway_ticks"], 110/2.8)
	multi := forecast(elite, all+","+other.String())
	near(multi["upkeep_silver_home"], 2.35)
	near(multi["upkeep_silver_field"], 3.95)
	near(multi["silver_runway_ticks"], 8)
	if multi["silver_payer_count"] != 2 {
		t.Fatal(multi)
	}
	if _, err := h.marchSilverForecast(ctx, world, owner, elite, uuid.NewString()); err == nil {
		t.Fatal("foreign/missing selection accepted")
	}
	if _, err := h.marchSilverForecast(ctx, world, owner, elite, "bad"); err == nil {
		t.Fatal("invalid selection accepted")
	}
	// Real tick, not a second arithmetic-only model: establish each starting
	// lazy stock at the same world tick, dispatch the chosen units, then bill.
	read := func(sid uuid.UUID) float64 {
		t.Helper()
		var amount float64
		if err := pool.QueryRow(ctx, `SELECT LEAST(cap,settled(amount,rate,calc_tick)) FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, sid).Scan(&amount); err != nil {
			t.Fatal(err)
		}
		return amount
	}
	beforeA, beforeB := read(a), read(b)
	_, err = pool.Exec(ctx, `UPDATE units SET status='marching',settlement_id=NULL WHERE id=ANY($1::uuid[])`, []uuid.UUID{elite, ship, other})
	if err != nil {
		t.Fatal(err)
	}
	upkeep := combat.NewUpkeepHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil, nil, events.NewScheduler(pool, clk).Clock())
	if err := upkeep.Handle(ctx, events.ScheduledEvent{WorldID: world, DueTick: 10}); err != nil {
		t.Fatal(err)
	}
	near(beforeA-read(a), 2.8)
	near(beforeB-read(b), 1.5)
	near(multi["silver_runway_ticks"], beforeB/(beforeB-read(b)))
	var grain float64
	if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'`, a).Scan(&grain); err != nil {
		t.Fatal(err)
	}
	near(grain, 1000-1.2-.5) // ships with provisions (including repair) and embarked troops eat onboard
	// Loss of the actual payer must never suggest infinite funds or capital fallback.
	_, err = pool.Exec(ctx, `UPDATE units SET support_settlement_id=NULL WHERE id=$1`, elite)
	if err != nil {
		t.Fatal(err)
	}
	near(forecast(elite, "")["silver_runway_ticks"], 0)
	_, err = pool.Exec(ctx, `INSERT INTO founder_phase(world_id,owner_id,population,grain_amount,grain_rate,silver_amount,silver_rate) VALUES($1,$2,1000,1200,-1,60,0)`, world, owner)
	if err != nil {
		t.Fatal(err)
	}
	refugee := forecast(elite, all)
	near(refugee["upkeep_silver_home"], 0)
	near(refugee["upkeep_silver_field"], 0)
	if refugee["silver_runway_ticks"] != nil {
		t.Fatal("founder exemption lost")
	}
}
