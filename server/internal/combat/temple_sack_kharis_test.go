package combat

import (
	"context"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"testing"
	"time"
)

func TestHostileSackLowersTempleWithoutKharisDebt(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSiegeFixture(t, pool, 0)
	if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,0,0,'plains'),($1,1,0,'plains')`, f.worldID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlements SET state='occupied',occupant_id=$2,occupied_since_tick=0,population=900 WHERE id=$1`, f.defSettlement, f.attacker); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES($1,'temple',3,'potnia')`, f.defSettlement); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_calc_tick) VALUES($1,$2,$3,80,0,current_world_tick()) ON CONFLICT(player_id,world_id) DO UPDATE SET kharis_amount=80,kharis_rate=0,kharis_calc_tick=current_world_tick()`, f.defender, f.worldID, f.defSettlement); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Now())
	_, err := ExecuteOccupyAction(ctx, pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk, &fakeBroadcaster{}, OccupyActionOrder{WorldID: f.worldID, PlayerID: f.attacker, SettlementID: f.defSettlement, Action: "sack"})
	if err != nil {
		t.Fatal(err)
	}
	var level int
	pool.QueryRow(ctx, `SELECT level FROM buildings WHERE settlement_id=$1 AND building_type='temple'`, f.defSettlement).Scan(&level)
	if level != 2 {
		t.Fatal("temple not downgraded", level)
	}
	var amount float64
	pool.QueryRow(ctx, `SELECT kharis_amount FROM player_world_records WHERE player_id=$1 AND world_id=$2`, f.defender, f.worldID).Scan(&amount)
	if amount != 80 {
		t.Fatal("sack charged victim", amount)
	}
	var n int
	pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind='TempleDemolitionWrath'`, f.worldID).Scan(&n)
	if n != 0 {
		t.Fatal("sack anger notice", n)
	}
}
