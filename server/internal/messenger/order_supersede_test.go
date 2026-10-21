package messenger

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestSupersedeUnboardedMovementReturnsOnlyOwnWaitingEnvelopes(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE map_tiles SET terrain='plains' WHERE world_id=$1 AND q=1 AND r=0`, f.worldID); err != nil {
		t.Fatal(err)
	}
	var prov, port, unitID, otherUnitID uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES($1,1,0,'plains',true) RETURNING id`, f.worldID).Scan(&prov); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,state) VALUES($1,$2,'Second Port','achaean',$3,'capital','active') RETURNING id`, f.worldID, prov, f.ownerID).Scan(&port); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,q,r) VALUES($1,$2,'spearman','land',100,'positioned',5,0) RETURNING id`, f.worldID, f.ownerID).Scan(&unitID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,q,r) VALUES($1,$2,'spearman','land',100,'positioned',6,0) RETURNING id`, f.worldID, f.ownerID).Scan(&otherUnitID); err != nil {
		t.Fatal(err)
	}
	put := func(targetUnit uuid.UUID, verb, status string) uuid.UUID {
		t.Helper()
		id := f.waitingMessenger(t, port, 500)
		raw, _ := json.Marshal(OrderDeliveryPayload{WorldID: f.worldID, PlayerID: f.ownerID, UnitID: targetUnit, Verb: verb, March: &combat.MarchOrder{TargetQ: 37, TargetR: 21}})
		if _, err := f.pool.Exec(ctx, `UPDATE messengers SET kind='order',order_payload=$2,passage_status=$3 WHERE id=$1`, id, raw, status); err != nil {
			t.Fatal(err)
		}
		return id
	}
	old1 := put(unitID, "march", "awaiting_passage")
	old2 := put(unitID, "redirect", "awaiting_passage")
	aboard := put(unitID, "march", "aboard")
	stance := put(unitID, "stance", "awaiting_passage")
	otherUnitOrder := put(otherUnitID, "march", "awaiting_passage")
	clk := clock.NewTestClock(time.Now())
	sched := events.NewScheduler(f.pool, clk)
	// A failed replacement rolls back both callbacks and their return jobs.
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := SupersedeUnboardedMovementTx(ctx, tx, sched, f.worldID, f.ownerID, unitID, "march", clk.Now(), 500)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if len(ids) != 2 {
		tx.Rollback(ctx)
		t.Fatalf("rollback candidate IDs=%v", ids)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{old1, old2} {
		var state string
		if err := f.pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id=$1`, id).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state != "outbound" {
			t.Fatalf("rollback changed %s", state)
		}
	}
	tx, err = f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids, err = SupersedeUnboardedMovementTx(ctx, tx, sched, f.worldID, f.ownerID, unitID, "march", clk.Now(), 500)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if len(ids) != 2 {
		tx.Rollback(ctx)
		t.Fatalf("superseded=%v", ids)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{old1, old2, aboard, stance, otherUnitOrder} {
		var state string
		var withdrawn bool
		var passageStatus *string
		if err := f.pool.QueryRow(ctx, `SELECT status,withdrawn,passage_status FROM messengers WHERE id=$1`, id).Scan(&state, &withdrawn, &passageStatus); err != nil {
			t.Fatal(err)
		}
		want := "outbound"
		if id == old1 || id == old2 {
			want = "returning"
			if !withdrawn {
				t.Fatal("order was not withdrawn")
			}
		}
		if state != want {
			t.Fatalf("runner %s state=%s want %s", id, state, want)
		}
		if id == otherUnitOrder && (withdrawn || passageStatus == nil || *passageStatus != "awaiting_passage") {
			t.Fatalf("other unit's order changed: withdrawn=%v passage=%v", withdrawn, passageStatus)
		}
	}
	var jobs int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='MessengerReturn' AND payload->>'messenger_id' IN ($2,$3)`, f.worldID, old1.String(), old2.String()).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 2 {
		t.Fatalf("physical return jobs=%d", jobs)
	}
	tx, err = f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	ids, err = SupersedeUnboardedMovementTx(ctx, tx, sched, f.worldID, f.ownerID, unitID, "stance", clk.Now(), 500)
	if err != nil || len(ids) != 0 {
		t.Fatalf("independent stance supersession %v %v", ids, err)
	}
}
