package kharis

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"testing"
)

func TestOmenNoFactsMeansSilence(t *testing.T) {
	pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET rate=CASE WHEN good_key='grain' THEN $2 ELSE 0 END WHERE settlement_id=$1`, sid, economy.GrainConsumptionPerTick(1000)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,kharis_amount,kharis_cap,kharis_calc_tick) SELECT owner_id,world_id,40,100,0 FROM settlements WHERE id=$1 ON CONFLICT DO NOTHING`, sid); err != nil {
		t.Fatal(err)
	}
	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.generateOmen(ctx, sid, wid)
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM gossip_events WHERE world_id=$1 AND category='omen'`, wid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		var text string
		_ = pool.QueryRow(ctx, `SELECT text FROM gossip_events WHERE world_id=$1 AND category='omen' LIMIT 1`, wid).Scan(&text)
		t.Fatalf("no true omen candidate, must remain silent; got %d invented omens: %s", n, text)
	}
}

func TestOmenTemplatesRequireTruePredicatesAndBoundPrecision(t *testing.T) {
	for _, tc := range []struct {
		name  string
		f     omenFact
		valid bool
	}{
		{"no_storm", omenFact{Kind: "storm"}, false}, {"storm", omenFact{Kind: "storm", ID: "real", Critical: true}, true},
		{"no_deposit", omenFact{Kind: "deposit"}, false}, {"deposit", omenFact{Kind: "deposit", Good: "tin"}, true},
		{"false_surplus", omenFact{Kind: "food_surplus", Net: -1}, false}, {"surplus", omenFact{Kind: "food_surplus", Net: 1}, true},
		{"false_deficit", omenFact{Kind: "food_deficit", Net: 1}, false}, {"deficit", omenFact{Kind: "food_deficit", Net: -1}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []float64{1, 5, 30, 60, 90} {
				_, precision, ok := omenText(tc.f, omenLevel(k), 0, 0, true)
				if ok != tc.valid {
					t.Fatalf("kharis%g condition validity%v want%v", k, ok, tc.valid)
				}
				if k < 90 && precision == 4 {
					t.Fatal("low kharis disclosed a specific hex")
				}
			}
		})
	}
	// This is the actual cap earned by a normal level-3 temple, not a magic
	// test standing above the legal ceiling. Lesser temples cannot reach it.
	top := EffectiveKharisCap(100, 3)
	if top < 90 || omenLevel(top).Precision != 4 {
		t.Fatalf("top omen unreachable at real cap%g", top)
	}
	if omenLevel(EffectiveKharisCap(100, 2)).Precision == 4 {
		t.Fatal("level2 temple granted top precision")
	}
	_, p, ok := omenText(omenFact{Kind: "storm", ID: "real", Critical: true}, omenLevel(top), 0, 0, true)
	if !ok || p != 4 {
		t.Fatal("high kharis cannot produce specific truthful omen")
	}
	_, p, _ = omenText(omenFact{Kind: "storm", ID: "real", Critical: false}, omenLevel(top), 0, 0, true)
	if p == 4 {
		t.Fatal("nonurgent omen disclosed specific hex")
	}
}

func TestOmenObservedOutcomeCooldownAndSpecificity(t *testing.T) {
	pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET rate=CASE WHEN good_key='grain' THEN $2 ELSE 0 END WHERE settlement_id=$1`, sid, economy.GrainConsumptionPerTick(1000)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE buildings SET level=3 WHERE settlement_id=$1 AND building_type='temple'`, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE provinces SET coastal=true WHERE id=(SELECT province_id FROM settlements WHERE id=$1)`, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,kharis_amount,kharis_cap,kharis_calc_tick,kharis_rate) SELECT owner_id,world_id,95,100,0,0 FROM settlements WHERE id=$1 ON CONFLICT DO NOTHING`, sid); err != nil {
		t.Fatal(err)
	}
	var storm string
	if err := pool.QueryRow(ctx, `INSERT INTO sea_storms(world_id,heading,created_tick) VALUES($1,0,0) RETURNING id::text`, wid).Scan(&storm); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES($1,0,0,1,0)`, storm); err != nil {
		t.Fatal(err)
	}
	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.randomFloat = func() float64 { return 0 }
	emit := func() {
		t.Helper()
		if err := h.generateOmen(ctx, sid, wid); err != nil {
			t.Fatal(err)
		}
	}
	emit()
	// Another temple in the same realm must not multiply the frequency.
	var other uuid.UUID
	if err := pool.QueryRow(ctx, `WITH p AS (INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,8,8,'plains') RETURNING id) INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,population,is_capital) SELECT $1,p.id,'Other temple','achaean',s.owner_id,'colony',500,false FROM p CROSS JOIN settlements s WHERE s.id=$2 RETURNING id`, wid, sid).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES($1,'temple',1,'potnia')`, other); err != nil {
		t.Fatal(err)
	}
	if err := h.generateOmen(ctx, other, wid); err != nil {
		t.Fatal(err)
	}
	emit()
	var count int
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM gossip_events WHERE world_id=$1 AND category='omen'`, wid).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("retry bypassed realm cooldown: %d", count)
	}
	if err := pool.QueryRow(ctx, `SELECT fact_data FROM gossip_events WHERE world_id=$1 AND category='omen'`, wid).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var high omenOutcome
	if err := json.Unmarshal(raw, &high); err != nil {
		t.Fatal(err)
	}
	if !high.True || high.Category != "storm" || high.Precision != 4 || high.Q == nil || *high.Q != 1 || high.FactID == "" {
		t.Fatalf("missing actual high omen outcome: %+v", high)
	}
	var eventsCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE world_id=$1 AND event_type='OmenObservedV1'`, wid).Scan(&eventsCount); err != nil {
		t.Fatal(err)
	}
	if eventsCount != 1 {
		t.Fatal("outcome not atomic with gossip")
	}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=2 WHERE id=$1`, wid); err != nil {
		t.Fatal(err)
	}
	h.randomFloat = func() float64 { return .5 }
	emit()
	if err := pool.QueryRow(ctx, `SELECT fact_data FROM gossip_events WHERE world_id=$1 AND category='omen' ORDER BY generated_at DESC LIMIT 1`, wid).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var ordinary omenOutcome
	if err := json.Unmarshal(raw, &ordinary); err != nil {
		t.Fatal(err)
	}
	if ordinary.Precision == 4 || ordinary.Q != nil {
		t.Fatal("ordinary top omen leaked exact hex")
	}
	if _, err := pool.Exec(ctx, `UPDATE player_world_records SET kharis_amount=1,kharis_rate=0 WHERE world_id=$1`, wid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=14 WHERE id=$1`, wid); err != nil {
		t.Fatal(err)
	}
	h.randomFloat = func() float64 { return 0 }
	emit()
	if err := pool.QueryRow(ctx, `SELECT fact_data FROM gossip_events WHERE world_id=$1 AND category='omen' ORDER BY generated_at DESC LIMIT 1`, wid).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var low omenOutcome
	if err := json.Unmarshal(raw, &low); err != nil {
		t.Fatal(err)
	}
	if low.Precision != 0 || low.Q != nil {
		t.Fatalf("low kharis learned precise knowledge: %+v", low)
	}
	if _, err := pool.Exec(ctx, `UPDATE sea_storm_track SET q=4 WHERE storm_id=$1`, storm); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=26 WHERE id=$1`, wid); err != nil {
		t.Fatal(err)
	}
	emit()
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM gossip_events WHERE world_id=$1 AND category='omen'`, wid).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("low kharis saw a fact outside its3hex radius: %d omens", count)
	}

}
