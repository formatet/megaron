package handlers

import (
	"context"
	"github.com/google/uuid"
	"math"
	"testing"
)

// A tracked partial land top-up and its spill reserve shares of the ACTUAL debit.
// Cancelling either part must return cedar, never convert it to nominal timber.
func TestRecruitWarChariotCedarCancellationLandAndSpill(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'stable',1) ON CONFLICT DO NOTHING`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE settlement_goods SET amount=10000,rate=0 WHERE settlement_id=$1`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	for good, amount := range map[string]float64{"timber": 0, "cedar": 100, "bronze": 100} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,$2,$3,0,100000,0) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=$3,rate=0`, f.settlementID, good, amount); err != nil {
			t.Fatal(err)
		}
	}
	var legacy uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,settlement_id,support_settlement_id,origin_settlement_id,recruitment_costs) VALUES($1,$2,'war_chariot','land',80,'forming',$3,$3,$3,'{}') RETURNING id`, f.worldID, f.playerID, f.settlementID).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	rec, body := f.post(t, recruitPath(f), map[string]any{"unit_type": "war_chariot"})
	if rec.Code != 201 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	paid := body["paid_costs"].(map[string]any)
	if paid["cedar"] != float64(16) || paid["timber"] != float64(0) {
		t.Fatalf("actual cedar debit not shown: %+v", paid)
	}
	var spill uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT id FROM units WHERE settlement_id=$1 AND type='war_chariot' AND id!=$2`, f.settlementID, legacy).Scan(&spill); err != nil {
		t.Fatal(err)
	}
	for i, id := range []uuid.UUID{legacy, spill} {
		rec, _ = f.post(t, "/worlds/"+f.worldID.String()+"/units/"+id.String()+"/cancel-training", map[string]any{})
		if rec.Code != 200 {
			t.Fatal(rec.Code, rec.Body.String())
		}
		state := snapshotRecruitState(t, f)
		expected := 87.2
		if i == 1 {
			expected = 100
		}
		if math.Abs(state.Goods["cedar"]-expected) > 1e-6 || state.Goods["timber"] != 0 {
			t.Fatalf("part%d refunded nominal rather than paid goods: %+v", i, state.Goods)
		}
	}
	var size int
	var status string
	if err := f.pool.QueryRow(ctx, `SELECT size,status FROM units WHERE id=$1`, legacy).Scan(&size, &status); err != nil {
		t.Fatal(err)
	}
	if size != 100 || status != "disbanded" {
		t.Fatalf("cancelled tracked cohort state: size%d status%s", size, status)
	}
}
