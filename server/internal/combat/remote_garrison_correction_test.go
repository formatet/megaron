package combat

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
)

func TestRemoteLandGarrisonCorrectionPreservesPhysicalState(t *testing.T) {
	f := newExpeditionFixture(t, -2, 8, nil)
	ctx := context.Background()
	pool := testPool(t)
	if _, err := pool.Exec(ctx, `UPDATE units SET q=4,r=0,support_settlement_id=$2 WHERE id=$1`, f.unitID, f.capitalID); err != nil {
		t.Fatal(err)
	}
	var naval, correct, nullPosition uuid.UUID
	for _, v := range []struct {
		id              *uuid.UUID
		category, utype string
		q               any
	}{{&naval, "naval", "galley", 1}, {&correct, "land", "spearman", 0}, {&nullPosition, "land", "spearman", nil}} {
		if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,settlement_id,q,r) VALUES($1,$2,$3,$4,100,'garrison',$5,$6,$7) RETURNING id`, f.worldID, f.ownerID, v.utype, v.category, f.capitalID, v.q, func() any {
			if v.q == nil {
				return nil
			}
			return 0
		}()).Scan(v.id); err != nil {
			t.Fatal(err)
		}
	}
	body, err := os.ReadFile("../../db/migrations/217_correct_remote_land_garrisons.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			t.Fatal(err)
		}
	}
	var status string
	var city, support *uuid.UUID
	var q, r int
	var owner uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT status,settlement_id,support_settlement_id,q,r,owner_id FROM units WHERE id=$1`, f.unitID).Scan(&status, &city, &support, &q, &r, &owner); err != nil {
		t.Fatal(err)
	}
	if status != "positioned" || city != nil || support == nil || *support != f.capitalID || q != 4 || r != 0 || owner != f.ownerID {
		t.Fatalf("correction moved physical state/payor: %s city%v support%v q%d r%d owner%s", status, city, support, q, r, owner)
	}
	var outcomes int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='UnitGarrisonLocationCorrectedV1' AND payload->>'q'='4' AND payload->>'previous_settlement_id'=$2`, f.unitID, f.capitalID.String()).Scan(&outcomes); err != nil {
		t.Fatal(err)
	}
	if outcomes != 1 {
		t.Fatalf("correction outcomes%d, want1 after rerun", outcomes)
	}
	for _, id := range []uuid.UUID{naval, correct, nullPosition} {
		if err := pool.QueryRow(ctx, `SELECT status,settlement_id FROM units WHERE id=$1`, id).Scan(&status, &city); err != nil {
			t.Fatal(err)
		}
		if status != "garrison" || city == nil || *city != f.capitalID {
			t.Fatalf("changed legitimate/unknown garrison %s", id)
		}
	}
}
