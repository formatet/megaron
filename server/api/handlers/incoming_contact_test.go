package handlers

import (
	"context"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"testing"
)

// A received letter names its writer, never where they live (Timothy
// 2026-10-24, temenos_settlement "Svar och kartkunskap"). The recipient's map
// memory and write list learn nothing from it at any stage; the sender still
// knows the city their own runner physically reached.
func TestIncomingLetterRevealsNoOriginCity(t *testing.T) {
	f := setupPlacementFixture(t, nil)
	pool := p10TestPool(t)
	ctx := context.Background()
	var viewer, other, prov, city, letter uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, f.settlementID).Scan(&viewer); err != nil {
		t.Fatal(err)
	}
	var dest province.MapPosition
	if err := pool.QueryRow(ctx, `SELECT p.map_q,p.map_r FROM settlements s JOIN provinces p ON p.id=s.province_id WHERE s.id=$1`, f.settlementID).Scan(&dest.Q, &dest.R); err != nil {
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
	has := func(ps []province.MapPosition, want province.MapPosition) bool {
		for _, p := range ps {
			if p == want {
				return true
			}
		}
		return false
	}
	for _, tc := range []struct {
		status                     string
		withdrawn, senderKnowsDest bool
	}{{"outbound", false, false}, {"delivered", false, true}, {"returning", false, true}, {"arrived", false, true}, {"returning", true, false}, {"lost", false, false}} {
		if _, err := pool.Exec(ctx, `UPDATE messengers SET status=$2,withdrawn=$3 WHERE id=$1`, letter, tc.status, tc.withdrawn); err != nil {
			t.Fatal(err)
		}
		if loadRememberedTiles(ctx, pool, f.worldID, viewer)[[2]int{20, 20}] {
			t.Fatalf("%s withdrawn=%v: the recipient's map remembers the sender's city", tc.status, tc.withdrawn)
		}
		if has(province.VisibleOrigins(ctx, pool, f.worldID, viewer), origin) {
			t.Fatalf("%s withdrawn=%v: the recipient may write to the sender's city", tc.status, tc.withdrawn)
		}
		if got := has(province.ContactedPositions(ctx, pool, f.worldID, other), dest); got != tc.senderKnowsDest {
			t.Fatalf("%s withdrawn=%v: sender knows the destination=%v, want %v", tc.status, tc.withdrawn, got, tc.senderKnowsDest)
		}
		// Reading through one TX must not require another connection or poison it.
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if loadRememberedTiles(ctx, tx, f.worldID, viewer)[[2]int{20, 20}] {
			t.Fatal("TX contact disagrees")
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
	}
}
