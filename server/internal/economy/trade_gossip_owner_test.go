package economy

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestDeliveryTransportGossipKeepsOriginalOwner(t *testing.T) {
	for _, physical := range []bool{false, true} {
		name := "legacy"
		if physical {
			name = "physical"
		}
		t.Run(name, func(t *testing.T) {
			pool := testPool(t)
			f := newNavalPlayerTradeFixture(t, pool)
			ctx := context.Background()
			var route uuid.UUID
			if err := pool.QueryRow(ctx, `INSERT INTO trade_routes(world_id,origin_id,destination_id,good_key,quantity,arrives_at) VALUES($1,$2,$3,'tin',2,now()) RETURNING id`, f.worldID, f.initiatorCity, f.counterCity).Scan(&route); err != nil {
				t.Fatal(err)
			}
			var transport uuid.UUID
			if physical {
				if err := pool.QueryRow(ctx, `INSERT INTO transports(world_id,owner_id,kind,origin_id,dest_id,category,origin_q,origin_r,dest_q,dest_r,departs_at,arrives_at,due_tick,status) VALUES($1,$2,'trade',$3,$4,'land',0,0,5,0,now(),now(),1,'in_transit') RETURNING id`, f.worldID, f.initiator, f.initiatorCity, f.counterCity).Scan(&transport); err != nil {
					t.Fatal(err)
				}
			}
			payload, _ := json.Marshal(map[string]any{"trade_route_id": route, "destination_id": f.counterCity, "good_key": "tin", "quantity": 2, "delivered_quantity": 2, "transport_id": transport})
			h := NewDeliveryHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clock.NewTestClock(time.Now())))
			if err := h.Handle(ctx, events.ScheduledEvent{ID: time.Now().UnixNano(), WorldID: f.worldID, Payload: payload}); err != nil {
				t.Fatal(err)
			}
			var own int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM gossip_events WHERE world_id=$1 AND recipient_id=$2`, f.worldID, f.initiator).Scan(&own); err != nil {
				t.Fatal(err)
			}
			if own != 0 {
				t.Fatalf("delivery told owner its own trade rumour: %d", own)
			}
			var recorded uuid.UUID
			if err := pool.QueryRow(ctx, `SELECT transport_owner_id FROM gossip_events WHERE world_id=$1 AND recipient_id=$2 AND category='economy'`, f.worldID, f.counterparty).Scan(&recorded); err != nil {
				t.Fatal(err)
			}
			if recorded != f.initiator {
				t.Fatal("delivery lost original transport owner")
			}
		})
	}
}
