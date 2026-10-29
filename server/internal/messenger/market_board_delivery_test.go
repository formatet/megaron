package messenger

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// A delivered messenger stands in the market: its sender learns the board as
// it is at delivery (megaron_plan_marknadstavlan). No Market, no reading.
func TestDeliveredMessengerReadsMarketBoard(t *testing.T) {
	pool := handlerIdemTestPool(t)
	ctx := context.Background()
	f := newHandlerIdemFixture(t, pool, "board")
	var owner uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "board-owner-"+uuid.NewString()).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1`, f.destID, owner); err != nil {
		t.Fatal(err)
	}
	h := NewArrivalHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())), events.NewStore(pool), nil)
	deliver := func() {
		t.Helper()
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, hex_q, hex_r, arrives_at)
			 VALUES ($1,$2,$3,$4,'what do you need?','outbound',3,0,now()) RETURNING id`,
			f.worldID, f.sender, f.originID, f.destID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		payload, _ := json.Marshal(ArrivalPayload{MessengerID: id})
		if err := h.Handle(ctx, events.ScheduledEvent{ID: 1, WorldID: f.worldID, Payload: payload, DueTick: 500}); err != nil {
			t.Fatal(err)
		}
	}
	read := func() (economy.BoardReading, bool) {
		t.Helper()
		readings, err := economy.LoadBoardReadings(ctx, pool, f.worldID, f.sender)
		if err != nil {
			t.Fatal(err)
		}
		r, ok := readings[f.destID]
		return r, ok
	}

	deliver()
	if _, ok := read(); ok {
		t.Fatal("a city without a Market gave a board reading")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'market',1)`, f.destID); err != nil {
		t.Fatal(err)
	}
	if err := economy.WriteMarketBoard(ctx, pool, f.worldID, f.destID, nil, []string{"tin"}, 400); err != nil {
		t.Fatal(err)
	}
	deliver()
	r, ok := read()
	if !ok || len(r.Sells) != 1 || r.Sells[0] != "tin" || r.ReadTick != 500 {
		t.Fatalf("delivery did not read the board: ok=%v %+v", ok, r)
	}
}
