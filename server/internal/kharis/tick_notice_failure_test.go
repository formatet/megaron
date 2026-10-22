package kharis

import (
	"context"
	"math"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestTickNoticeFailureKeepsWorldRunning(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE worlds SET status='archived' WHERE status='active'`)
	wid := uuid.New()
	exec(`INSERT INTO worlds(id,name,status,current_tick) VALUES($1,$2,'active',10)`, wid, "notice-failure-"+wid.String())
	t.Cleanup(func() { pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE id=$1`, wid) })
	// A sorts first even though it is inserted last. Failure of the first Wanax
	// must neither skip the second nor kill the world's recurring queue.
	a := uuid.MustParse("00000000-0000-0000-0000-000000001401")
	b := uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffff1402")
	t.Cleanup(func() {
		// Claims have no player FK; these fixed test scopes must be reusable.
		pool.Exec(ctx, `DELETE FROM processed_tick_claims WHERE scope_id IN ($1,$2)`, a, b)
		pool.Exec(ctx, `DELETE FROM worlds WHERE id=$1`, wid)
		pool.Exec(ctx, `DELETE FROM players WHERE id IN ($1,$2)`, a, b)
	})
	city := map[uuid.UUID]uuid.UUID{}
	for i, owner := range []uuid.UUID{b, a} {
		exec(`INSERT INTO players(id,username,password_hash) VALUES($1,$2,'x')`, owner, "failure-"+owner.String())
		province, sid := uuid.New(), uuid.New()
		city[owner] = sid
		exec(`INSERT INTO provinces(id,world_id,map_q,map_r,terrain_type) VALUES($1,$2,$3,0,'plains')`, province, wid, i)
		exec(`INSERT INTO settlements(id,world_id,province_id,owner_id,name,culture_id,is_capital,population) VALUES($1,$2,$3,$4,'Failure city','achaean',true,100)`, sid, wid, province, owner)
		exec(`INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_calc_tick) VALUES($1,$2,$3,30,0,10)`, owner, wid, sid)
		exec(`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'fish',100,2,1000,10)`, sid)
	}
	// CHECK is scoped to A, so a global failure or swallowed transaction cannot
	// accidentally make B's notice look successful.
	exec(`ALTER TABLE notifications ADD CONSTRAINT test_notice_owner_failure CHECK(player_id <> '00000000-0000-0000-0000-000000001401'::uuid OR kind <> 'KharisBandChanged') NOT VALID`)
	t.Cleanup(func() {
		pool.Exec(ctx, `ALTER TABLE notifications DROP CONSTRAINT IF EXISTS test_notice_owner_failure`)
	})
	h := newTestTickHandler(pool)
	h.randomFloat = func() float64 { return 1 }
	first := events.ScheduledEvent{ID: 941400001, WorldID: wid, DueTick: 10}
	if err := h.Handle(ctx, first); err != nil {
		t.Fatalf("one owner's observation must not abort world tick: %v", err)
	}
	check := func(tick int, aWant, bWant int, amountWant float64) {
		t.Helper()
		var an, bn, next int
		var amount float64
		if err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE player_id=$2),count(*) FILTER(WHERE player_id=$3) FROM notifications WHERE world_id=$1 AND kind='KharisBandChanged'`, wid, a, b).Scan(&an, &bn); err != nil {
			t.Fatal(err)
		}
		if an != aWant || bn != bWant {
			t.Fatalf("notices A%d/B%d want%d/%d", an, bn, aWant, bWant)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='KharisTick' AND due_tick=$2 AND processed_at IS NULL AND failed_at IS NULL`, wid, tick+1).Scan(&next); err != nil {
			t.Fatal(err)
		}
		if next != 1 {
			t.Fatalf("next world tick queued=%d want1", next)
		}
		for _, owner := range []uuid.UUID{a, b} {
			if err := pool.QueryRow(ctx, `SELECT kharis_amount FROM player_world_records WHERE world_id=$1 AND player_id=$2`, wid, owner).Scan(&amount); err != nil {
				t.Fatal(err)
			}
			if math.Abs(amount-amountWant) > 1e-9 {
				t.Fatalf("owner%s maintenance=%g want%g", owner, amount, amountWant)
			}
		}
	}
	check(10, 0, 1, 29.89)
	exec(`ALTER TABLE notifications DROP CONSTRAINT test_notice_owner_failure`)
	exec(`UPDATE worlds SET current_tick=11 WHERE id=$1`, wid)
	if err := h.Handle(ctx, events.ScheduledEvent{ID: 941400002, WorldID: wid, DueTick: 11}); err != nil {
		t.Fatal(err)
	}
	check(11, 1, 1, 29.7799)
	// A's durable prior band survives the failed notice and describes the actual
	// transition on the next observation; B's continuing band stays silent.
	var from, to string
	var observed int
	if err := pool.QueryRow(ctx, `SELECT body_json->>'from_band',body_json->>'to_band',world_tick FROM notifications WHERE world_id=$1 AND player_id=$2 AND kind='KharisBandChanged'`, wid, a).Scan(&from, &to, &observed); err != nil {
		t.Fatal(err)
	}
	if from != "Indifferent" || to != "Suspicious" || observed != 11 {
		t.Fatalf("delayed A transition %s/%s tick%d", from, to, observed)
	}
	// A garrison observation failure also leaves the world's next tick alive.
	exec(`ALTER TABLE notifications ADD CONSTRAINT test_notice_garrison_failure CHECK(kind <> 'GarrisonStarvationStarted') NOT VALID`)
	t.Cleanup(func() {
		pool.Exec(ctx, `ALTER TABLE notifications DROP CONSTRAINT IF EXISTS test_notice_garrison_failure`)
	})
	exec(`UPDATE worlds SET current_tick=12 WHERE id=$1`, wid)
	if _, err := events.NewStore(pool).Append(ctx, uuid.New(), events.StreamCombat, "UnitAttrition", map[string]any{"reason": "grain_shortage", "garrison_settlement_id": city[a]}, wid, nil); err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(ctx, events.ScheduledEvent{ID: 941400003, WorldID: wid, DueTick: 12}); err != nil {
		t.Fatalf("garrison observation must not abort world tick: %v", err)
	}
	check(12, 1, 1, 29.6698)
}
