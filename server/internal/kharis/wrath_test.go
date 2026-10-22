package kharis

import (
	"context"
	"github.com/google/uuid"
	"math"
	"sync/atomic"
	"testing"
)

func TestWrathThresholdDoesNotDependOnTemple(t *testing.T) {
	for _, temple := range []bool{false, true} {
		for _, tc := range []struct{ k, want float64 }{{1, .03}, {14.999, .03}, {15, 0}, {29.9, 0}, {30, 0}} {
			if got := PunishmentProbability(tc.k, temple); got != tc.want {
				t.Errorf("k%g temple%v probability%g want%g", tc.k, temple, got, tc.want)
			}
		}
	}
}
func TestWrathProtectionLastsSevenTicksAndDoesNotExtendOnRetry(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	wid, sid, pid := starvationWarningFixture(t, 100, 0)
	h := newTestTickHandler(pool)
	for _, tc := range []struct {
		tick  int
		stock float64
	}{{10, 50}, {10, 50}, {16, 50}, {17, 25}} {
		if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=$2 WHERE id=$1`, wid, tc.tick); err != nil {
			t.Fatal(err)
		}
		h.applyDivinePunishment(ctx, sid, wid, pid)
		var got float64
		if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'`, sid).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if math.Abs(got-tc.stock) > 1e-9 {
			t.Errorf("tick%d stock%g want%g", tc.tick, got, tc.stock)
		}
	}
}
func TestWrathDeclineNoticeOnceAndNoPositiveNetWarning(t *testing.T) {
	pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
	ctx := context.Background()
	var pid uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, sid).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_calc_tick) VALUES($1,$2,$3,30.1,0,current_world_tick())`, pid, wid, sid); err != nil {
		t.Fatal(err)
	}
	h := newTestTickHandler(pool)
	h.randomFloat = func() float64 { return .99 }
	w := wanaxSnap{playerID: pid, settlementID: sid, kharis: 30.1, kharisCap: 50, cityCount: 1, population: 1000}
	if err := h.processMaintenance(ctx, w, wid, atomic.AddInt64(&advanceOneDayEventID, 1)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind='KharisDeclining'`, wid).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("decline notices %d want1", count)
	}
	var net float64
	var days int
	if err := pool.QueryRow(ctx, `SELECT (body_json->>'net_per_tick')::float8,(body_json->>'days_to_15')::int FROM notifications WHERE world_id=$1 AND kind='KharisDeclining'`, wid).Scan(&net, &days); err != nil {
		t.Fatal(err)
	}
	if math.Abs(net+.2) > 1e-9 || days != 75 {
		t.Fatalf("forecast net%g days%d", net, days)
	}

	if err := h.observeKharisBand(ctx, w, wid); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind='KharisDeclining'`, wid).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("retry duplicated warning")
	}
}

func TestWrathPunishmentFailureRollsBackLossShieldAndEvent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	wid, sid, pid := starvationWarningFixture(t, 100, 0)
	h := newTestTickHandler(pool)
	if _, err := pool.Exec(ctx, `ALTER TABLE notifications ADD CONSTRAINT reject_wrath_test CHECK(kind<>'DivinePunishment') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `ALTER TABLE notifications DROP CONSTRAINT IF EXISTS reject_wrath_test`) })
	if err := h.applyDivinePunishment(ctx, sid, wid, pid); err == nil {
		t.Fatal("failed notification accepted punishment")
	}
	var stock float64
	var count int
	if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'`, sid).Scan(&stock); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM events WHERE world_id=$1 AND event_type='DivinePunishment')+(SELECT count(*) FROM kharis_notice_state WHERE world_id=$1 AND condition='divine_punishment')`, wid).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if stock != 100 || count != 0 {
		t.Fatalf("partial punishment persisted stock%g records%d", stock, count)
	}
}
func TestWrathDeclinePositiveNetSuppressedAndRecoveryRearms(t *testing.T) {
	pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
	ctx := context.Background()
	var pid uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, sid).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_calc_tick) VALUES($1,$2,$3,40,0,current_world_tick())`, pid, wid, sid); err != nil {
		t.Fatal(err)
	}
	staffTestTemple(t, pool, sid, 2)
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=100,rate=0 WHERE settlement_id=$1 AND good_key='grain'`, sid); err != nil {
		t.Fatal(err)
	}
	h := newTestTickHandler(pool)
	w := wanaxSnap{playerID: pid, settlementID: sid, kharis: 40}
	if err := h.rememberInitialBand(ctx, w, wid); err != nil {
		t.Fatal(err)
	}
	for i, tc := range []struct {
		amount float64
		fed    bool
		want   int
	}{{29, true, 0}, {28, false, 0}, {31, false, 0}, {29, false, 1}, {28, false, 1}, {31, false, 1}, {29, false, 2}} {
		if !tc.fed {
			staffTestTemple(t, pool, sid, 0)
		}
		if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=$2 WHERE id=$1`, wid, i+1); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE player_world_records SET kharis_amount=$3,kharis_calc_tick=$4 WHERE player_id=$1 AND world_id=$2`, pid, wid, tc.amount, i+1); err != nil {
			t.Fatal(err)
		}
		if err := h.observeKharisBand(ctx, w, wid); err != nil {
			t.Fatal(err)
		}
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind='KharisDeclining'`, wid).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != tc.want {
			t.Fatalf("step%d warnings%d want%d", i, n, tc.want)
		}
	}
}
func TestWrathProtectedRealmNeverPunishedEvenWithTemple(t *testing.T) {
	pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
	ctx := context.Background()
	var pid uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, sid).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_calc_tick) VALUES($1,$2,$3,10,0,current_world_tick())`, pid, wid, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=100,rate=0 WHERE settlement_id=$1 AND good_key='grain'`, sid); err != nil {
		t.Fatal(err)
	}
	h := newTestTickHandler(pool)
	h.randomFloat = func() float64 { return 0 }
	for _, tc := range []struct {
		tick  int
		count int
	}{{0, 1}, {6, 1}, {7, 2}} {
		if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=$2 WHERE id=$1`, wid, tc.tick); err != nil {
			t.Fatal(err)
		}
		w := wanaxSnap{playerID: pid, settlementID: sid, kharis: 10, kharisCap: 50, cityCount: 1, population: 1000, templeCities: 1}
		if err := h.processMaintenance(ctx, w, wid, atomic.AddInt64(&advanceOneDayEventID, 1)); err != nil {
			t.Fatal(err)
		}
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE world_id=$1 AND event_type='DivinePunishment'`, wid).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != tc.count {
			t.Fatalf("templed realm tick%d punishments%d want%d", tc.tick, n, tc.count)
		}
		status, err := LoadWrathStatus(ctx, pool, pid, wid)
		if err != nil {
			t.Fatal(err)
		}
		if status.ProtectionUntil == nil || status.ProtectionRemainingDays <= 0 {
			t.Fatalf("shield expiry missing %+v", status)
		}
	}
}
