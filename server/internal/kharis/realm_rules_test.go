package kharis

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"sync/atomic"
	"testing"
)

func realmColony(t *testing.T, pool *pgxpool.Pool, wid, pid uuid.UUID, q int, state string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var prov, sid uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,$2,0,'plains') RETURNING id`, wid, q).Scan(&prov); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital,population,state) VALUES($1,$2,'Realm colony','achaean',$3,'colony',false,1000,$4) RETURNING id`, wid, prov, pid, state).Scan(&sid); err != nil {
		t.Fatal(err)
	}
	return sid
}

func TestRealmKharis_CountsTempledCitiesIncludingCapital(t *testing.T) {
	pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
	ctx := context.Background()
	var pid uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, sid).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 6; i++ {
		col := realmColony(t, pool, wid, pid, 10*i, "active")
		if _, err := pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES($1,'temple',1,'potnia')`, col); err != nil {
			t.Fatal(err)
		}
	}
	realmColony(t, pool, wid, pid, 70, "collapsed")
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=0,rate=0 WHERE settlement_id IN(SELECT id FROM settlements WHERE owner_id=$1)`, pid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_calc_tick,founded_tick) VALUES($1,$2,$3,40,0,current_world_tick(),current_world_tick())`, pid, wid, sid); err != nil {
		t.Fatal(err)
	}
	d, err := ProjectDecay(ctx, pool, pid, wid)
	if err != nil {
		t.Fatal(err)
	}
	if d.CityCount != 6 || d.Base != 0 || math.Abs(d.Cities-43.2) > 1e-10 || math.Abs(d.Residents) > 1e-10 {
		t.Fatalf("burden n%d base%g extra%g", d.CityCount, d.Base, d.Cities)
	}
	net, _, _, _, err := ProjectDailyNet(ctx, pool, pid, wid)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(net+43.2) > 1e-10 {
		t.Fatalf("projected%g", net)
	}
	h := newTestTickHandler(pool)
	if err := h.Handle(ctx, scheduledRealmTick(wid, atomic.AddInt64(&advanceOneDayEventID, 1))); err != nil {
		t.Fatal(err)
	}
	var amount float64
	if err := pool.QueryRow(ctx, `SELECT kharis_amount FROM player_world_records WHERE player_id=$1 AND world_id=$2`, pid, wid).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if math.Abs(amount-1) > 1e-10 {
		t.Fatalf("actual all-templed-six-city kharis%g want38.3", amount)
	}
}

func TestRealmPunishmentHitsEligibleColonyAndReplayIsProtected(t *testing.T) {
	pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
	ctx := context.Background()
	var pid uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, sid).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	colony := realmColony(t, pool, wid, pid, 10, "active")
	var foreign uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, uuid.NewString()).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	foreignCity := realmColony(t, pool, wid, foreign, 20, "active")
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=0,rate=0 WHERE settlement_id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	for _, city := range []uuid.UUID{colony, foreignCity} {
		if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',100,0,1000,current_world_tick())`, city); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_calc_tick,founded_tick) VALUES($1,$2,$3,14,0,current_world_tick(),current_world_tick())`, pid, wid, sid); err != nil {
		t.Fatal(err)
	}
	h := newTestTickHandler(pool)
	h.randomFloat = func() float64 { return 0 }
	eid := atomic.AddInt64(&advanceOneDayEventID, 1)
	snap := wanaxSnap{playerID: pid, settlementID: sid, kharis: 14, kharisCap: 50, templeCities: 0, cityCount: 2}
	if err := h.processMaintenance(ctx, snap, wid, eid); err != nil {
		t.Fatal(err)
	}
	if err := h.processMaintenance(ctx, snap, wid, eid); err != nil {
		t.Fatal(err)
	} // retry must not punish twice
	for _, tc := range []struct {
		city uuid.UUID
		want float64
	}{{colony, 50}, {foreignCity, 100}} {
		var amount float64
		if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'`, tc.city).Scan(&amount); err != nil {
			t.Fatal(err)
		}
		if amount != tc.want {
			t.Fatalf("city%s grain%g want%g", tc.city, amount, tc.want)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE event_type='DivinePunishment' AND world_id=$1 AND stream_id=$2 AND payload->>'type'='harvest_failure' AND (payload->>'amount')::float8=50`, wid, colony).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("colony punishment outcomes%d want1", count)
	}
	// The post-punishment shield, not a standing temple, protects this same tick.
	snap.templeCities = 1
	if err := h.processMaintenance(ctx, snap, wid, atomic.AddInt64(&advanceOneDayEventID, 1)); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE event_type='DivinePunishment' AND world_id=$1`, wid).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("shield did not protect: punishments%d want1", count)
	}
}
