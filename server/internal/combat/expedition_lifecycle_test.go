package combat

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
)

// nextArrival advances the real clock, retaining the exact event for retry.
func (f expeditionFixture) nextArrival(t *testing.T) events.ScheduledEvent {
	t.Helper()
	var tick int
	if err := testPool(t).QueryRow(context.Background(), `SELECT arrive_tick FROM units WHERE id=$1`, f.unitID).Scan(&tick); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool(t).Exec(context.Background(), `UPDATE worlds SET current_tick=$2 WHERE id=$1`, f.worldID, tick); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(unit.ScheduledUnitArrivalPayload{UnitID: f.unitID, WorldID: f.worldID, ArriveTick: &tick})
	return events.ScheduledEvent{WorldID: f.worldID, Payload: raw}
}
func (f expeditionFixture) untilReturning(t *testing.T) {
	t.Helper()
	for i := 0; i < 30; i++ {
		var intent string
		if err := testPool(t).QueryRow(context.Background(), `SELECT march_intent FROM units WHERE id=$1`, f.unitID).Scan(&intent); err != nil {
			t.Fatal(err)
		}
		if intent == "explore_return" {
			return
		}
		if err := f.h.Handle(context.Background(), f.nextArrival(t)); err != nil {
			t.Fatal(err)
		}
	}
	t.Fatal("never turned home")
}

func TestExpeditionLifecycle_AtomicReportRetry(t *testing.T) {
	f := newExpeditionFixture(t, -2, 16, nil)
	f.start(t, 9, 0, 12)
	f.untilReturning(t)
	ctx := context.Background()
	pool := testPool(t)
	event := f.nextArrival(t)
	// Force archive failure AFTER event writes and re-garrisoning. All must roll back.
	name := "expedition_archive_" + fmt.Sprintf("%x", f.unitID[:])
	if _, err := pool.Exec(ctx, `ALTER TABLE notifications ADD CONSTRAINT `+name+` CHECK (player_id <> '`+f.ownerID.String()+`' OR kind <> 'ExpeditionReport') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `ALTER TABLE notifications DROP CONSTRAINT IF EXISTS `+name) })
	if err := f.h.Handle(ctx, event); err == nil {
		t.Fatal("archive failure did not fail arrival")
	}
	if n := len(f.eventPayloads(t, unit.EventExpeditionReport)); n != 0 {
		t.Fatalf("rolled-back report events=%d", n)
	}
	if n := len(f.eventPayloads(t, unit.EventUnitArrived)); n != 0 {
		t.Fatalf("rolled-back arrived events=%d", n)
	}
	var status string
	var rows int
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, f.unitID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "marching" {
		t.Fatalf("failed transaction changed status to %s", status)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM unit_expeditions WHERE unit_id=$1`, f.unitID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatal("failed transaction removed expedition")
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE notifications DROP CONSTRAINT `+name); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := f.h.Handle(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(f.eventPayloads(t, unit.EventExpeditionReport)); n != 1 {
		t.Fatalf("retry report events=%d", n)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE player_id=$1 AND kind='ExpeditionReport'`, f.ownerID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("retry archive reports=%d", rows)
	}
}

