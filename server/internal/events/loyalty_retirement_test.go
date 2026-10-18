package events

import (
	"context"
	"os"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"github.com/google/uuid"
)

var retiredLoyaltyTypes = []ScheduledEventType{ScheduledLoyaltyDecayTick, ScheduledLoyaltyWelfareTick, ScheduledColonyPenaltyTick, ScheduledBorrowedArmyTick}

func TestRetiredLoyaltyTicksCompleteWithoutEffects(t *testing.T) {
	pool := testWorkerPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`); err != nil {
		t.Fatal(err)
	}
	var wid uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick) VALUES('retired-loyalty','active',5) RETURNING id`).Scan(&wid); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE id=$1`, wid) })
	clk := clock.NewTestClock(time.Unix(0, 0))
	sched, worker := NewScheduler(pool, clk), NewWorker(pool, clk)
	for _, typ := range retiredLoyaltyTypes {
		// Invalid legacy payload deliberately requires no parsing or projection.
		if err := sched.EnqueueTick(ctx, wid, typ, map[string]string{"retired": "true"}, 5); err != nil {
			t.Fatal(err)
		}
		worker.Register(typ, RetiredLoyaltyTick)
	}
	for i := 0; i < 2; i++ {
		if err := worker.processBatch(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var total, done, failed, attempts, emitted int
	if err := pool.QueryRow(ctx, `SELECT count(*),count(processed_at),count(failed_at),sum(attempts) FROM scheduled_events WHERE world_id=$1`, wid).Scan(&total, &done, &failed, &attempts); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE world_id=$1`, wid).Scan(&emitted); err != nil {
		t.Fatal(err)
	}
	if total != 4 || done != 4 || failed != 0 || attempts != 4 || emitted != 0 {
		t.Fatalf("legacy work must complete once, silently: total%d done%d failed%d attempts%d emitted%d", total, done, failed, attempts, emitted)
	}
	for _, typ := range retiredLoyaltyTypes {
		if err := RetiredLoyaltyTick(ctx, ScheduledEvent{WorldID: wid, EventType: typ}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRemoveLoyaltyMigrationPreservesOtherStateAndAudit(t *testing.T) {
	pool := testWorkerPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	// Restore the exact preceding schema inside a transaction; leave the fully
	// migrated test database intact after this lifecycle proof.
	down, err := os.ReadFile("../../db/migrations/185_remove_loyalty.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`); err != nil {
		t.Fatal(err)
	}
	var wid uuid.UUID
	if err = tx.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick) VALUES('loyalty-migration','active',5) RETURNING id`).Scan(&wid); err != nil {
		t.Fatal(err)
	}
	var pid, sid uuid.UUID
	if err = tx.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,0,0,'plains') RETURNING id`, wid).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,state,loyalty,loyalty_points,loyalty_trend) VALUES($1,$2,'Keep city','achaean','active',4,90,'rising') RETURNING id`, wid, pid).Scan(&sid); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO loyalty_events(settlement_id,world_id,event_type,loyalty_delta,reason) VALUES($1,$2,'well_fed',1,'old')`, sid, wid); err != nil {
		t.Fatal(err)
	}
	if _, err = NewStore(pool).AppendTx(ctx, tx, sid, StreamProvince, "LoyaltyChanged", map[string]int{"new_level": 4}, wid, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'silver',41,2,100,3)`, sid); err != nil {
		t.Fatal(err)
	}
	for _, typ := range retiredLoyaltyTypes {
		if _, err = tx.Exec(ctx, `INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,failed_at,attempts) VALUES($1,$2,'{}',now(),6,now(),3)`, wid, string(typ)); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick) VALUES($1,$2,'{}',now(),99)`, wid, string(typ)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick) VALUES($1,'FoodTick','{}',now(),6)`, wid); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var s string
		if err := tx.QueryRow(ctx, `SELECT jsonb_build_object('city',to_jsonb(s)-'loyalty'-'loyalty_points'-'loyalty_trend','goods',(SELECT jsonb_agg(g) FROM settlement_goods g WHERE settlement_id=$1),'audit',(SELECT jsonb_agg(e) FROM events e WHERE world_id=$2))::text FROM settlements s WHERE id=$1`, sid, wid).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	before := snapshot()
	up, err := os.ReadFile("../../db/migrations/185_remove_loyalty.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(); after != before {
		t.Fatalf("migration changed city/goods/audit: before%s after%s", before, after)
	}
	var done, failed, food int
	if err = tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE event_type!='FoodTick' AND processed_at IS NOT NULL),count(failed_at),count(*) FILTER(WHERE event_type='FoodTick' AND processed_at IS NULL) FROM scheduled_events WHERE world_id=$1`, wid).Scan(&done, &failed, &food); err != nil {
		t.Fatal(err)
	}
	if done != 8 || failed != 0 || food != 1 {
		t.Fatalf("wrong retirement: done%d failed%d food%d", done, failed, food)
	}
	var table *string
	var columns int
	if err = tx.QueryRow(ctx, `SELECT to_regclass('loyalty_events')::text,(SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='settlements' AND column_name IN ('loyalty','loyalty_points','loyalty_trend'))`).Scan(&table, &columns); err != nil {
		t.Fatal(err)
	}
	if table != nil || columns != 0 {
		t.Fatalf("legacy schema remains: table%v columns%d", table, columns)
	}
}
