package kharis

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"testing"
)

func staffTestTemple(t *testing.T, pool *pgxpool.Pool, sid uuid.UUID, count int) {
	t.Helper()
	ctx := context.Background()
	// Fixtures explicitly reassign real people; never create additive labour.
	if _, err := pool.Exec(ctx, `DELETE FROM settlement_placement WHERE settlement_id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) SELECT $1,n,'building','temple','cult' FROM generate_series(1,$2::int) n`, sid, count); err != nil {
		t.Fatal(err)
	}
}

func TestTempleProjectionCountsOnlyRealWorkers(t *testing.T) {
	pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
	ctx := context.Background()
	var owner uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, sid).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO divine_labor_valuations(world_id,good_key,rarity_spread,rarity_volume,divine_value,calc_tick) VALUES($1,'grain',0,0,1,0)`, wid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=100,rate=0 WHERE settlement_id=$1 AND good_key='grain'`, sid); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{0, 1, 2, 1, 0} {
		staffTestTemple(t, pool, sid, n)
		net, has, workers, capacity, err := ProjectDailyNet(ctx, pool, owner, wid)
		want := float64(n)*KharisPerTempleWorker - computeDailyDecay(1)
		if err != nil || !has || workers != float64(n) || capacity != 2 || math.Abs(net-want) > 1e-8 {
			t.Fatalf("%d workers net%g want%g workers%g cap%g has%v err%v", n, net, want, workers, capacity, has, err)
		}
	}
}