func TestExpeditionLifecycle_HomeLost(t *testing.T) {
	for _, when := range []string{"outbound", "homeward", "missing_home", "no_fallback"} {
		t.Run(when, func(t *testing.T) {
			f := newExpeditionFixture(t, -2, 16, nil)
			f.start(t, 9, 0, 12)
			if when == "homeward" {
				f.untilReturning(t)
			}
			pool := testPool(t)
			ctx := context.Background()
			var foreign uuid.UUID
			if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "foreign-"+uuid.NewString()).Scan(&foreign); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1`, f.capitalID, foreign); err != nil {
				t.Fatal(err)
			}
			if when == "missing_home" {
				if _, err := pool.Exec(ctx, `UPDATE units SET home_settlement_id=NULL WHERE id=$1`, f.unitID); err != nil {
					t.Fatal(err)
				}
			}
			var fallback uuid.UUID
			if when != "no_fallback" {
				var province uuid.UUID
				if err := pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,-1,0,'plains') RETURNING id`, f.worldID).Scan(&province); err != nil {
					t.Fatal(err)
				}
				if err := pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type) VALUES($1,$2,'Fallback','achaean',$3,'colony') RETURNING id`, f.worldID, province, f.ownerID).Scan(&fallback); err != nil {
					t.Fatal(err)
				}
				f.runUntilHome(t)
			} else {
				if err := f.h.Handle(ctx, f.nextArrival(t)); err != nil {
					t.Fatal(err)
				}
			}
			var status string
			var sid *uuid.UUID
			var rows int
			if err := pool.QueryRow(ctx, `SELECT status,settlement_id FROM units WHERE id=$1`, f.unitID).Scan(&status, &sid); err != nil {
				t.Fatal(err)
			}
			if when == "no_fallback" {
				if status != "positioned" || sid != nil {
					t.Fatalf("without home status=%s settlement=%v", status, sid)
				}
			} else if status != "garrison" || sid == nil || *sid != fallback {
				t.Fatalf("fallback status=%s settlement=%v", status, sid)
			}
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM unit_expeditions WHERE unit_id=$1`, f.unitID).Scan(&rows); err != nil {
				t.Fatal(err)
			}
			if rows != 0 {
				t.Fatal("completed/stopped expedition retained state")
			}
		})
	}
}

func TestExpeditionLifecycle_SameTickRedirectCancels(t *testing.T) {
	f := newExpeditionFixture(t, -2, 16, nil)
	res := f.start(t, 9, 0, 12)
	q, r := res.TargetQ, res.TargetR
	if _, err := ExecuteRecall(context.Background(), testPool(t), f.scheduler, f.eventStore, f.clk, RecallOrder{WorldID: f.worldID, UnitID: f.unitID, Mode: "redirect", NewTargetQ: &q, NewTargetR: &r}); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := testPool(t).QueryRow(context.Background(), `SELECT count(*) FROM unit_expeditions WHERE unit_id=$1`, f.unitID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatal("same-target redirect retained expedition and can resume steering")
	}
	f.runUntilHome(t)
	if len(f.eventPayloads(t, unit.EventExpeditionReport)) != 0 {
		t.Fatal("cancelled expedition reported completion")
	}
}

func TestExpeditionLifecycle_ReservesAsymmetricHomeRoute(t *testing.T) {
	f := newExpeditionFixture(t, 0, 16, nil)
	ctx := context.Background()
	pool := testPool(t)
	// Narrow plains corridor; entering the cedar home costs more than leaving it.
	if _, err := pool.Exec(ctx, `DELETE FROM map_tiles WHERE world_id=$1 AND r<>0`, f.worldID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE map_tiles SET terrain='forest_cedar' WHERE world_id=$1 AND q=0`, f.worldID); err != nil {
		t.Fatal(err)
	}
	f.start(t, 10, 0, 12)
	legs := f.runUntilHome(t)
	if got := legs[len(legs)-1].tick; got > 12 {
		t.Fatalf("asymmetric route arrived at %d, after budget 12", got)
	}
}

func TestExpeditionLifecycle_FirstLegReservesHomeFromField(t *testing.T) {
	f := newExpeditionFixture(t, 0, 16, nil)
	ctx := context.Background()
	pool := testPool(t)
	if _, err := pool.Exec(ctx, `UPDATE units SET status='positioned',settlement_id=NULL,q=9,r=0 WHERE id=$1`, f.unitID); err != nil {
		t.Fatal(err)
	}
	// First unseen target is nearby, but home is much farther away than 4 ticks.
	_, err := StartMarch(ctx, pool, f.scheduler, f.eventStore, f.clk, MarchOrder{WorldID: f.worldID, PlayerID: f.ownerID, UnitID: f.unitID, TargetQ: 14, TargetR: 0, Intent: "explore", ExpeditionTicks: 4}, nil)
	if err == nil {
		t.Fatal("accepted nearby exploration with return route exceeding budget")
	}
}

func TestExpeditionLifecycle_ShorthandedNavalHomeBudget(t *testing.T) {
	f := newExpeditionFixture(t, 0, 25, nil)
	ctx := context.Background()
	pool := testPool(t)
	if _, err := pool.Exec(ctx, `DELETE FROM map_tiles WHERE world_id=$1 AND r<>0`, f.worldID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE map_tiles SET terrain=CASE WHEN q=0 THEN 'plains' WHEN q<4 THEN 'deep_sea' ELSE 'coastal_sea' END WHERE world_id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE units SET type='galley',category='naval',size=1,crew=1 WHERE id=$1`, f.unitID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',10000,0,10000,0) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,cap=10000`, f.capitalID); err != nil {
		t.Fatal(err)
	}
	f.start(t, 10, 0, 12)
	legs := f.runUntilHome(t)
	if got := legs[len(legs)-1].tick; got > 12 {
		t.Fatalf("shorthanded ship arrived at %d, after budget 12", got)
	}
}

