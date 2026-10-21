package handlers

import (
	"context"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"testing"
)

func TestIncomingContactRemembersOnlyOriginAfterPhysicalDelivery(t *testing.T) {
	f := setupPlacementFixture(t, nil)
	pool := p10TestPool(t)
	ctx := context.Background()
	var viewer, other, prov, city, letter uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, f.settlementID).Scan(&viewer); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "incoming-"+uuid.NewString()).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,20,20,'plains') RETURNING id`, f.worldID).Scan(&prov); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital) VALUES($1,$2,'Remote','achaean',$3,'capital',true) RETURNING id`, f.worldID, prov, other).Scan(&city); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,status,hex_q,hex_r,sent_at,arrives_at) VALUES($1,$2,$3,$4,'Let us trade','outbound',0,0,now(),now()) RETURNING id`, f.worldID, other, city, f.settlementID).Scan(&letter); err != nil {
		t.Fatal(err)
	}
	origin := province.MapPosition{Q: 20, R: 20}
	for _, tc := range []struct {
		status           string
		withdrawn, known bool
	}{{"outbound", false, false}, {"delivered", false, true}, {"returning", false, true}, {"arrived", false, true}, {"returning", true, false}, {"lost", false, false}} {
		if _, err := pool.Exec(ctx, `UPDATE messengers SET status=$2,withdrawn=$3 WHERE id=$1`, letter, tc.status, tc.withdrawn); err != nil {
			t.Fatal(err)
		}
		mem := loadRememberedTiles(ctx, pool, f.worldID, viewer)
		if mem[[2]int{20, 20}] != tc.known {
			t.Fatalf("%s withdrawn%v origin known=%v want%v", tc.status, tc.withdrawn, mem[[2]int{20, 20}], tc.known)
		}
		for _, p := range [][2]int{{19, 20}, {20, 19}, {21, 20}, {20, 21}} {
			if mem[p] {
				t.Fatalf("contact leaked neighbouring tile%v", p)
			}
		}
		contactable := false
		for _, p := range province.VisibleOrigins(ctx, pool, f.worldID, viewer) {
			if p == origin {
				contactable = true
			}
		}
		if contactable != tc.known {
			t.Fatalf("%s reachability=%v want%v", tc.status, contactable, tc.known)
		}
		// Reading through one TX must not require another connection or poison it.
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		got := loadRememberedTiles(ctx, tx, f.worldID, viewer)
		if got[[2]int{20, 20}] != tc.known {
			t.Fatal("TX contact disagrees")
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
	}
}
