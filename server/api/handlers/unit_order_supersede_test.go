package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/messenger"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecondMovementOrderCallsBackOnlyUnboardedPriorRunner(t *testing.T) {
	f, router := setupRecallCourierWorld(t)
	pool := unitLoadTestPool(t)
	ctx := context.Background()
	var origin, port, province uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM settlements WHERE world_id=$1 AND owner_id=$2 LIMIT 1`, f.worldID, f.playerID).Scan(&origin); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES($1,1,0,'plains',true) RETURNING id`, f.worldID).Scan(&province); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,state) VALUES($1,$2,'Waiting Port','achaean',$3,'capital','active') RETURNING id`, f.worldID, province, f.playerID).Scan(&port); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(messenger.OrderDeliveryPayload{WorldID: f.worldID, PlayerID: f.playerID, UnitID: f.unitID, Verb: "march"})
	var old uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,message_text,status,kind,hex_q,hex_r,dest_q,dest_r,arrives_at,order_payload,passage_status,passage_port_id,passage_since_tick) VALUES($1,$2,$3,'Runner — march to (37,21).','outbound','order',0,0,4,0,now(),$4,'awaiting_passage',$5,0) RETURNING id`, f.worldID, f.playerID, origin, raw, port).Scan(&old); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/worlds/"+f.worldID.String()+"/units/"+f.unitID.String()+"/recall", bytes.NewReader([]byte(`{"target_q":2,"target_r":1}`)))
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 202 {
		t.Fatalf("new order %d %s", rec.Code, rec.Body.String())
	}
	var response struct {
		MessengerID uuid.UUID   `json:"messenger_id"`
		Superseded  []uuid.UUID `json:"superseded_messenger_ids"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Superseded) != 1 || response.Superseded[0] != old {
		t.Fatalf("superseded=%v", response.Superseded)
	}
	var state string
	var withdrawn bool
	if err := pool.QueryRow(ctx, `SELECT status,withdrawn FROM messengers WHERE id=$1`, old).Scan(&state, &withdrawn); err != nil {
		t.Fatal(err)
	}
	if state != "returning" || !withdrawn {
		t.Fatalf("old runner state=%s withdrawn=%v", state, withdrawn)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id=$1`, response.MessengerID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "outbound" {
		t.Fatalf("replacement state=%s", state)
	}
	var returns int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE event_type='MessengerReturn' AND payload->>'messenger_id'=$1`, old.String()).Scan(&returns); err != nil {
		t.Fatal(err)
	}
	if returns != 1 {
		t.Fatalf("physical returns=%d", returns)
	}
}
