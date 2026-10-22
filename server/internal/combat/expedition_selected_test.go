package combat

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/province"
)

// Timothy's two deep-sea destinations: a nearby unseen candidate must not
// replace (24,20). A coast candidate must not override the first leg either.
func TestExpedition_SelectedGalleyTargetFirstThenCoast(t *testing.T) {
	f := newExpeditionFixture(t, -2, 10, nil)
	ctx := context.Background()
	pool := testPool(t)
	for _, query := range []string{
		`UPDATE map_tiles SET q=q+20,r=r+20,terrain='deep_sea' WHERE world_id=$1`,
		`UPDATE provinces SET map_q=map_q+20,map_r=map_r+20 WHERE world_id=$1`,
		`UPDATE map_tiles SET terrain='plains' WHERE world_id=$1 AND q=20 AND r=20`,
		`UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id=$1 AND q=25 AND r=24`,
	} {
		if _, err := pool.Exec(ctx, query, f.worldID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE units SET type='galley',category='naval',size=1,crew=100 WHERE id=$1`, f.unitID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',10000,0,10000,0)`, f.capitalID); err != nil {
		t.Fatal(err)
	}
	var targetTerrain, alternativeTerrain string
	if err := pool.QueryRow(ctx, `SELECT (SELECT terrain FROM map_tiles WHERE world_id=$1 AND q=24 AND r=20),(SELECT terrain FROM map_tiles WHERE world_id=$1 AND q=23 AND r=16)`, f.worldID).Scan(&targetTerrain, &alternativeTerrain); err != nil {
		t.Fatal(err)
	}
	res := f.start(t, 24, 20, 100)
	if res.TargetQ != 24 || res.TargetR != 20 {
		t.Fatalf("first leg=(%d,%d), want selected (24,20)", res.TargetQ, res.TargetR)
	}
	var q, r int
	if err := pool.QueryRow(ctx, `SELECT target_q,target_r FROM units WHERE id=$1`, f.unitID).Scan(&q, &r); err != nil {
		t.Fatal(err)
	}
	if q != 24 || r != 20 {
		t.Fatalf("persisted first leg=(%d,%d), want (24,20)", q, r)
	}
	t.Logf("galley selected=(24,20) %s; alternative=(23,16) %s; actual first=(%d,%d)", targetTerrain, alternativeTerrain, q, r)
	if targetTerrain != "deep_sea" || alternativeTerrain != "deep_sea" {
		t.Fatal("wrong terrain fixture")
	}
	if err := f.h.Handle(ctx, f.nextArrival(t)); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT target_q,target_r FROM units WHERE id=$1`, f.unitID).Scan(&q, &r); err != nil {
		t.Fatal(err)
	}
	if q != 25 || r != 24 {
		t.Fatalf("second leg=(%d,%d), want unseen coast (25,24)", q, r)
	}
}

func TestExpedition_SecondLegNearestUnseenFromSelected(t *testing.T) {
	f := newExpeditionFixture(t, -2, 16, nil)
	ctx := context.Background()
	if _, err := testPool(t).Exec(ctx, `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES($1,$2,4,0)`, f.worldID, f.ownerID); err != nil {
		t.Fatal(err)
	}
	res := f.start(t, 4, 0, 100)
	if res.TargetQ != 4 || res.TargetR != 0 {
		t.Fatalf("seen selected target replaced: %+v", res)
	}
	if err := f.h.Handle(ctx, f.nextArrival(t)); err != nil {
		t.Fatal(err)
	}
	var q, r int
	if err := testPool(t).QueryRow(ctx, `SELECT target_q,target_r FROM units WHERE id=$1`, f.unitID).Scan(&q, &r); err != nil {
		t.Fatal(err)
	}
	if q != 1 || r != 3 {
		t.Fatalf("second leg=(%d,%d), want nearest unseen from (4,0): (1,3)", q, r)
	}
}

// Persist a pre-change first leg: area (9,0), actual destination (4,0).
// Arrival must finish that saved route and choose the next leg from (4,0).
func TestExpedition_LegacySavedLegUnchanged(t *testing.T) {
	f := newExpeditionFixture(t, -2, 16, nil)
	f.start(t, 4, 0, 100)
	ctx := context.Background()
	pool := testPool(t)
	if _, err := pool.Exec(ctx, `UPDATE unit_expeditions SET area_q=9 WHERE unit_id=$1`, f.unitID); err != nil {
		t.Fatal(err)
	}
	var route string
	if err := pool.QueryRow(ctx, `SELECT march_route::text FROM units WHERE id=$1`, f.unitID).Scan(&route); err != nil {
		t.Fatal(err)
	}
	event := f.nextArrival(t)
	// Loading an old row retains the actual destination and its saved route.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, ok, err := loadExpedition(ctx, tx, f.unitID)
	if err != nil || !ok {
		tx.Rollback(ctx)
		t.Fatalf("legacy load: %v / %v", ok, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var unchanged bool
	if err := pool.QueryRow(ctx, `SELECT target_q=4 AND target_r=0 AND march_route::text=$2 FROM units WHERE id=$1`, f.unitID, route).Scan(&unchanged); err != nil {
		t.Fatal(err)
	}
	if !unchanged {
		t.Fatal("saved pre-change leg rewritten")
	}
	g, roads, err := province.LoadWorldGraph(ctx, pool, f.worldID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.h.Handle(ctx, event); err != nil {
		t.Fatal(err)
	}
	expected, err := nextExpeditionLeg(ctx, pool, g, roads, f.worldID, f.ownerID, province.MapPosition{Q: 9, R: 0}, province.MapPosition{Q: 4, R: 0}, "land")
	if err != nil {
		t.Fatal(err)
	}
	var q, r int
	if err := pool.QueryRow(ctx, `SELECT target_q,target_r FROM units WHERE id=$1`, f.unitID).Scan(&q, &r); err != nil {
		t.Fatal(err)
	}
	if !expected.Found || q != expected.Target.Q || r != expected.Target.R {
		t.Fatalf("legacy next=(%d,%d), want old area planner %+v", q, r, expected)
	}
}
