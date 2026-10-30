package kharis

import (
	"context"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
)

type committedNoticeRecorder struct {
	t         *testing.T
	pool      *pgxpool.Pool
	delivered int
}

func (r *committedNoticeRecorder) NotifyPlayer(context.Context, uuid.UUID, uuid.UUID, string, int, any) error {
	r.t.Fatal("transition must never insert through live hub")
	return nil
}
func (r *committedNoticeRecorder) DeliverCommittedNotification(ctx context.Context, world, owner uuid.UUID, id, kind string, level int, body any) {
	r.t.Helper()
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE id=$1 AND world_id=$2 AND player_id=$3 AND kind=$4`, id, world, owner, kind).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	if n != 1 {
		r.t.Fatal("live delivery preceded durable commit")
	}
	r.delivered++
}

func TestGarrisonTransitionNoticeLifecycle(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	wid, sid, uid := starvationGarrisonFixture(t, 1)
	var owner uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, sid).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	h := newTestTickHandler(pool)
	recorder := &committedNoticeRecorder{t: t, pool: pool}
	h.hub = recorder
	_, err := events.NewStore(pool).Append(ctx, uid, events.StreamCombat, "UnitAttrition", map[string]any{"reason": "grain_shortage", "lost": 1, "garrison_settlement_id": sid}, wid, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The witness remains authoritative after even the last garrison disbands.
	if _, err = pool.Exec(ctx, `UPDATE units SET size=0,status='disbanded' WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	h.applyStarvation(ctx, wid)
	h.applyStarvation(ctx, wid)
	var n int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND player_id=$2 AND kind='GarrisonStarvationStarted'`, wid, owner).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("garrison transition notices=%d want1", n)
	}
	if _, err = pool.Exec(ctx, `UPDATE worlds SET current_tick=current_tick+1 WHERE id=$1`, wid); err != nil {
		t.Fatal(err)
	}
	h.applyStarvation(ctx, wid)
	h.applyStarvation(ctx, wid)
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND player_id=$2 AND kind='GarrisonStarvationEnded'`, wid, owner).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("garrison recovery notices=%d want1", n)
	}
	if recorder.delivered != 2 {
		t.Fatalf("postcommit deliveries%d want2", recorder.delivered)
	}
}

