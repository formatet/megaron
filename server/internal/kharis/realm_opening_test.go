package kharis

import (
	"context"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"math"
	"sync/atomic"
	"testing"
)

func TestRealmKharis_NoGrace_ProjectionAndTick(t *testing.T) {
	pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
	ctx := context.Background()
	var pid uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, sid).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM buildings WHERE settlement_id=$1 AND building_type='temple'`, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_calc_tick,founded_tick) VALUES($1,$2,$3,40,0,current_world_tick(),current_world_tick())`, pid, wid, sid); err != nil {
		t.Fatal(err)
	}
	for _, age := range []int{0, 1, 143, 144, 1000} {
		if _, err := pool.Exec(ctx, `UPDATE player_world_records SET founded_tick=current_world_tick()-$3 WHERE player_id=$1 AND world_id=$2`, pid, wid, age); err != nil {
			t.Fatal(err)
		}
		net, has, _, _, err := ProjectDailyNet(ctx, pool, pid, wid)
		if err != nil {
			t.Fatal(err)
		}
		if has || math.Abs(net+.2) > 1e-10 {
			t.Fatalf("age%d net%g has%v", age, net, has)
		}
	}
	h := newTestTickHandler(pool)
	if err := h.Handle(ctx, scheduledRealmTick(wid, atomic.AddInt64(&advanceOneDayEventID, 1))); err != nil {
		t.Fatal(err)
	}
	var amount float64
	if err := pool.QueryRow(ctx, `SELECT kharis_amount FROM player_world_records WHERE player_id=$1 AND world_id=$2`, pid, wid).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if math.Abs(amount-39.8) > 1e-10 {
		t.Fatalf("actual first tick%g want39.8", amount)
	}
}

func scheduledRealmTick(wid uuid.UUID, id int64) events.ScheduledEvent {
	return events.ScheduledEvent{ID: id, WorldID: wid, DueTick: 1}
}
