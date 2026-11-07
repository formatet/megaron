package kharis

// Potnia's fertility on the population tick (bönesystemet v3 steg 5a).

import (
	"context"
	"math"
	"sync/atomic"
	"testing"

	"formatet/megaron/server/internal/economy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fertilityRig struct {
	pool  *pgxpool.Pool
	world uuid.UUID
	city  uuid.UUID
	owner uuid.UUID
}

func newFertilityRig(t *testing.T, unmet float64) fertilityRig {
	t.Helper()
	terrains := [6]string{"plains", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone"}
	pool, worldID, sid := newGrowthFixture(t, terrains, 2000)
	ctx := context.Background()
	need := economy.GrainConsumptionPerTick(2000)
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
 VALUES ($1,'grain',10000,$2,1000000,current_world_tick()) ON CONFLICT (settlement_id, good_key) DO UPDATE SET rate=EXCLUDED.rate, amount=EXCLUDED.amount, cap=EXCLUDED.cap, calc_tick=EXCLUDED.calc_tick`, sid, need+5); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlements SET food_unmet_amount=$2 WHERE id=$1`, sid, unmet); err != nil {
		t.Fatal(err)
	}
	r := fertilityRig{pool: pool, world: worldID, city: sid}
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, sid).Scan(&r.owner); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r fertilityRig) bless(t *testing.T, owner uuid.UUID, scope string, city *uuid.UUID, from, until int) {
	t.Helper()
	if _, err := r.pool.Exec(context.Background(), `INSERT INTO prayer_blessings(world_id,owner_id,effect,scope,settlement_id,from_tick,until_tick) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		r.world, owner, economy.FertilityEffect, scope, city, from, until); err != nil {
		t.Fatal(err)
	}
}

// decay runs one population tick and returns the people gained (negative = lost).
func (r fertilityRig) decay(t *testing.T) int {
	t.Helper()
	ctx := context.Background()
	var before, after int
	_ = r.pool.QueryRow(ctx, `SELECT population FROM settlements WHERE id=$1`, r.city).Scan(&before)
	newTestTickHandler(r.pool).applyDecay(ctx, r.world, atomic.AddInt64(&advanceOneDayEventID, 1))
	_ = r.pool.QueryRow(ctx, `SELECT population FROM settlements WHERE id=$1`, r.city).Scan(&after)
	return after - before
}

func (r fertilityRig) setTick(t *testing.T, tick int) {
	t.Helper()
	if _, err := r.pool.Exec(context.Background(), `UPDATE worlds SET current_tick=$2 WHERE id=$1`, r.world, tick); err != nil {
		t.Fatal(err)
	}
}

func TestFertility_FedCityGrowsHalfAgainFasterThenNormalAfterwards(t *testing.T) {
	r := newFertilityRig(t, 0)
	// control: the same city before the prayer: 2000 * 0.5 % = 10
	if got := r.decay(t); got != 10 {
		t.Fatalf("control growth %d want 10", got)
	}
	// 2010 * 0.5 % * 1.5 = 15.075 -> 15
	r.bless(t, r.owner, "city", &r.city, 0, 20)
	if got := r.decay(t); got != 15 {
		t.Fatalf("blessed growth %d want 15", got)
	}
	// once until_tick has come the blessing is over (2025 * 0.5 % = 10.1 -> 10)
	r.setTick(t, 20)
	if got := r.decay(t); got != 10 {
		t.Fatalf("growth after the blessing %d want 10", got)
	}
}

func TestFertility_RealmCoversOwnCityNotAnotherWanaxsNorAnotherCity(t *testing.T) {
	r := newFertilityRig(t, 0)
	ctx := context.Background()
	var stranger, strangerProv, otherCity uuid.UUID
	if err := r.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "fert-s-"+uuid.NewString()).Scan(&stranger); err != nil {
		t.Fatal(err)
	}
	if err := r.pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,9,9,'plains') RETURNING id`, r.world).Scan(&strangerProv); err != nil {
		t.Fatal(err)
	}
	if err := r.pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,state,population) VALUES($1,$2,'Elsewhere','achaean',$3,'colony','active',100) RETURNING id`, r.world, strangerProv, r.owner).Scan(&otherCity); err != nil {
		t.Fatal(err)
	}
	r.bless(t, stranger, "realm", nil, 0, 20)      // somebody else's realm
	r.bless(t, r.owner, "city", &otherCity, 0, 20) // my other city
	if got := r.decay(t); got != 10 {
		t.Fatalf("growth %d want the plain 10", got)
	}
	r.bless(t, r.owner, "realm", nil, 0, 20)
	if got := r.decay(t); got != 15 { // 2010 * 0.0075 = 15.075
		t.Fatalf("realm-blessed growth %d want 15", got)
	}
}

func TestFertility_AStarvingCityStillShrinks(t *testing.T) {
	r := newFertilityRig(t, 50)
	r.bless(t, r.owner, "realm", nil, 0, 20)
	want := -int(2000 - math.Round(2000*(1-starvationPopLossRatePerTick)))
	if got := r.decay(t); got != want {
		t.Fatalf("starving blessed city changed by %d want %d (fertility is growth only)", got, want)
	}
}
