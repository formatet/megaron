package kharis

import (
	"context"
	"math"
	"sync/atomic"
	"testing"

	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

func TestL1TempleClimbsToFirstPrayerAndCaps(t *testing.T) {
	for _, level := range []int{1, 2, 3} {
		t.Run(string(rune('0'+level)), func(t *testing.T) {
			pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
			ctx := context.Background()
			var pid uuid.UUID
			if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, sid).Scan(&pid); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_calc_tick)
			 VALUES($1,$2,$3,$4,0,current_world_tick())`, pid, wid, sid, Floor); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE buildings SET level=$2 WHERE settlement_id=$1 AND building_type='temple'`, sid, level); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE settlement_labor SET weight=$2 WHERE settlement_id=$1 AND good_key='cult'`, sid, templeDevotionCapacity(level)); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=1000,rate=0,calc_tick=0 WHERE settlement_id=$1 AND good_key='grain'`, sid); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO divine_labor_valuations(world_id,good_key,rarity_spread,rarity_volume,divine_value,calc_tick) VALUES($1,'grain',0,0,1,0)`, wid); err != nil {
				t.Fatal(err)
			}
			cap := TempleKharisCeiling(level)
			net, _, dev, _, err := ProjectDailyNet(ctx, pool, pid, wid)
			if err != nil {
				t.Fatal(err)
			}
			if net <= 0 || (level == 1 && net > .5) {
				t.Fatalf("L%d ordinary net%g want weakly positive L1", level, net)
			}
			h := newTestTickHandler(pool)
			amount := Floor
			firstPrayer, firstCap := 0, 0
			for tick := 1; tick <= 180; tick++ {
				if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=$2 WHERE id=$1`, wid, tick); err != nil {
					t.Fatal(err)
				}
				w := wanaxSnap{playerID: pid, settlementID: sid, kharis: amount, kharisCap: cap, devotionSum: dev, templeCities: 1, maxTempleLevel: level}
				if err := h.processMaintenance(ctx, w, wid, atomic.AddInt64(&advanceOneDayEventID, 1)); err != nil {
					t.Fatal(err)
				}
				var next float64
				if err := pool.QueryRow(ctx, `SELECT kharis_amount FROM player_world_records WHERE player_id=$1 AND world_id=$2`, pid, wid).Scan(&next); err != nil {
					t.Fatal(err)
				}
				want := math.Min(cap, amount+net)
				if math.Abs(next-want) > 1e-8 {
					t.Fatalf("L%d tick%d actual%g projected%g", level, tick, next, want)
				}
				amount = next
				if firstPrayer == 0 && amount >= religion.MoodSuspicious {
					firstPrayer = tick
				}
				if firstCap == 0 && amount >= cap-1e-8 {
					firstCap = tick
				}
			}
			if level == 1 && (firstPrayer == 0 || firstPrayer > 15 || cap >= religion.MoodFavorable) {
				t.Fatalf("L1 firstPrayer%d cap%g", firstPrayer, cap)
			}
			if firstCap == 0 || math.Abs(amount-cap) > 1e-8 {
				t.Fatalf("L%d failed cap: tick%d amount%g cap%g", level, firstCap, amount, cap)
			}
			t.Logf("L%d net%+.3f first prayer tick%d cap%g reached tick%d; actual180ticks", level, net, firstPrayer, cap, firstCap)
			// An unfunded temple still earns zero gross and loses the same decay.
			if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=0,rate=0 WHERE settlement_id=$1`, sid); err != nil {
				t.Fatal(err)
			}
			net, _, _, _, err = ProjectDailyNet(ctx, pool, pid, wid)
			if err != nil {
				t.Fatal(err)
			}
			if net != -1.8 {
				t.Fatalf("L%d unfunded net%g changed baseline decay", level, net)
			}
		})
	}
}