func TestExpeditionLifecycle_NewMarchClearsEvenMatchingTick(t *testing.T) {
	f := newExpeditionFixture(t, -2, 16, nil)
	f.start(t, 9, 0, 12)
	ctx := context.Background()
	pool := testPool(t)
	// Simulate an externally stopped leg retaining its old mission row. The next
	// march arrives at the same tick, so a tick-only validity guard is insufficient.
	if _, err := pool.Exec(ctx, `UPDATE scheduled_events SET processed_at=now() WHERE world_id=$1 AND event_type='UnitArrival' AND payload->>'unit_id'=$2`, f.worldID, f.unitID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE units SET status='positioned',q=0,r=0 WHERE id=$1`, f.unitID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES($1,$2,4,0) ON CONFLICT DO NOTHING`, f.worldID, f.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := StartMarch(ctx, pool, f.scheduler, f.eventStore, f.clk, MarchOrder{WorldID: f.worldID, PlayerID: f.ownerID, UnitID: f.unitID, TargetQ: 4, TargetR: 0}, nil); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM unit_expeditions WHERE unit_id=$1`, f.unitID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatal("new march retained old expedition with matching arrival tick")
	}
}

func TestExpeditionLifecycle_AtomicTurnRetry(t *testing.T) {
	f := newExpeditionFixture(t, 0, 16, nil)
	f.start(t, 9, 0, 12)
	ctx := context.Background()
	pool := testPool(t)
	// Ensure the next arrival must turn home, then fail the outcome write.
	if _, err := pool.Exec(ctx, `UPDATE unit_expeditions SET turn_tick=0 WHERE unit_id=$1`, f.unitID); err != nil {
		t.Fatal(err)
	}
	event := f.nextArrival(t)
	name := "expedition_event_" + fmt.Sprintf("%x", f.unitID[:])
	if _, err := pool.Exec(ctx, `ALTER TABLE events ADD CONSTRAINT `+name+` CHECK (stream_id <> '`+f.unitID.String()+`' OR event_type <> 'ExpeditionTurnedHome') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `ALTER TABLE events DROP CONSTRAINT IF EXISTS `+name) })
	if err := f.h.Handle(ctx, event); err == nil {
		t.Fatal("event-store failure did not fail turn")
	}
	var homeward bool
	var rows int
	if err := pool.QueryRow(ctx, `SELECT homeward FROM unit_expeditions WHERE unit_id=$1`, f.unitID).Scan(&homeward); err != nil {
		t.Fatal(err)
	}
	if homeward {
		t.Fatal("failed outcome left expedition turned home")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE player_id=$1 AND kind='ExpeditionTurnedHome'`, f.ownerID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatal("rolled-back turn left archive notification")
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE events DROP CONSTRAINT `+name); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := f.h.Handle(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.eventPayloads(t, unit.EventExpeditionTurnedHome)) != 1 {
		t.Fatal("retried turn did not produce exactly one outcome")
	}
}

func TestExpeditionLifecycle_HalfTimeWhileRoamingNearHome(t *testing.T) {
	f := newExpeditionFixture(t, -7, 7, nil)
	f.start(t, 0, 0, 12)
	for _, leg := range f.runUntilHome(t) {
		if leg.intent == "explore" && leg.tick > 6 {
			t.Fatalf("roaming outbound leg ended at %d, after turn tick 6", leg.tick)
		}
	}
}
