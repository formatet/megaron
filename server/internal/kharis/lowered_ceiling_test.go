package kharis

import (
	"context"
	"github.com/google/uuid"
	"math"
	"sync/atomic"
	"testing"
)

func TestLoweredCeilingDriftsWithoutCliffAndForecastMatches(t *testing.T) {
	for _, tc := range []struct {
		start float64
		staff int
		want  float64
	}{{80, 2, 79.8}, {80, 0, 79.8}, {50.1, 2, 49.9}, {49, 2, 50}} {
		pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
		ctx := context.Background()
		var pid uuid.UUID
		if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, sid).Scan(&pid); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_calc_tick) VALUES($1,$2,$3,$4,0,current_world_tick())`, pid, wid, sid, tc.start); err != nil {
			t.Fatal(err)
		}
		staffTestTemple(t, pool, sid, tc.staff)
		if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=10000,rate=0,calc_tick=current_world_tick() WHERE settlement_id=$1 AND good_key='grain'`, sid); err != nil {
			t.Fatal(err)
		}
		net, _, dev, _, err := ProjectDailyNet(ctx, pool, pid, wid)
		if err != nil {
			t.Fatal(err)
		}
		if tc.start > 50 && math.Abs(net+.2) > 1e-9 {
			t.Errorf("above ceiling forecast %g want-.2", net)
		}
		h := newTestTickHandler(pool)
		w := wanaxSnap{playerID: pid, settlementID: sid, kharis: tc.start, kharisCap: 50, cityCount: 1, population: 1000, templeCities: 1, maxTempleLevel: 1, devotionSum: dev}
		if err := h.processMaintenance(ctx, w, wid, atomic.AddInt64(&advanceOneDayEventID, 1)); err != nil {
			t.Fatal(err)
		}
		var got float64
		if err := pool.QueryRow(ctx, `SELECT kharis_amount FROM player_world_records WHERE player_id=$1 AND world_id=$2`, pid, wid).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("start%g staff%d standing%g want%g", tc.start, tc.staff, got, tc.want)
		}

		var recorded float64
		if err := pool.QueryRow(ctx, `SELECT (payload->>'applied_net')::float8 FROM events WHERE world_id=$1 AND stream_id=$2 AND event_type='KharisMaintained' ORDER BY id DESC LIMIT 1`, wid, sid).Scan(&recorded); err != nil {
			t.Fatal(err)
		}
		if math.Abs(recorded-(got-tc.start)) > 1e-9 {
			t.Fatal("audit does not record applied outcome", recorded, got-tc.start)
		}
		// Replay is still exactly once even while standing above the new ceiling.
		if err := h.processMaintenance(ctx, w, wid, advanceOneDayEventID); err != nil {
			t.Fatal(err)
		}
		var after float64
		pool.QueryRow(ctx, `SELECT kharis_amount FROM player_world_records WHERE player_id=$1 AND world_id=$2`, pid, wid).Scan(&after)
		if after != got {
			t.Fatal("replay changed standing")
		}
	}
}
