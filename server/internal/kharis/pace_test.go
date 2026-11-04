package kharis

import (
	"context"
	"math"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

// maintainTicks runs processMaintenance once per tick and returns the standing after each (index 0 = start).
func maintainTicks(t *testing.T, start float64, ticks, templeLevel int) []float64 {
	t.Helper()
	pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
	ctx := context.Background()
	var pid uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, sid).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_calc_tick,founded_tick)
	 VALUES($1,$2,$3,$4,0,current_world_tick(),current_world_tick())`, pid, wid, sid, start); err != nil {
		t.Fatal(err)
	}
	w := wanaxSnap{population: 1000, cityCount: 1, playerID: pid, settlementID: sid, kharisCap: EffectiveKharisCap(100, templeLevel)}
	if templeLevel == 0 {
		if _, err := pool.Exec(ctx, `DELETE FROM buildings WHERE settlement_id=$1 AND building_type='temple'`, sid); err != nil {
			t.Fatal(err)
		}
	} else {
		staffTestTemple(t, pool, sid, 2)
		if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=1000,rate=0,calc_tick=0 WHERE settlement_id=$1 AND good_key='grain'`, sid); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO divine_labor_valuations(world_id,good_key,rarity_spread,rarity_volume,divine_value,calc_tick) VALUES($1,'grain',0,0,1,0)`, wid); err != nil {
			t.Fatal(err)
		}
		_, _, dev, _, err := ProjectDailyNet(ctx, pool, pid, wid)
		if err != nil {
			t.Fatal(err)
		}
		w.devotionSum, w.templeCities, w.maxTempleLevel = dev, 1, templeLevel
	}
	h := newTestTickHandler(pool)
	out := []float64{start}
	for tick := 1; tick <= ticks; tick++ {
		if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=$2 WHERE id=$1`, wid, tick); err != nil {
			t.Fatal(err)
		}
		w.kharis = out[len(out)-1]
		if err := h.processMaintenance(ctx, w, wid, atomic.AddInt64(&advanceOneDayEventID, 1)); err != nil {
			t.Fatal(err)
		}
		var next float64
		if err := pool.QueryRow(ctx, `SELECT kharis_amount FROM player_world_records WHERE player_id=$1 AND world_id=$2`, pid, wid).Scan(&next); err != nil {
			t.Fatal(err)
		}
		out = append(out, next)
	}
	return out
}

// Timothy 2026-11-04: with one city and no temple, 40 -> 30 takes 30 game days, so a temple built
// at once (staffed after ~22 game days) is standing before the wrath threshold is crossed.
func TestKharisPace_NoTempleFortyToThirtyInThirtyDays(t *testing.T) {
	k := maintainTicks(t, 40, 31, 0)
	if k[29] < 30+1e-9 {
		t.Fatalf("tick 29: %g want above 30", k[29])
	}
	if math.Abs(k[30]-30) > 1e-9 {
		t.Fatalf("tick 30: %g want 30", k[30])
	}
	if k[31] >= 30 {
		t.Fatalf("tick 31: %g want below 30", k[31])
	}
}

// The climb is as slow as the fall: a fully staffed L1 temple nets 7/12 - 1/3 = 0.25 a game day.
func TestKharisPace_L1TempleThirtyToFiftyInEightyDays(t *testing.T) {
	if got := KharisPerTempleWeight - KharisPerDemand; math.Abs(got-0.25) > 1e-12 {
		t.Fatalf("L1 net %g want 0.25", got)
	}
	k := maintainTicks(t, 30, 80, 1)
	if k[79] >= 50-1e-9 {
		t.Fatalf("tick 79: %g reached the L1 ceiling early", k[79])
	}
	if math.Abs(k[80]-50) > 1e-9 {
		t.Fatalf("tick 80: %g want 50", k[80])
	}
}
