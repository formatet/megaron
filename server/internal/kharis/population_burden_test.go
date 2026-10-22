package kharis

import (
	"context"
	"github.com/google/uuid"
	"math"
	"sync/atomic"
	"testing"
)

// Different city populations must count as residents, not rounded workers or
// equal per-city weights. The full handler's SELECT and read projection agree.
func TestPopulationBurdenFullTickMatchesProjection(t *testing.T) {
	pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
	ctx := context.Background()
	var pid uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, sid).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	a := realmColony(t, pool, wid, pid, 20, "active")
	b := realmColony(t, pool, wid, pid, 40, "active")
	if _, err := pool.Exec(ctx, `UPDATE settlements SET population=CASE WHEN id=$1 THEN 1201 WHEN id=$2 THEN 1299 ELSE population END WHERE id IN($1,$2)`, a, b); err != nil {
		t.Fatal(err)
	}
	realmColony(t, pool, wid, pid, 60, "collapsed")
	realmColony(t, pool, wid, pid, 80, "razed")
	realmColony(t, pool, wid, pid, 100, "sunk")
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=0,rate=0 WHERE settlement_id IN(SELECT id FROM settlements WHERE owner_id=$1)`, pid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_calc_tick) VALUES($1,$2,$3,40,0,current_world_tick())`, pid, wid, sid); err != nil {
		t.Fatal(err)
	}
	net, _, _, _, err := ProjectDailyNet(ctx, pool, pid, wid)
	if err != nil {
		t.Fatal(err)
	}
	// 3 cities, exactly 3500 residents: .1 + .4 + .35 = .85.
	if math.Abs(net+.85) > 1e-9 {
		t.Errorf("population projection net%g want-.85", net)
	}
	d, err := ProjectDecay(ctx, pool, pid, wid)
	if err != nil {
		t.Fatal(err)
	}
	if d.CityCount != 3 || d.Population != 3500 || math.Abs(d.Residents-.35) > 1e-10 || math.Abs(d.Total-.85) > 1e-10 {
		t.Fatalf("bad breakdown %+v", d)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlements SET state='collapsed' WHERE id=$1`, b); err != nil {
		t.Fatal(err)
	}
	d, err = ProjectDecay(ctx, pool, pid, wid)
	if err != nil {
		t.Fatal(err)
	}
	if d.CityCount != 2 || d.Population != 2201 || math.Abs(d.Total-.5201) > 1e-10 {
		t.Fatalf("collapsed city still counted %+v", d)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlements SET state='active' WHERE id=$1`, b); err != nil {
		t.Fatal(err)
	}
	h := newTestTickHandler(pool)
	if err := h.Handle(ctx, scheduledRealmTick(wid, atomic.AddInt64(&advanceOneDayEventID, 1))); err != nil {
		t.Fatal(err)
	}
	var amount float64
	if err := pool.QueryRow(ctx, `SELECT kharis_amount FROM player_world_records WHERE player_id=$1 AND world_id=$2`, pid, wid).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if math.Abs(amount-39.15) > 1e-9 {
		t.Errorf("full tick kharis%g want39.15", amount)
	}
}
