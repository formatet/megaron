package kharis

import (
	"context"
	"fmt"
	"testing"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

type civilianNoticeRecorder struct{ *committedNoticeRecorder }

func (*civilianNoticeRecorder) BroadcastEvent(uuid.UUID, string, any) {}

func TestCivilianHungerSeventyTicksAuditAndTransitions(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	wid, sid, owner := starvationWarningFixture(t, 0, 0)
	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	mustExec(`UPDATE settlements SET population=1000 WHERE id=$1`, sid)
	// Historical inbox rows remain byte-for-byte intact.
	var legacy uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO notifications(world_id,player_id,kind,level,body_json,world_tick) VALUES($1,$2,'FoodShortfall',2,'{"name":"historic","unmet":9}',0) RETURNING id`, wid, owner).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	recorder := &civilianNoticeRecorder{&committedNoticeRecorder{t: t, pool: pool}}
	for i := 0; i < 70; i++ {
		tick := i + 1
		hungry := i < 10 || (i >= 15 && i < 40) || (i >= 45 && i < 65)
		stock := 10.0
		if hungry {
			stock = 1
		}
		mustExec(`UPDATE worlds SET current_tick=$2 WHERE id=$1`, wid, tick)
		mustExec(`UPDATE settlement_goods SET amount=0,rate=0,calc_tick=$2 WHERE settlement_id=$1`, sid, tick)
		mustExec(`UPDATE settlement_goods SET amount=$2 WHERE settlement_id=$1 AND good_key='grain'`, sid, stock)
		event := events.ScheduledEvent{ID: int64(260000 + i), WorldID: wid, DueTick: tick}
		// New handler instances and exact scheduled-event retries must not consume
		// twice, duplicate daily audit, or make notification state depend on a hub.
		for attempt := 0; attempt < 2; attempt++ {
			food := economy.NewFoodTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), recorder)
			if err := food.Handle(ctx, event); err != nil {
				t.Fatal(err)
			}
			h := newTestTickHandler(pool)
			h.hub = recorder
			if err := h.applyStarvation(ctx, wid); err != nil {
				t.Fatal(err)
			}
		}
	}
	var audits, starts, ends, historical int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE world_id=$1 AND event_type='FoodShortfall'`, wid).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE kind='CivilianStarvationStarted'),count(*) FILTER(WHERE kind='CivilianStarvationEnded'),count(*) FILTER(WHERE kind='FoodShortfall') FROM notifications WHERE world_id=$1`, wid).Scan(&starts, &ends, &historical); err != nil {
		t.Fatal(err)
	}
	if audits != 55 || starts != 3 || ends != 3 || historical != 1 || recorder.delivered != 6 {
		t.Fatalf("70-tick outcome audit=%d start=%d end=%d old=%d delivery=%d; want55/3/3/1/6", audits, starts, ends, historical, recorder.delivered)
	}
	var name string
	var unmet float64
	var tick int
	if err := pool.QueryRow(ctx, `SELECT body_json->>'name',(body_json->>'unmet')::float8,world_tick FROM notifications WHERE world_id=$1 AND kind='CivilianStarvationStarted' ORDER BY world_tick LIMIT 1`, wid).Scan(&name, &unmet, &tick); err != nil {
		t.Fatal(err)
	}
	if name != "Starveton" || unmet != 4 || tick != 1 {
		t.Fatalf("onset context %s %g %d", name, unmet, tick)
	}
	var unchanged bool
	if err := pool.QueryRow(ctx, `SELECT kind='FoodShortfall' AND body_json='{"name":"historic","unmet":9}'::jsonb AND world_tick=0 FROM notifications WHERE id=$1`, legacy).Scan(&unchanged); err != nil {
		t.Fatal(err)
	}
	if !unchanged {
		t.Fatal("historical row changed")
	}
}

func TestCivilianHungerNoticeFailureRetryKeepsFoodAndWorldRunning(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	wid, sid, owner := starvationWarningFixture(t, 1, 0)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE settlements SET population=1000 WHERE id=$1`, sid)
	exec(`UPDATE worlds SET current_tick=1 WHERE id=$1`, wid)
	food := newTestFoodTickHandler(pool)
	event := events.ScheduledEvent{ID: 261001, WorldID: wid, DueTick: 1}
	if err := food.Handle(ctx, event); err != nil {
		t.Fatal(err)
	}
	constraint := "civil_notice_" + fmt.Sprintf("%x", wid[:4])
	exec(fmt.Sprintf(`ALTER TABLE notifications ADD CONSTRAINT %s CHECK (world_id<>'%s' OR kind<>'CivilianStarvationStarted')`, constraint, wid))
	t.Cleanup(func() { pool.Exec(ctx, `ALTER TABLE notifications DROP CONSTRAINT IF EXISTS `+constraint) })
	h := newTestTickHandler(pool)
	if err := h.Handle(ctx, events.ScheduledEvent{ID: 261002, WorldID: wid, DueTick: 1}); err != nil {
		t.Fatalf("notice fault stopped world: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='KharisTick' AND due_tick=2`, wid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("next world tick count=%d", n)
	}
	var unmet float64
	if err := pool.QueryRow(ctx, `SELECT food_unmet_amount FROM settlements WHERE id=$1`, sid).Scan(&unmet); err != nil {
		t.Fatal(err)
	}
	if unmet != 4 {
		t.Fatalf("notice failure altered food debit: %g", unmet)
	}
	exec(`ALTER TABLE notifications DROP CONSTRAINT ` + constraint)
	for i := 0; i < 2; i++ {
		if err := newTestTickHandler(pool).applyStarvation(ctx, wid); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND player_id=$2 AND kind='CivilianStarvationStarted'`, wid, owner).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("retried onset count=%d", n)
	}
}

func TestCivilianHungerArchiveDeletionAndOwnerIsolation(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	wid, sid, owner := starvationWarningFixture(t, 0, 0)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	observe := func() {
		t.Helper()
		if err := newTestTickHandler(pool).applyStarvation(ctx, wid); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE settlements SET food_unmet_amount=4 WHERE id=$1`, sid)
	observe()
	exec(`DELETE FROM notifications WHERE world_id=$1`, wid)
	exec(`UPDATE worlds SET current_tick=1 WHERE id=$1`, wid)
	observe()
	var n int
	pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1`, wid).Scan(&n)
	if n != 0 {
		t.Fatal("archive clearing caused repeated hunger onset")
	}
	other := uuid.New()
	exec(`INSERT INTO players(id,username,password_hash) VALUES($1,$2,'x')`, other, "civil-owner-"+other.String())
	exec(`UPDATE settlements SET owner_id=$2,name='Secret new city' WHERE id=$1`, sid, other)
	exec(`UPDATE worlds SET current_tick=2 WHERE id=$1`, wid)
	observe()
	var oldName, reason string
	if err := pool.QueryRow(ctx, `SELECT body_json->>'name',body_json->>'reason' FROM notifications WHERE world_id=$1 AND player_id=$2 AND kind='CivilianStarvationEnded'`, wid, owner).Scan(&oldName, &reason); err != nil {
		t.Fatal(err)
	}
	if oldName != "Starveton" || reason != "no_longer_owned_or_active" {
		t.Fatalf("former owner context %s/%s", oldName, reason)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND player_id=$2 AND kind='CivilianStarvationStarted'`, wid, other).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("new owner starts=%d", n)
	}
}

