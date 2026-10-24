package economy

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestSilverFlowReligiousTradePaysFullSilver(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newNavalPlayerTradeFixture(t, pool)
	leg := f.leg1Transport(t)
	if _, err := pool.Exec(ctx, `UPDATE transports SET category='land',ship_unit_id=NULL WHERE id=$1`, leg); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=17 WHERE id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES($1,'temple',1,'potnia')`, f.counterCity); err != nil {
		t.Fatal(err)
	}

	h := legacyDeliveryHandler(f)
	// travel_mins 150 -> round(2.5) = 3 ticks.
	raw, _ := json.Marshal(map[string]any{
		"transport_id": leg, "destination_id": f.counterCity, "good_key": "silver", "quantity": 100,
		"then_return": map[string]any{
			"destination_id": f.initiatorCity, "good_key": "wine", "quantity": 20,
			"messenger_id": uuid.New().String(), "travel_mins": 150, "owner_id": f.counterparty,
			"origin_q": 5, "origin_r": 0, "dest_q": 0, "dest_r": 0,
		},
	})
	evt := events.ScheduledEvent{ID: time.Now().UnixNano(), WorldID: f.worldID, Payload: raw}
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("legacy silver leg must deliver: %v", err)
	}
	var amount float64
	if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, f.counterCity).Scan(&amount); err != nil || amount != 100 {
		t.Fatalf("silver credit %v %v", amount, err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE world_id=$1 AND event_type='TempleTithe'`, f.worldID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("new tithe events=%d %v", count, err)
	}
}
