package combat

import (
	"context"
	"fmt"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/kharis"
	"github.com/google/uuid"
	"testing"
	"time"
)

// Exercise the actual producer and consumer in the production tick order,
// including extinction of the final garrison. No synthetic loss event.
func TestStarvationSignal_ActualLossEvenLastGarrison(t *testing.T) {
	for _, size := range []int{100, 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			pool := testPool(t)
			ctx := context.Background()
			f := newSupportFixture(t, pool, "actual-loss")
			seedGoods(t, pool, f.capitalID, f.tick, 0, 10000)
			seedGoods(t, pool, f.townID, f.tick, 10000, 10000)
			// The civilian emergency herd cannot feed the army in order28a.
			if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,calc_tick) VALUES($1,'livestock',10000,0,$2) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=1000,calc_tick=$2`, f.capitalID, f.tick); err != nil {
				t.Fatal(err)
			}
			var fedUnit uuid.UUID
			if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,home_settlement_id,type,category,size,status) VALUES($1,$2,$3,$3,$3,'spearman','land',100,'garrison') RETURNING id`, f.worldID, f.owner, f.townID).Scan(&fedUnit); err != nil {
				t.Fatal(err)
			}
			var uid uuid.UUID
			if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,home_settlement_id,type,category,size,status) VALUES($1,$2,$3,$3,$3,'spearman','land',$4,'garrison') RETURNING id`, f.worldID, f.owner, f.capitalID, size).Scan(&uid); err != nil {
				t.Fatal(err)
			}
			sched := events.NewScheduler(pool, clock.NewTestClock(time.Unix(0, 0)))
			store := events.NewStore(pool)
			if events.TickPriority(events.ScheduledUpkeepTick) >= events.TickPriority(events.ScheduledKharisTick) {
				t.Fatal("upkeep must precede kharis")
			}
			if err := NewUpkeepHandler(pool, sched, store, nil, nil, sched.Clock()).Handle(ctx, events.ScheduledEvent{ID: 13009901, WorldID: f.worldID, DueTick: f.tick}); err != nil {
				t.Fatal(err)
			}
			var recorded string
			if err := pool.QueryRow(ctx, `SELECT payload->>'garrison_settlement_id' FROM events WHERE stream_id=$1 AND event_type='UnitAttrition'`, uid).Scan(&recorded); err != nil {
				t.Fatal(err)
			}
			if recorded != f.capitalID.String() {
				t.Fatalf("loss location %s want %s", recorded, f.capitalID)
			}
			if size == 1 {
				var status string
				if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, uid).Scan(&status); err != nil {
					t.Fatal(err)
				}
				if status != "disbanded" {
					t.Fatalf("last garrison status %s", status)
				}
				// The outcome must survive later cleanup/location changes as well.
				if _, err := pool.Exec(ctx, `UPDATE units SET settlement_id=NULL WHERE id=$1`, uid); err != nil {
					t.Fatal(err)
				}
			}
			if err := kharis.NewTickHandler(pool, sched, store, nil).Handle(ctx, events.ScheduledEvent{ID: 13009902, WorldID: f.worldID, DueTick: f.tick}); err != nil {
				t.Fatal(err)
			}
			var n int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='StarvationDamage' AND world_tick=current_world_tick()`, f.capitalID).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 1 {
				t.Fatalf("actual current-tick attrition must signal hunger, got %d", n)
			}
			var fedSize, fedLosses, fedAudits int
			if err := pool.QueryRow(ctx, `SELECT size,(SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='UnitAttrition'),(SELECT count(*) FROM events WHERE stream_id=$2 AND event_type='StarvationDamage' AND world_tick=current_world_tick()) FROM units WHERE id=$1`, fedUnit, f.townID).Scan(&fedSize, &fedLosses, &fedAudits); err != nil {
				t.Fatal(err)
			}
			if fedSize != 100 || fedLosses != 0 || fedAudits != 0 {
				t.Fatalf("fed city B must remain separate from hungry A: size%d losses%d StarvationDamage%d", fedSize, fedLosses, fedAudits)
			}
			kharisH := kharis.NewTickHandler(pool, sched, store, nil)
			if err := kharisH.Handle(ctx, events.ScheduledEvent{ID: 13009902, WorldID: f.worldID, DueTick: f.tick}); err != nil {
				t.Fatal(err)
			}
			var starts, wrongCity, day int
			if err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE body_json->>'settlement_id'=$2 AND kind='GarrisonStarvationStarted'),count(*) FILTER(WHERE body_json->>'settlement_id'=$3),min(world_tick) FROM notifications WHERE world_id=$1`, f.worldID, f.capitalID.String(), f.townID.String()).Scan(&starts, &wrongCity, &day); err != nil {
				t.Fatal(err)
			}
			if starts != 1 || wrongCity != 0 || day != f.tick {
				t.Fatalf("actual upkeep→kharis notices start%d wrongcity%d day%d", starts, wrongCity, day)
			}
			if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=$2 WHERE id=$1`, f.worldID, f.tick+1); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=10000,rate=0,calc_tick=$2 WHERE settlement_id=$1 AND good_key IN ('grain','silver')`, f.capitalID, f.tick+1); err != nil {
				t.Fatal(err)
			}
			if err := NewUpkeepHandler(pool, sched, store, nil, nil, sched.Clock()).Handle(ctx, events.ScheduledEvent{ID: 13009903, WorldID: f.worldID, DueTick: f.tick + 1}); err != nil {
				t.Fatal(err)
			}
			for retry := 0; retry < 2; retry++ {
				if err := kharisH.Handle(ctx, events.ScheduledEvent{ID: 13009904, WorldID: f.worldID, DueTick: f.tick + 1}); err != nil {
					t.Fatal(err)
				}
			}
			var ends int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind='GarrisonStarvationEnded' AND body_json->>'settlement_id'=$2 AND world_tick=$3`, f.worldID, f.capitalID.String(), f.tick+1).Scan(&ends); err != nil {
				t.Fatal(err)
			}
			if ends != 1 {
				t.Fatalf("actual recovered upkeep→kharis recovery notices%d want1", ends)
			}
		})
	}
}
