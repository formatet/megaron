package kharis

import (
	"context"
	"math"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

func TestEarlyTempleOfferingLocalGiftAndProjection(t *testing.T) {
	for _, good := range []string{"grain", "fish", "timber", "silver"} {
		t.Run(good, func(t *testing.T) {
			pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
			ctx := context.Background()
			var pid uuid.UUID
			if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, sid).Scan(&pid); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_calc_tick)
			 VALUES($1,$2,$3,20,0,current_world_tick())`, pid, wid, sid); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE buildings SET level=2 WHERE settlement_id=$1 AND building_type='temple';`, sid); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE settlement_labor SET weight=.3 WHERE settlement_id=$1 AND good_key='cult'`, sid); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=0,rate=0 WHERE settlement_id=$1`, sid); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick)
			 VALUES($1,$2,.25,0,100,current_world_tick()) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=.25,rate=0`, sid, good); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO divine_labor_valuations(world_id,good_key,rarity_spread,rarity_volume,divine_value,calc_tick)
			 SELECT $1,key,0,0,1,current_world_tick() FROM goods WHERE key IN ('grain','fish','timber','silver')`, wid); err != nil {
				t.Fatal(err)
			}
			offers, err := LoadTempleOffers(ctx, pool, pid, wid)
			if err != nil {
				t.Fatal(err)
			}
			if len(offers) != 1 || !offers[0].Fed || offers[0].Offering[good] != .25 || offers[0].Required["grain"] != .25 {
				t.Fatalf("plan: %+v", offers)
			}
			net, _, _, _, err := ProjectDailyNet(ctx, pool, pid, wid)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(net-(.3*kharisPerTempleDay-decayBas)) > 1e-8 {
				t.Fatalf("fed staffed L2 net=%g want shared yield minus decay", net)
			}
			h := newTestTickHandler(pool)
			eid := atomic.AddInt64(&advanceOneDayEventID, 1)
			w := wanaxSnap{playerID: pid, settlementID: sid, kharis: 20, kharisCap: 75, devotionSum: .3, templeCities: 1, maxTempleLevel: 2}
			if err := h.processMaintenance(ctx, w, wid, eid); err != nil {
				t.Fatal(err)
			}
			if err := h.processMaintenance(ctx, w, wid, eid); err != nil {
				t.Fatal(err)
			}
			var after, amount, gain float64
			if err := pool.QueryRow(ctx, `SELECT kharis_amount FROM player_world_records WHERE player_id=$1 AND world_id=$2`, pid, wid).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key=$2`, sid, good).Scan(&amount); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `SELECT (payload->>'kharis_gain')::float8 FROM events WHERE world_id=$1 AND event_type='KharisOffering' ORDER BY id DESC LIMIT 1`, wid).Scan(&gain); err != nil {
				t.Fatal(err)
			}
			if math.Abs(after-(20+net)) > 1e-8 || amount != 0 || math.Abs(gain-.3*kharisPerTempleDay) > 1e-8 {
				t.Fatalf("actual after%g amount%g gain%g", after, amount, gain)
			}
			// A richer remote city of the same Wanax is never charged for this temple.
			var other uuid.UUID
			var otherProvince uuid.UUID
			if err := pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,8,8,'plains') RETURNING id`, wid).Scan(&otherProvince); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital,population)
			 VALUES($1,$3,'Remote','achaean',$2,'capital',false,100) RETURNING id`, wid, pid, otherProvince).Scan(&other); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',99,0,100,current_world_tick())`, other); err != nil {
				t.Fatal(err)
			}
			net, _, _, _, err = ProjectDailyNet(ctx, pool, pid, wid)
			if err != nil {
				t.Fatal(err)
			}
			if net != -1.8 {
				t.Fatalf("empty local temple borrowed remote goods: net%g", net)
			}
			fed, total := h.applyTempleOffering(ctx, pid, wid)
			if fed != 0 || total != 1 {
				t.Fatalf("empty local fed%d/%d", fed, total)
			}
			if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'`, other).Scan(&amount); err != nil {
				t.Fatal(err)
			}
			if amount != 99 {
				t.Fatalf("remote charged %g", amount)
			}
		})
	}
}

func TestEarlyTempleOfferingMixedGiftAtomic(t *testing.T) {
	pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=0,rate=0 WHERE settlement_id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	for _, g := range []string{"grain", "fish"} {
		if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick)
		 VALUES($1,$2,.125,0,100,current_world_tick()) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=.125,rate=0`, sid, g); err != nil {
			t.Fatal(err)
		}
	}
	h := newTestTickHandler(pool)
	values := map[string]float64{"grain": 1, "fish": 1}
	// Reject the second debit after the first one has actually executed.
	if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_early_temple_fish() RETURNS trigger LANGUAGE plpgsql AS $$
	 BEGIN IF NEW.good_key='fish' AND NEW.settlement_id='`+sid.String()+`' THEN RAISE EXCEPTION 'reject fish'; END IF; RETURN NEW; END $$;
	 CREATE TRIGGER reject_early_temple_fish BEFORE UPDATE ON settlement_goods FOR EACH ROW EXECUTE FUNCTION reject_early_temple_fish()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DROP TRIGGER IF EXISTS reject_early_temple_fish ON settlement_goods; DROP FUNCTION IF EXISTS reject_early_temple_fish()`)
	})
	if fed, err := h.chargeTempleOffering(ctx, sid, values); err == nil || fed {
		t.Fatalf("failed debit fed%v err%v", fed, err)
	}
	var grain, fish float64
	if err := pool.QueryRow(ctx, `SELECT (SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'),(SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='fish')`, sid).Scan(&grain, &fish); err != nil {
		t.Fatal(err)
	}
	if grain != .125 || fish != .125 {
		t.Fatalf("partial gift escaped rollback grain%g fish%g", grain, fish)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER reject_early_temple_fish ON settlement_goods; DROP FUNCTION reject_early_temple_fish()`); err != nil {
		t.Fatal(err)
	}
	if fed, err := h.chargeTempleOffering(ctx, sid, values); err != nil || !fed {
		t.Fatalf("retry fed%v err%v", fed, err)
	}
	_ = wid
}

func TestEarlyTempleOfferingScarcityAndDeterministicSubstitutes(t *testing.T) {
	values := map[string]float64{"grain": 1.5, "fish": 3, "timber": 3, "silver": 3, "oil": 9, "wine": 9}
	if got := scarcityFromValues(values, map[string]float64{"grain": 1}, nil); got != 1.5 {
		t.Fatalf("early scarcity %g", got)
	}
	for i := 0; i < 20; i++ {
		plan, fed := planTempleOffering(map[string]float64{"grain": .1, "fish": 1, "timber": 1, "silver": 1}, values)
		if !fed || plan["grain"] != .1 || math.Abs(plan["fish"]-.075) > 1e-10 || len(plan) != 2 {
			t.Fatalf("priority/value plan%v fed%v", plan, fed)
		}
	}
	plan, fed := planTempleOffering(map[string]float64{"grain": .1, "fish": .01, "oil": 999, "wine": 999}, values)
	if fed || len(plan) != 0 {
		t.Fatalf("incomplete/late gift should not pay: %v %v", plan, fed)
	}
}
