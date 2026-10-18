package economy

import (
	"context"
	"math"
	"testing"
)

// Exact translated Scheria catchment sampled read-only from live: 9 plains,
// 4 sea, 5 olive woods. No farm; the extra 42 people produce .42 fish under the
// best-worker remainder rule. Stock/food store are deliberately irrelevant here.
func TestAIRound4_Scheria1042FoodRequirement(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	wid, _, sid := foundingForecastFixture(t, 1042, "plains")
	terrains := []struct {
		q, r    int
		terrain string
	}{{-2, 0, "coastal_sea"}, {-2, 1, "forest_olive_grove"}, {-2, 2, "forest_olive_grove"}, {-1, -1, "coastal_sea"}, {-1, 0, "plains"}, {-1, 1, "forest_olive_grove"}, {-1, 2, "forest_olive_grove"}, {0, -2, "coastal_sea"}, {0, -1, "coastal_sea"}, {0, 1, "forest_olive_grove"}, {0, 2, "plains"}, {1, -2, "plains"}, {1, -1, "plains"}, {1, 0, "plains"}, {1, 1, "plains"}, {2, -2, "plains"}, {2, -1, "plains"}, {2, 0, "plains"}}
	for _, h := range terrains {
		if _, err := pool.Exec(ctx, `UPDATE map_tiles SET terrain=$4,coastal=$5 WHERE world_id=$1 AND q=$2 AND r=$3`, wid, h.q, h.r, h.terrain, h.terrain == "coastal_sea"); err != nil {
			t.Fatal(err)
		}
	}
	for n := 1; n <= 5; n++ {
		q, r := -2, 0
		if n >= 3 {
			q, r = -1, -1
		}
		if n == 5 {
			q, r = 0, -2
		}
		if _, err := pool.Exec(ctx, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES($1,$2,'hex',$3,$4,'fish')`, sid, n, q, r); err != nil {
			t.Fatal(err)
		}
	}
	check := func(wantFish float64) {
		t.Helper()
		if err := RecomputeProduction(ctx, pool, sid); err != nil {
			t.Fatal(err)
		}
		var grain, fish float64
		if err := pool.QueryRow(ctx, `SELECT (SELECT rate FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'),COALESCE((SELECT rate FROM settlement_goods WHERE settlement_id=$1 AND good_key='fish'),0)`, sid).Scan(&grain, &fish); err != nil {
			t.Fatal(err)
		}
		req, ok, automatic, err := FoodGubbarRequiredWithAutomatic(ctx, pool, sid)
		if automatic.RemainderPeople != 42 || math.Abs(automatic.RemainderFish-.42) > 1e-9 || automatic.NearjordGrain != 0.5 || math.Abs(automatic.GrainPerTick()-grain) > 1e-9 {
			t.Fatalf("automatic food %+v vs actualgrain%f", automatic, grain)
		}
		if err != nil || !ok || req != 5 || math.Abs(grain-.5) > 1e-9 || math.Abs(fish-(wantFish+.42)) > 1e-9 {
			t.Fatalf("grain%f fish%f req%d suff%v err%v", grain, fish, req, ok, err)
		}
		t.Logf("Scheria1042: grain=%.2f fish=%.2f demand=%.2f foodnet=%.2f required=%d", grain, fish, GrainConsumptionPerTick(1042), FoodNet(grain+fish, 1042), req)
		if wantFish >= 5 && FoodNet(grain+fish, 1042) < 0 {
			t.Fatal("five food citizens must actually feed the people")
		}
		if wantFish < 5 && FoodNet(grain+fish, 1042) >= 0 {
			t.Fatal("fewer than five food citizens must actually fail")
		}
	}
	check(5)
	if _, err := pool.Exec(ctx, `DELETE FROM settlement_placement WHERE settlement_id=$1 AND gubbe_ordinal>1`, sid); err != nil {
		t.Fatal(err)
	}
	check(1)
	if _, err := pool.Exec(ctx, `DELETE FROM settlement_placement WHERE settlement_id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	check(0)
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=99999 WHERE settlement_id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	check(0)
}
