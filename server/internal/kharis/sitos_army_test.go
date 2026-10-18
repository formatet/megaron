package kharis

import (
	"context"
	"formatet/megaron/server/internal/events"
	"testing"
)

func TestSitosArmy_NoFalseStarvationAfterPaidUpkeep(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	wid, sid, uid := starvationGarrisonFixture(t, 100)
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_granary(settlement_id,good_key,amount) VALUES($1,'grain',400)`, sid); err != nil {
		t.Fatal(err)
	}
	// A previous tick's actual loss must not make today's already-fed unit hungry.
	eventID, err := events.NewStore(pool).Append(ctx, uid, events.StreamCombat, "UnitAttrition", map[string]any{"reason": "grain_shortage", "lost": 10}, wid, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE events SET world_tick=current_world_tick()-1 WHERE id=$1`, eventID.ID); err != nil {
		t.Fatal(err)
	}
	// Civilians can leave city grain zero after a successfully paid upkeep.
	// No UnitAttrition outcome exists for the current tick.
	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.applyStarvation(ctx, wid)
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='StarvationDamage'`, sid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("false StarvationDamage%d: grain reserve exists and no army loss occurred", n)
	}
}

// Run on both old and new code: false StarvationDamage only appended an audit
// event (and a chronicle line in prod), never changed these game projections.
func TestSitosArmy_AuditHasNoStateEffects(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	wid, sid, _ := starvationGarrisonFixture(t, 100)
	if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,kharis_amount,kharis_rate,kharis_calc_tick) SELECT owner_id,world_id,25,0,current_world_tick() FROM settlements WHERE id=$1 ON CONFLICT DO NOTHING`, sid); err != nil {
		t.Fatal(err)
	}
	hash := func() string {
		t.Helper()
		var value string
		if err := pool.QueryRow(ctx, `SELECT md5(jsonb_build_object('settlement',to_jsonb(s),'realm',(SELECT jsonb_agg(to_jsonb(r)) FROM player_world_records r WHERE r.world_id=s.world_id AND r.player_id=s.owner_id),'goods',(SELECT jsonb_agg(to_jsonb(g) ORDER BY good_key) FROM settlement_goods g WHERE g.settlement_id=s.id),'units',(SELECT jsonb_agg(to_jsonb(u) ORDER BY id) FROM units u WHERE u.settlement_id=s.id),'loyalty',(SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM loyalty_events l WHERE l.settlement_id=s.id))::text) FROM settlements s WHERE s.id=$1`, sid).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := hash()
	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.applyStarvation(ctx, wid)
	if after := hash(); before != after {
		t.Fatalf("audit changed game projections: %s -> %s", before, after)
	}
	var audits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='StarvationDamage'`, sid).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	t.Logf("StarvationDamage%d; full settlement/realm(goods/kharis)/units/loyalty projection hashes identical %s", audits, before)
}
