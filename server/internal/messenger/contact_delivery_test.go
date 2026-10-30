package messenger

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// A messenger's contact snapshot is taken when it is delivered, never when it
// is sent (megaron_kontrakt_frammande_stad: an outcome, not an intention).
func TestMessengerContactIsTakenAtDelivery(t *testing.T) {
	pool := handlerIdemTestPool(t)
	ctx := context.Background()
	f := newHandlerIdemFixture(t, pool, "contact")
	var owner uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "contact-owner-"+uuid.NewString()).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlements SET owner_id=$2, population=3000 WHERE id=$1`, f.destID, owner); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, hex_q, hex_r, arrives_at)
		 VALUES ($1,$2,$3,$4,'greetings','outbound',3,0,now()) RETURNING id`,
		f.worldID, f.sender, f.originID, f.destID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	contact := func() (int, string, string, bool) {
		t.Helper()
		var tick int
		var source, size string
		err := pool.QueryRow(ctx, `SELECT contact_tick, source, size FROM player_contacted_settlements WHERE player_id=$1 AND settlement_id=$2`,
			f.sender, f.destID).Scan(&tick, &source, &size)
		return tick, source, size, err == nil
	}
	if _, _, _, ok := contact(); ok {
		t.Fatal("a messenger on its way already gave contact")
	}
	h := NewArrivalHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())), events.NewStore(pool), nil)
	payload, _ := json.Marshal(ArrivalPayload{MessengerID: id})
	if err := h.Handle(ctx, events.ScheduledEvent{ID: 1, WorldID: f.worldID, Payload: payload, DueTick: 500}); err != nil {
		t.Fatal(err)
	}
	tick, source, size, ok := contact()
	if !ok || tick != 500 || source != "messenger" || size != "town" {
		t.Fatalf("delivery contact = tick %d source %q size %q ok %v, want 500 messenger town", tick, source, size, ok)
	}
}
