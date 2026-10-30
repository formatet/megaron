package kharis

import (
	"context"
	"formatet/megaron/server/internal/economy"
	"github.com/google/uuid"
	"math"
	"testing"
)

func TestReligionV2DemandWeightAndNet(t *testing.T) {
	for cities, want := range []float64{0, 1, 4, 9, 16} {
		if got := RealmDemand(cities); got != want {
			t.Fatalf("cities %d demand %g want %g", cities, got, want)
		}
		for _, population := range []int{0, 100, 50000} {
			if got := computeDailyDecay(cities, population); math.Abs(got-1.2*want) > 1e-9 {
				t.Fatalf("population must not change demand: %g", got)
			}
		}
	}
	for level := 1; level <= 3; level++ {
		want := []float64{0, 1, 4, 9}[level]
		capacity := []int{0, 2, 4, 8}[level]
		if TempleWeight(level) != want || economy.TempleWorkplaces(level) != capacity {
			t.Fatal("level weight/workplaces", level)
		}
		if got := EffectiveTempleWeight(level, capacity/2, capacity, 1); got != want/2 {
			t.Fatal("half staffing", got)
		}
	}
	if got := 4*KharisPerTempleWeight - computeDailyDecay(4, 15600); math.Abs(got+10.8) > 1e-9 {
		t.Fatal("four full L1 net", got)
	}
	if got := KharisPerTempleWeight - computeDailyDecay(1, 1000); math.Abs(got-.9) > 1e-9 {
		t.Fatal("one full L1 net", got)
	}
}

func TestReligionV2IndependentTitheParts(t *testing.T) {
	values := map[string]float64{"grain": 1, "fish": 1, "timber": 1, "silver": 1, "incense": 1}
	for _, tc := range []struct {
		name             string
		level            int
		stock            map[string]float64
		fraction, weight float64
	}{
		{"L2 no silver", 2, map[string]float64{"grain": .75}, .75, 3},
		{"L3 grain only", 3, map[string]float64{"grain": 1.75}, .7, 6.3},
		{"L3 silver incense only", 3, map[string]float64{"silver": .25, "incense": .1}, .3, 2.7},
		{"no partial grain", 2, map[string]float64{"grain": .74}, 0, 0},
		{"no partial silver", 2, map[string]float64{"grain": .75, "silver": .09}, .75, 3},
		{"silver cannot replace grain", 1, map[string]float64{"silver": 100}, 0, 0},
		{"full L3", 3, map[string]float64{"grain": 1.75, "silver": .25, "incense": .1}, 1, 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := newTempleOfferForLevel(uuid.Nil, "city", tc.level, economy.TempleWorkplaces(tc.level), tc.stock, values)
			if math.Abs(o.PaidFraction-tc.fraction) > 1e-9 || math.Abs(o.EffectiveWeight-tc.weight) > 1e-9 {
				t.Fatalf("plan %+v", o)
			}
			for _, part := range o.Parts {
				if !part.Paid && o.Offering[part.Good] != 0 {
					t.Fatalf("unpaid part debited: %+v", o)
				}
			}
		})
	}
}

func TestReligionV2TransitionShieldNoticeAndReplay(t *testing.T) {
	pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
	ctx := context.Background()
	var pid uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, sid).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_calc_tick) VALUES($1,$2,$3,40,0,current_world_tick())`, pid, wid, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE religion_v2_transition SET completed=false;`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `UPDATE religion_v2_transition SET completed=false`) })
	staffTestTemple(t, pool, sid, 2)
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',100,0,4000,current_world_tick())
 ON CONFLICT (settlement_id,good_key) DO UPDATE SET amount=100,rate=0,calc_tick=current_world_tick()`, sid); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := CompleteReligionTransition(ctx, pool); err != nil {
			t.Fatal(err)
		}
	}
	var tick, until, count int
	var demand, given float64
	if err := pool.QueryRow(ctx, `SELECT current_tick FROM worlds WHERE id=$1`, wid).Scan(&tick); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT (context->>'until_tick')::int FROM kharis_notice_state WHERE world_id=$1 AND player_id=$2 AND condition='divine_punishment'`, wid, pid).Scan(&until); err != nil {
		t.Fatal(err)
	}
	if until != tick+14 {
		t.Fatalf("transition shield %d want %d", until, tick+14)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*),min((body_json->>'demand')::float8),min((body_json->>'given_weight')::float8) FROM notifications WHERE world_id=$1 AND player_id=$2 AND kind='ReligionRuleChangedV2'`, wid, pid).Scan(&count, &demand, &given); err != nil {
		t.Fatal(err)
	}
	if count != 1 || demand != 1 || given != 1 {
		t.Fatalf("notice %d demand %g given %g", count, demand, given)
	}
}
