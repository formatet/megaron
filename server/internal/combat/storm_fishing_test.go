package combat

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/economy"
	"github.com/google/uuid"
)

// The weather step recomputes a fishing city when a storm comes onto or
// leaves its fishing hex, so the stored rate follows the storm (D.1).
func TestRecomputeStormFishing_FollowsTheStorm(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`); err != nil {
		t.Fatal(err)
	}
	var world, owner, prov, city uuid.UUID
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick) VALUES($1,'active',300) RETURNING id`, "stormfish-"+uuid.NewString()).Scan(&world))
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE id=$1`, world) })
	must(pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "fisher-"+uuid.NewString()).Scan(&owner))
	must(pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,0,0,'plains') RETURNING id`, world).Scan(&prov))
	_, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain,coastal) VALUES($1,0,0,'plains',true),($1,1,0,'coastal_sea',true)`, world)
	must(err)
	must(pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital,population) VALUES($1,$2,'Halia','achaean',$3,'capital',true,60) RETURNING id`, world, prov, owner).Scan(&city))
	_, err = pool.Exec(ctx, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES($1,1,'hex',1,0,'fish')`, city)
	must(err)
	must(economy.RecomputeProduction(ctx, pool, city))
	rate := func() float64 {
		var r float64
		must(pool.QueryRow(ctx, `SELECT rate FROM settlement_goods WHERE settlement_id=$1 AND good_key='fish'`, city).Scan(&r))
		return r
	}
	calm := rate()
	if calm <= 0 {
		t.Fatalf("calm rate %v", calm)
	}
	var storm uuid.UUID
	must(pool.QueryRow(ctx, `INSERT INTO sea_storms(world_id,heading,created_tick) VALUES($1,0,299) RETURNING id`, world).Scan(&storm))
	_, err = pool.Exec(ctx, `INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES($1,300,0,1,0)`, storm)
	must(err)
	step := func() {
		tx, err := pool.Begin(ctx)
		must(err)
		defer tx.Rollback(ctx)
		must(recomputeStormFishing(ctx, tx, world, 300))
		must(tx.Commit(ctx))
	}
	step()
	// What remains is the city's own shore catch, not a fishing place.
	if got := rate(); got >= calm {
		t.Fatalf("storm on the fishing hex: rate %v, want below calm %v", got, calm)
	}
	// Next tick the storm stands elsewhere; it stood here the tick before.
	_, err = pool.Exec(ctx, `UPDATE worlds SET current_tick=301 WHERE id=$1`, world)
	must(err)
	_, err = pool.Exec(ctx, `INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES($1,301,0,9,9)`, storm)
	must(err)
	tx, err := pool.Begin(ctx)
	must(err)
	must(recomputeStormFishing(ctx, tx, world, 301))
	must(tx.Commit(ctx))
	if got := rate(); got != calm {
		t.Fatalf("storm gone: rate %v, want calm %v", got, calm)
	}
}
