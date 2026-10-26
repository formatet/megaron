package combat

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"testing"
)

func TestCompletionNoticesCarryCityAndPlayerNames(t *testing.T) {
	pool := testPool(t)
	world, owner, city := newBuildNotifyFixture(t, pool)
	body := runBuildComplete(t, pool, world, city, "stonequarry")
	if body["settlement_name"] != "Notifytown" || body["building_name"] != "Stone Quarry" {
		t.Fatalf("build context:%+v", body)
	}
	if body["hex_q"] == nil || body["hex_r"] == nil {
		t.Fatal("lost actual build site")
	}
	var ship uuid.UUID
	if err := pool.QueryRow(context.Background(), `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id) VALUES($1,$2,'merchantman','naval',1,10,'forming',$3,$3) RETURNING id`, world, owner, city).Scan(&ship); err != nil {
		t.Fatal(err)
	}
	hub := &fakeBroadcaster{}
	h := NewTrainCompleteHandler(pool, events.NewStore(pool), hub)
	raw, _ := json.Marshal(TrainCompletePayload{SettlementID: city, UnitType: "merchantman", UnitID: ship, Count: 1})
	if err := h.Handle(context.Background(), events.ScheduledEvent{WorldID: world, Payload: raw}); err != nil {
		t.Fatal(err)
	}
	for i, kind := range hub.notified {
		if kind == "TrainComplete" {
			b := hub.payloads[i].(map[string]any)
			if b["settlement_name"] != "Notifytown" || b["unit_name"] != "Emporos" {
				t.Fatalf("train context:%+v", b)
			}
			return
		}
	}
	t.Fatal("no completion notice")
}
