package handlers

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/messenger"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestArchivedStalledOrderShowsSenderOnlySealedInstruction(t *testing.T) {
	f := setupNotifFixture(t)
	ctx := context.Background()
	var province, city, unitID, msgID uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,0,0,'plains') RETURNING id`, f.worldID).Scan(&province); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital,population) VALUES($1,$2,'Lato','achaean',$3,'capital',true,500) RETURNING id`, f.worldID, province, f.playerID).Scan(&city); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,support_settlement_id,ordinal,q,r) VALUES($1,$2,'spearman','land',100,'positioned',$3,3,4,2) RETURNING id`, f.worldID, f.playerID, city).Scan(&unitID); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(messenger.OrderDeliveryPayload{WorldID: f.worldID, PlayerID: f.playerID, UnitID: unitID, Verb: "march", March: &combat.MarchOrder{TargetQ: 37, TargetR: 21, Intent: "colonize"}})
	if err := f.pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,kind,order_payload,hex_q,hex_r,arrives_at) VALUES($1,$2,$3,$3,'Runner — march order to (37,21).','order',$4,37,21,now()) RETURNING id`, f.worldID, f.playerID, city, payload).Scan(&msgID); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"messenger_id": msgID, "kind": "order", "port_name": "Vasiliki", "q": 4, "r": 2})
	if _, err := f.pool.Exec(ctx, `INSERT INTO notifications(world_id,player_id,kind,level,body_json) VALUES($1,$2,'PassageStalled',2,$3)`, f.worldID, f.playerID, body); err != nil {
		t.Fatal(err)
	}
	code, out := f.do(t, "GET", "/worlds/"+f.worldID.String()+"/notifications")
	if code != 200 {
		t.Fatalf("list %d %v", code, out)
	}
	rows := out["notifications"].([]any)
	got := rows[0].(map[string]any)["body"].(map[string]any)["order_summary"].(string)
	for _, part := range []string{"Spearmen", "(37,21)", "colonize"} {
		if !strings.Contains(got, part) {
			t.Fatalf("summary %q misses %q", got, part)
		}
	}
	other := uuid.New()
	if got := messenger.OwnOrderSummary(ctx, f.pool, f.worldID, other, msgID); got != "" {
		t.Fatalf("foreign reader saw %q", got)
	}
	forged, _ := json.Marshal(map[string]any{"messenger_id": uuid.New(), "kind": "order"})
	if _, err := f.pool.Exec(ctx, `INSERT INTO notifications(world_id,player_id,kind,level,body_json) VALUES($1,$2,'PassageStalled',2,$3)`, f.worldID, f.playerID, forged); err != nil {
		t.Fatal(err)
	}
	code, out = f.do(t, "GET", "/worlds/"+f.worldID.String()+"/notifications")
	if code != 200 {
		t.Fatal(code)
	}
	rows = out["notifications"].([]any)
	for _, row := range rows {
		b := row.(map[string]any)["body"].(map[string]any)
		if b["messenger_id"] == forgedID(forged) && b["order_summary"] != nil {
			t.Fatalf("forged summary %v", b)
		}
	}
}
func forgedID(raw []byte) string {
	var b map[string]string
	_ = json.Unmarshal(raw, &b)
	return b["messenger_id"]
}
