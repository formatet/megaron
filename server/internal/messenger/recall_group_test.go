package messenger

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestRecallGroup_PassageRebuildsEveryMemberAndRejectsStaleGeneration(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	id := f.waitingMessenger(t, f.originID, f.currentTick+1)
	p := OrderDeliveryPayload{WorldID: f.worldID, PlayerID: f.ownerID, MessengerID: id, Verb: RecallGroupMemberVerb, RecallGroup: &RecallGroup{Q: 5, R: 0, Members: []RecallGroupMember{{UnitID: uuid.New()}, {UnitID: uuid.New()}, {UnitID: uuid.New()}}}}
	raw, _ := json.Marshal(p)
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET kind='order',order_payload=$2 WHERE id=$1`, id, raw); err != nil {
		t.Fatal(err)
	}
	sched := events.NewScheduler(f.pool, clock.NewTestClock(time.Now()))
	for generation := 1; generation <= 2; generation++ {
		tx, err := f.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := scheduleCompletion(ctx, tx, sched, id, f.currentTick+generation+1, time.Now().Add(time.Hour)); err != nil {
			tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := f.pool.Query(ctx, `SELECT id,payload FROM scheduled_events WHERE payload->>'messenger_id'=$1 ORDER BY id`, id.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	counts := map[int]int{}
	var stale events.ScheduledEvent
	for rows.Next() {
		var e events.ScheduledEvent
		if err := rows.Scan(&e.ID, &e.Payload); err != nil {
			t.Fatal(err)
		}
		var m OrderDeliveryPayload
		json.Unmarshal(e.Payload, &m)
		counts[m.PassageGeneration]++
		if len(m.RecallGroup.Members) != 3 || m.Recall == nil || m.Recall.UnitID != m.UnitID {
			t.Fatalf("lost member envelope %+v", m)
		}
		if m.PassageGeneration == 1 {
			stale = e
		}
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	rows.Close()
	if counts[1] != 3 || counts[2] != 3 {
		t.Fatalf("reboarding counts%v", counts)
	}
	// Missing unit IDs are deliberate: a stale generation must return before touching them.
	h := NewOrderDeliveryHandler(f.pool, sched, events.NewStore(f.pool), nil, clock.NewTestClock(time.Now()))
	if err := h.Handle(ctx, stale); err != nil {
		t.Fatalf("stale generation executed: %v", err)
	}
	var receipts int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM processed_deliveries WHERE event_id=$1`, stale.ID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if receipts != 0 {
		t.Fatal("stale receipt")
	}
}
