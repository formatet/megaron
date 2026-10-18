package main

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"os"
	"testing"
)

func TestRecomputeDryRun_PreviewsAndRollsBack(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var wid, pid, owner, sid uuid.UUID
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`)
	must(err)
	must(pool.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick) VALUES('dryrun','active',0) RETURNING id`).Scan(&wid))
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `UPDATE worlds SET status='archived' WHERE id=$1`, wid) })
	must(pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "dryrun-"+uuid.NewString()).Scan(&owner))
	must(pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,0,0,'plains') RETURNING id`, wid).Scan(&pid))
	must(pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital,population) VALUES($1,$2,'Dryrun','achaean',$3,'capital',true,1042) RETURNING id`, wid, pid, owner).Scan(&sid))
	_, err = pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,1,0,'coastal_sea')`, wid)
	must(err)
	_, err = pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',1200,4.28,10000,0)`, sid)
	must(err)
	// An invalid legacy placement proves reconciliation is also rolled back.
	_, err = pool.Exec(ctx, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES($1,1,'building','mine','silver')`, sid)
	must(err)
	snapshot := func() string {
		t.Helper()
		var data string
		must(pool.QueryRow(ctx, `SELECT jsonb_build_object('goods',(SELECT jsonb_agg(to_jsonb(g) ORDER BY good_key) FROM settlement_goods g WHERE settlement_id=$1),'placements',(SELECT jsonb_agg(to_jsonb(p) ORDER BY gubbe_ordinal) FROM settlement_placement p WHERE settlement_id=$1),'settlement',(SELECT to_jsonb(s) FROM settlements s WHERE id=$1))::text`, sid).Scan(&data))
		return data
	}
	before := snapshot()
	for n := 0; n < 2; n++ {
		r, err := recomputeSettlement(ctx, pool, sid, true)
		must(err)
		if r.Removed != 1 || math.Abs(r.Before.Grain-4.28) > 1e-9 || math.Abs(r.After.Grain-.5) > 1e-9 || r.After.Fish < .42 {
			t.Fatalf("dryrun did not compute delta: %+v", r)
		}
		if got := snapshot(); got != before {
			t.Fatalf("dryrun persisted changes\nbefore%s\nafter%s", before, got)
		}
		t.Logf("dry-run grain %.4f -> %.4f fish %.4f -> %.4f removed%d replaced%d, full snapshot unchanged", r.Before.Grain, r.After.Grain, r.Before.Fish, r.After.Fish, r.Removed, r.Replaced)
	}
	r, err := recomputeSettlement(ctx, pool, sid, false)
	must(err)
	if snapshot() == before {
		t.Fatal("apply did not persist")
	}
	second, err := recomputeSettlement(ctx, pool, sid, true)
	must(err)
	if second.Before != r.After || second.After != r.After || second.Removed != 0 {
		t.Fatalf("apply/dryrun disagree %+v %+v", r, second)
	}
}