func TestCivilianFishMealRecoveryLeavesGarrisonHungry(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	wid, sid, _ := starvationWarningFixture(t, 0, 0)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE settlements SET population=1000 WHERE id=$1`, sid)
	for tick := 1; tick <= 2; tick++ {
		exec(`UPDATE worlds SET current_tick=$2 WHERE id=$1`, wid, tick)
		if tick == 2 {
			exec(`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'fish',5,0,1000,2)`, sid)
		}
		if err := newTestFoodTickHandler(pool).Handle(ctx, events.ScheduledEvent{ID: int64(262000 + tick), WorldID: wid, DueTick: tick}); err != nil {
			t.Fatal(err)
		}
		if _, err := events.NewStore(pool).Append(ctx, uuid.New(), events.StreamCombat, "UnitAttrition", map[string]any{"reason": "grain_shortage", "garrison_settlement_id": sid}, wid, nil); err != nil {
			t.Fatal(err)
		}
		if err := newTestTickHandler(pool).applyStarvation(ctx, wid); err != nil {
			t.Fatal(err)
		}
	}
	var civilEnd, armyEnd int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE kind='CivilianStarvationEnded'),count(*) FILTER(WHERE kind='GarrisonStarvationEnded') FROM notifications WHERE world_id=$1`, wid).Scan(&civilEnd, &armyEnd); err != nil {
		t.Fatal(err)
	}
	if civilEnd != 1 || armyEnd != 0 {
		t.Fatalf("fish-fed people/army recovery=%d/%d want1/0", civilEnd, armyEnd)
	}
}

// A fully fed meal really ends a period: alternating meals are not one long
// period. This records the contract's inbox cost without inventing hysteresis.
func TestCivilianHungerAlternatingMealsAreSeparatePeriods(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	wid, sid, _ := starvationWarningFixture(t, 0, 0)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE settlements SET population=1000 WHERE id=$1`, sid)
	for tick := 1; tick <= 70; tick++ {
		exec(`UPDATE worlds SET current_tick=$2 WHERE id=$1`, wid, tick)
		stock := 10.0
		if tick%2 == 1 {
			stock = 1
		}
		exec(`UPDATE settlement_goods SET amount=$2,rate=0,calc_tick=$3 WHERE settlement_id=$1 AND good_key='grain'`, sid, stock, tick)
		if err := newTestFoodTickHandler(pool).Handle(ctx, events.ScheduledEvent{ID: int64(263000 + tick), WorldID: wid, DueTick: tick}); err != nil {
			t.Fatal(err)
		}
		if err := newTestTickHandler(pool).applyStarvation(ctx, wid); err != nil {
			t.Fatal(err)
		}
	}
	var audits, starts, ends int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE world_id=$1 AND event_type='FoodShortfall'`, wid).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE kind='CivilianStarvationStarted'),count(*) FILTER(WHERE kind='CivilianStarvationEnded') FROM notifications WHERE world_id=$1`, wid).Scan(&starts, &ends); err != nil {
		t.Fatal(err)
	}
	if audits != 35 || starts != 35 || ends != 35 {
		t.Fatalf("alternating70 audit/start/end=%d/%d/%d want35/35/35", audits, starts, ends)
	}
}