func TestGarrisonTransitionOwnerAndCityIsolation(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	wid, _, owner := starvationWarningFixture(t, 100, 24)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	other := uuid.New()
	exec(`INSERT INTO players(id,username,password_hash) VALUES($1,$2,'x')`, other, "notice-owner-"+other.String())
	hungry := uuid.MustParse("ffffffff-ffff-ffff-ffff-fffffffffff1")
	calm := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	for i, sid := range []uuid.UUID{hungry, calm} {
		pid := uuid.New()
		exec(`INSERT INTO provinces(id,world_id,map_q,map_r,terrain_type) VALUES($1,$2,$3,0,'plains')`, pid, wid, 30+i)
		exec(`INSERT INTO settlements(id,world_id,province_id,owner_id,name,culture_id,is_capital) VALUES($1,$2,$3,$4,$5,'achaean',false)`, sid, wid, pid, owner, []string{"Hungry army, fed people", "No army losses"}[i])
		exec(`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',$2,$3,1000,0)`, sid, []float64{100, 0}[i], []float64{24, -10}[i])
	}
	h := newTestTickHandler(pool)
	witness := func() {
		t.Helper()
		if _, err := events.NewStore(pool).Append(ctx, uuid.New(), events.StreamCombat, "UnitAttrition", map[string]any{"reason": "grain_shortage", "garrison_settlement_id": hungry}, wid, nil); err != nil {
			t.Fatal(err)
		}
	}
	observe := func() {
		t.Helper()
		if err := h.applyStarvation(ctx, wid); err != nil {
			t.Fatal(err)
		}
	}
	witness()
	observe()
	observe()
	exec(`UPDATE worlds SET current_tick=current_tick+1 WHERE id=$1`, wid)
	witness()
	observe()
	var starts, wrong int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE body_json->>'settlement_id'=$2),count(*) FILTER(WHERE body_json->>'settlement_id'=$3) FROM notifications WHERE world_id=$1 AND kind='GarrisonStarvationStarted'`, wid, hungry.String(), calm.String()).Scan(&starts, &wrong); err != nil {
		t.Fatal(err)
	}
	if starts != 1 || wrong != 0 {
		t.Fatalf("city-specific transitions starts%d calm%d", starts, wrong)
	}
	// Clearing the personal archive must not delete the game's transition memory.
	exec(`DELETE FROM notifications WHERE world_id=$1 AND player_id=$2`, wid, owner)
	exec(`UPDATE settlements SET owner_id=$2,name='Hidden new owner name' WHERE id=$1`, hungry, other)
	exec(`UPDATE worlds SET current_tick=current_tick+1 WHERE id=$1`, wid)
	witness()
	observe()
	var newOwner int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND player_id=$2 AND kind='GarrisonStarvationStarted'`, wid, other).Scan(&newOwner); err != nil {
		t.Fatal(err)
	}
	if newOwner != 1 {
		t.Fatalf("new owner starts%d want1", newOwner)
	}
	var formerName, formerReason string
	if err := pool.QueryRow(ctx, `SELECT body_json->>'settlement_name',body_json->>'reason' FROM notifications WHERE world_id=$1 AND player_id=$2 AND kind='GarrisonStarvationEnded'`, wid, owner).Scan(&formerName, &formerReason); err != nil {
		t.Fatal(err)
	}
	if formerName != "Hungry army, fed people" || formerReason != "no_longer_owned_or_active" {
		t.Fatalf("former owner context leaked/incorrect: %s %s", formerName, formerReason)
	}
	exec(`UPDATE worlds SET current_tick=current_tick+1 WHERE id=$1`, wid)
	observe()
	observe()
	var ended int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind='GarrisonStarvationEnded' AND player_id=$2`, wid, other).Scan(&ended); err != nil {
		t.Fatal(err)
	}
	if ended != 1 {
		t.Fatalf("new owner recovery%d want1", ended)
	}
	exec(`UPDATE worlds SET current_tick=current_tick+1 WHERE id=$1`, wid)
	witness()
	observe()
	exec(`UPDATE settlements SET state='collapsed' WHERE id=$1`, hungry)
	exec(`UPDATE worlds SET current_tick=current_tick+1 WHERE id=$1`, wid)
	observe()
	observe()
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND player_id=$2 AND kind='GarrisonStarvationEnded'`, wid, other).Scan(&ended); err != nil {
		t.Fatal(err)
	}
	if ended != 2 {
		t.Fatalf("collapsed scope must close once: ended%d want2", ended)
	}
	var badTick int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind LIKE 'GarrisonStarvation%' AND (world_tick IS DISTINCT FROM (body_json->>'world_tick')::int OR body_json->>'province_id' IS NULL OR body_json->>'q' IS NULL OR body_json->>'r' IS NULL)`, wid).Scan(&badTick); err != nil {
		t.Fatal(err)
	}
	if badTick != 0 {
		t.Fatalf("missing location/tick%d", badTick)
	}
}

func TestKharisBandNoticeBoundariesAndExternalChanges(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	wid, sid, owner := starvationWarningFixture(t, 100, 24)
	if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_calc_tick) VALUES($1,$2,$3,30,0,0)`, owner, wid, sid); err != nil {
		t.Fatal(err)
	}
	h := newTestTickHandler(pool)
	h.randomFloat = func() float64 { return 1 }
	w := wanaxSnap{playerID: owner, settlementID: sid, kharis: 30, kharisCap: 100, cityCount: 1}
	if err := h.processMaintenance(ctx, w, wid, 920000001); err != nil {
		t.Fatal(err)
	}
	var meaning string
	if err := pool.QueryRow(ctx, `SELECT body_json->'kharis_standing'->>'summary' FROM notifications WHERE world_id=$1 AND kind='KharisBandChanged'`, wid).Scan(&meaning); err != nil {
		t.Fatal(err)
	}
	if meaning == "" {
		t.Fatal("new band notice lacks saved standing")
	}
	// First observation reports the maintenance crossing, including at tick zero.
	var from, to string
	var level int
	if err := pool.QueryRow(ctx, `SELECT body_json->>'from_band',body_json->>'to_band',level FROM notifications WHERE world_id=$1 AND kind='KharisBandChanged'`, wid).Scan(&from, &to, &level); err != nil {
		t.Fatal(err)
	}
	if from != "Indifferent" || to != "Suspicious" || level != 2 {
		t.Fatalf("first crossing %s -> %s level%d", from, to, level)
	}
	if err := h.processMaintenance(ctx, w, wid, 920000001); err != nil {
		t.Fatal(err)
	}
	// Between-tick rites change the pool before the next snapshot is loaded.
	for i, tc := range []struct {
		amount   float64
		from, to string
		level    int
	}{{5, "Suspicious", "Suspicious", 2}, {4.999, "Suspicious", "Wrathful", 2}, {30, "Wrathful", "Indifferent", 3}, {60, "Indifferent", "Favorable", 3}, {59.999, "Favorable", "Indifferent", 3}, {28.899, "Indifferent", "Suspicious", 2}, {5, "Suspicious", "Suspicious", 2}, {4.999, "Suspicious", "Wrathful", 2}} {
		if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=$2 WHERE id=$1`, wid, i+1); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE player_world_records SET kharis_amount=$3,kharis_calc_tick=$4 WHERE world_id=$1 AND player_id=$2`, wid, owner, tc.amount, i+1); err != nil {
			t.Fatal(err)
		}
		w.kharis = tc.amount
		if err := h.observeKharisBand(ctx, w, wid); err != nil {
			t.Fatal(err)
		}
		if err := h.observeKharisBand(ctx, w, wid); err != nil {
			t.Fatal(err)
		}
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind='KharisBandChanged' AND world_tick=$2`, wid, i+1).Scan(&n); err != nil {
			t.Fatal(err)
		}
		want := 1
		if tc.from == tc.to {
			want = 0
		}
		if n != want {
			t.Fatalf("tick%d crossing %s -> %s notices%d want%d", i+1, tc.from, tc.to, n, want)
		}
		if want == 1 {
			if err := pool.QueryRow(ctx, `SELECT body_json->>'from_band',body_json->>'to_band',level FROM notifications WHERE world_id=$1 AND kind='KharisBandChanged' AND world_tick=$2`, wid, i+1).Scan(&from, &to, &level); err != nil {
				t.Fatal(err)
			}
			if from != tc.from || to != tc.to || level != tc.level {
				t.Fatalf("tick%d got%s/%s/%d", i+1, from, to, level)
			}
		}
	}
}

func TestTransitionNoticeFailureRollsBackAndRetries(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	wid, sid, uid := starvationGarrisonFixture(t, 1)
	h := newTestTickHandler(pool)
	h.randomFloat = func() float64 { return 1 }
	var owner uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, sid).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`ALTER TABLE notifications ADD CONSTRAINT test_transition_notice_failure CHECK(kind NOT IN ('GarrisonStarvationStarted','KharisBandChanged')) NOT VALID`)
	t.Cleanup(func() {
		pool.Exec(ctx, `ALTER TABLE notifications DROP CONSTRAINT IF EXISTS test_transition_notice_failure`)
	})
	if _, err := events.NewStore(pool).Append(ctx, uid, events.StreamCombat, "UnitAttrition", map[string]any{"reason": "grain_shortage", "garrison_settlement_id": sid}, wid, nil); err != nil {
		t.Fatal(err)
	}
	if err := h.applyStarvation(ctx, wid); err == nil {
		t.Fatal("garrison notice persistence failure must reach worker retry")
	}
	var state, audit int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM kharis_notice_state WHERE world_id=$1),(SELECT count(*) FROM events WHERE world_id=$1 AND event_type='GarrisonStarvationStarted')`, wid).Scan(&state, &audit); err != nil {
		t.Fatal(err)
	}
	if state != 0 || audit != 0 {
		t.Fatalf("failed notice leaked state%d audit%d", state, audit)
	}
	exec(`INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_calc_tick) VALUES($1,$2,$3,30,0,0)`, owner, wid, sid)
	w := wanaxSnap{playerID: owner, settlementID: sid, kharis: 30, kharisCap: 100, cityCount: 1}
	if err := h.processMaintenance(ctx, w, wid, 920000002); err == nil {
		t.Fatal("band notice persistence failure must reach worker retry")
	}
	var baseline string
	var amount float64
	if err := pool.QueryRow(ctx, `SELECT state FROM kharis_notice_state WHERE world_id=$1 AND condition='kharis_band'`, wid).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	if baseline != "Indifferent" {
		t.Fatalf("failed notice consumed prior band: %s", baseline)
	}
	if err := pool.QueryRow(ctx, `SELECT kharis_amount FROM player_world_records WHERE world_id=$1 AND player_id=$2`, wid, owner).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if amount != 28.8 {
		t.Fatalf("maintenance amount%g want28.8", amount)
	}
	exec(`ALTER TABLE notifications DROP CONSTRAINT test_transition_notice_failure`)
	for i := 0; i < 2; i++ {
		if err := h.applyStarvation(ctx, wid); err != nil {
			t.Fatal(err)
		}
		w.kharis = amount
		if err := h.processMaintenance(ctx, w, wid, 920000002); err != nil {
			t.Fatal(err)
		}
	}
	var starts, bands int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE kind='GarrisonStarvationStarted'),count(*) FILTER(WHERE kind='KharisBandChanged') FROM notifications WHERE world_id=$1`, wid).Scan(&starts, &bands); err != nil {
		t.Fatal(err)
	}
	if starts != 1 || bands != 1 {
		t.Fatalf("retry notices starts%d bands%d want1each", starts, bands)
	}
	if err := pool.QueryRow(ctx, `SELECT kharis_amount FROM player_world_records WHERE world_id=$1 AND player_id=$2`, wid, owner).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if amount != 28.8 {
		t.Fatalf("retry charged maintenance twice: %g", amount)
	}
	// A stale observation cannot reset a newer condition or create a notice.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	n, err := h.recordTransition(ctx, tx, wid, owner, sid, "garrison_starvation", "false", "false", -1, "GarrisonStarvationEnded", 3, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if n != nil {
		t.Fatal("stale transition emitted notice")
	}
}
