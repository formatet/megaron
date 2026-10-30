package economy

import (
	"context"
	"encoding/json"
	"fmt"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
	"time"
)

func storageFixture(t *testing.T, cap float64) (*pgxpool.Pool, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE worlds SET status='archived' WHERE status='active'`)
	wid, owner, pid, sid := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO worlds(id,name,status,current_tick) VALUES($1,$2,'active',10)`, wid, "storage-"+wid.String())
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `UPDATE worlds SET status='archived' WHERE id=$1`, wid) })
	exec(`INSERT INTO players(id,username,password_hash,agora_exempt) VALUES($1,$2,'x',true)`, owner, "storage-"+owner.String())
	exec(`INSERT INTO provinces(id,world_id,map_q,map_r,terrain_type) VALUES($1,$2,0,0,'plains')`, pid, wid)
	exec(`INSERT INTO settlements(id,world_id,province_id,owner_id,name,culture_id,control_type,is_capital,state,population) VALUES($1,$2,$3,$4,'MalliaWest','achaean','colony',false,'active',1000)`, sid, wid, pid, owner)
	exec(`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'silver',0,0,$2,10)`, sid, cap)
	return pool, wid, sid, owner
}

func TestStorageDeliveryCapOutcomeAtomicAndRetrySafe(t *testing.T) {
	for _, cap := range []float64{60, 960} {
		t.Run(fmt.Sprint(cap), func(t *testing.T) {
			pool, wid, sid, owner := storageFixture(t, cap)
			ctx := context.Background()
			raw, _ := json.Marshal(map[string]any{"destination_id": sid, "good_key": "silver", "quantity": 150, "delivered_quantity": 150})
			e := events.ScheduledEvent{ID: int64(290000 + int(cap)), WorldID: wid, Payload: raw, DueTick: 10}
			h := NewDeliveryHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clock.NewTestClock(time.Unix(0, 0))))
			for i := 0; i < 2; i++ {
				if err := h.Handle(ctx, e); err != nil {
					t.Fatal(err)
				}
			}
			var amount float64
			if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, sid).Scan(&amount); err != nil {
				t.Fatal(err)
			}
			want := 150.
			if cap < 150 {
				want = cap
			}
			if amount != want {
				t.Fatalf("amount%g want%g", amount, want)
			}
			var count, audit int
			var lost float64
			if err := pool.QueryRow(ctx, `SELECT count(*),COALESCE(sum((body_json->>'lost_quantity')::float8),0) FROM notifications WHERE player_id=$1 AND kind='StorageOverflow'`, owner).Scan(&count, &lost); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='StorageOverflow'`, sid).Scan(&audit); err != nil {
				t.Fatal(err)
			}
			wantCount := 0
			if cap < 150 {
				wantCount = 1
			}
			if count != wantCount || audit != wantCount || lost != 150-want {
				t.Fatalf("storage outcome notices%d audit%d lost%g; want%d/%g", count, audit, lost, wantCount, 150-want)
			}
		})
	}
}

func TestStorageLossNoticeFailureRollsBackCreditAndClaim(t *testing.T) {
	pool, wid, sid, owner := storageFixture(t, 60)
	ctx := context.Background()
	constraint := "reject_storage_" + uuid.New().String()[:8]
	if _, err := pool.Exec(ctx, `ALTER TABLE notifications ADD CONSTRAINT `+constraint+` CHECK(kind<>'StorageOverflow' OR player_id<>'`+owner.String()+`'::uuid)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `ALTER TABLE notifications DROP CONSTRAINT IF EXISTS `+constraint)
	})
	raw, _ := json.Marshal(map[string]any{"destination_id": sid, "good_key": "silver", "quantity": 150, "delivered_quantity": 150})
	e := events.ScheduledEvent{ID: 290200, WorldID: wid, Payload: raw, DueTick: 10}
	h := NewDeliveryHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clock.NewTestClock(time.Unix(0, 0))))
	if err := h.Handle(ctx, e); err == nil {
		t.Fatal("missing notice persistence failure")
	}
	var amount float64
	var audits, claims int
	if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, sid).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='StorageOverflow'`, sid).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM processed_deliveries WHERE event_id=$1`, e.ID).Scan(&claims); err != nil {
		t.Fatal(err)
	}
	if amount != 0 || audits != 0 || claims != 0 {
		t.Fatalf("failed credit leaked stock%g audit%d claim%d", amount, audits, claims)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE notifications DROP CONSTRAINT `+constraint); err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(ctx, e); err != nil {
		t.Fatal(err)
	}
}

func TestStorageRealizedProductionLossAndNoticeAreOneOutcome(t *testing.T) {
	pool, wid, sid, owner := storageFixture(t, 60)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=50,rate=10 WHERE settlement_id=$1;`, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=12 WHERE id=$1`, wid); err != nil {
		t.Fatal(err)
	}
	realize := func() {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err := RecomputeProduction(ctx, tx, sid); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	realize()
	realize()
	var n int
	var lost float64
	if err := pool.QueryRow(ctx, `SELECT count(*),sum((payload->>'lost_quantity')::float8) FROM events WHERE stream_id=$1 AND event_type='StorageOverflow' AND payload->>'reason'='production'`, sid).Scan(&n, &lost); err != nil {
		t.Fatal(err)
	}
	if n != 1 || lost != 10 {
		t.Fatalf("realized production outcomes%d lost%g want1/10", n, lost)
	}
	// Archived notices are not resurrected by delivery jobs or a handler retry.
	var id int64
	var raw json.RawMessage
	if err := pool.QueryRow(ctx, `SELECT id,payload FROM scheduled_events WHERE world_id=$1 AND event_type='StorageOverflowDelivery'`, wid).Scan(&id, &raw); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM notifications WHERE player_id=$1 AND kind='StorageOverflow'`, owner); err != nil {
		t.Fatal(err)
	}
	h := NewStorageNoticeDelivery(pool, events.NewStore(pool), nil)
	e := events.ScheduledEvent{ID: id, WorldID: wid, Payload: raw, DueTick: 12}
	for i := 0; i < 2; i++ {
		if err := h.Handle(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE player_id=$1 AND kind='StorageOverflow'`, owner).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("delivery job resurrected archived notice")
	}
}

// A store that refills to its cap every few ticks is one ongoing loss: one notice,
// every outcome still written. A quiet spell longer than the window starts a new one.
func TestStorageOverflowProductionNoticeOncePerOngoingLoss(t *testing.T) {
	pool, wid, sid, owner := storageFixture(t, 60)
	ctx := context.Background()
	var quiet int
	if err := pool.QueryRow(ctx, `SELECT storage_overflow_quiet_ticks()`).Scan(&quiet); err != nil {
		t.Fatal(err)
	}
	overflowAt := func(tick int) {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=$2 WHERE id=$1`, wid, tick); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `SELECT storage_capped($1,'silver',100,60,'production')`, sid); err != nil {
			t.Fatal(err)
		}
	}
	counts := func() (notices, audits int) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE player_id=$1 AND kind='StorageOverflow'`, owner).Scan(&notices); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='StorageOverflow'`, sid).Scan(&audits); err != nil {
			t.Fatal(err)
		}
		return
	}
	last := 0
	for tick := 10; tick <= 60; tick += 5 { // the fish pattern seen live
		overflowAt(tick)
		last = tick
	}
	if n, a := counts(); n != 1 || a != 11 {
		t.Fatalf("5-tick refills: notices %d audits %d, want 1/11", n, a)
	}
	overflowAt(last + quiet - 1) // still inside the window
	if n, _ := counts(); n != 1 {
		t.Fatalf("inside the quiet window: notices %d, want 1", n)
	}
	overflowAt(last + 2*quiet) // a quiet spell longer than the window
	if n, a := counts(); n != 2 || a != 13 {
		t.Fatalf("after a quiet spell: notices %d audits %d, want 2/13", n, a)
	}
}
