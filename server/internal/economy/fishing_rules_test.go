package economy

import (
	"context"
	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
	"math"
	"testing"
)

func TestDeepFishingRequiresHarbour(t *testing.T) {
	for _, level := range []int{0, 1, 2, 3} {
		places, mult := hexGoodPlaces("deep_sea", false, false, false, false, map[string]int{"harbour": level}, "fish")
		want := 0
		wantMult := 1.0
		if level > 0 {
			want = []int{0, 1, 2, 4}[level]
		}
		if places != want || mult != wantMult {
			t.Errorf("harbour L%d deep fishing=%d x %v, want %d x %v", level, places, mult, want, wantMult)
		}
	}
}

func TestBayFishingLandAndBoundary(t *testing.T) {
	for land := 0; land <= 6; land++ {
		tile := hexRuleTile{rules: rulesWithLandNeighbors("coastal_sea", false, false, false, false, land)}
		base := 2
		wantMult := 1.0
		if land >= 4 {
			wantMult = 2
		}
		for _, level := range []int{0, 1, 2, 3} {
			places, mult := tile.goodPlaces(map[string]int{"harbour": level}, "fish")
			want := base
			if level > 0 {
				want = base
			}
			if places != want || mult != wantMult {
				t.Errorf("land%d level%d=%d want%d", land, level, places, want)
			}
		}
	}
}

func TestFishingOwnerOptionsCapacityEffectsAndDemolition(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	sid := seedFullRingFixture(t, 100, 1000, "deep_sea")
	var wid uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT world_id FROM settlements WHERE id=$1`, sid).Scan(&wid); err != nil {
		t.Fatal(err)
	}
	check := func(level int, want int) {
		t.Helper()
		opts, err := LoadHexProductionOptions(ctx, pool, sid, nil)
		if err != nil {
			t.Fatal(err)
		}
		total := 0
		for _, opt := range opts {
			if opt.PlaceCapPerGood["fish"] != want {
				t.Fatalf("level%d cap=%v want%d", level, opt.PlaceCapPerGood, want)
			}
			total += opt.PlaceCapPerGood["fish"]
		}
		capacity, err := LoadHexCapacity(ctx, pool, sid)
		if err != nil {
			t.Fatal(err)
		}
		if capacity["fish"] != total || total != 18*want {
			t.Fatalf("capacity=%d options=%d want%d", capacity["fish"], total, 18*want)
		}
		if level == 0 {
			if a := AutomaticFoodForPopulation(opts, 150); a.RemainderFish != 0 {
				t.Fatalf("unavailable deep fishing fed remainder: %+v", a)
			}
			slots := rankSlotsFromOptions(opts, hexgrid.Coord{})
			if len(slots) != 0 {
				t.Fatalf("deep without harbour offered founding food: %+v", slots)
			}
		}
	}
	check(0, 0)
	c := hexgrid.Coord{Q: 1, R: 0}
	fx, err := HexBuildEffects(ctx, pool, "harbour", wid, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(fx) != 1 || fx[0].PlacesWithout != 0 || fx[0].PlacesWith != 1 || !fx[0].NeedsBuilding {
		t.Fatalf("deep harbour effects=%+v", fx)
	}
	if fx[0].PlacesByLevel != [3]int{1, 2, 4} || fx[0].RateByLevel != [3]float64{1, 1, 1} {
		t.Fatalf("deep effect levels=%+v", fx)
	}
	if text := UpgradeEffectText(fx, 1); text != "this hex's fish places 1 → 2" {
		t.Fatalf("upgrade effect=%s", text)
	}
	goods, err := RuleBuildingGoods(ctx, pool, sid, "harbour", nil, nil)
	if err != nil || len(goods) != 1 || goods[0] != "fish" {
		t.Fatalf("goods=%v err=%v", goods, err)
	}
	for level := 1; level <= 3; level++ {
		if _, err := pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'harbour',$2) ON CONFLICT(settlement_id,building_type) WHERE hex_q IS NULL DO UPDATE SET level=EXCLUDED.level`, sid, level); err != nil {
			t.Fatal(err)
		}
		check(level, []int{0, 1, 2, 4}[level])
		opts, err := LoadHexProductionOptions(ctx, pool, sid, nil)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(opts[0].MultPerGood["fish"]-1) > 1e-9 {
			t.Fatalf("mult=%v", opts[0].MultPerGood)
		}
	}
	placeHexGubbe(t, pool, sid, 1, c, "fish")
	if err := RecomputeProduction(ctx, pool, sid); err != nil {
		t.Fatal(err)
	}
	var fishRate float64
	if err := pool.QueryRow(ctx, `SELECT rate FROM settlement_goods WHERE settlement_id=$1 AND good_key='fish'`, sid).Scan(&fishRate); err != nil {
		t.Fatal(err)
	}
	if math.Abs(fishRate-1) > 1e-9 {
		t.Fatalf("actual staffed L3 deep rate=%v want%v", fishRate, 1)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM buildings WHERE settlement_id=$1 AND building_type='harbour'`, sid); err != nil {
		t.Fatal(err)
	}
	lifted, err := LiftPlacementsAfterDemolish(ctx, tx, sid, uuid.Nil, "harbour", nil, nil)
	if err != nil || len(lifted) != 1 || lifted[0] != 1 {
		t.Fatalf("lifted=%v err=%v", lifted, err)
	}
}

func TestBayLoadedOwnerCountsRiversButNotMissingMap(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	sid := seedFullRingFixture(t, 100, 1000, "deep_sea")
	var wid uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT world_id FROM settlements WHERE id=$1`, sid).Scan(&wid); err != nil {
		t.Fatal(err)
	}
	c := hexgrid.Coord{Q: 1, R: 0}
	if _, err := pool.Exec(ctx, `UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id=$1 AND q=1 AND r=0`, wid); err != nil {
		t.Fatal(err)
	}
	// Its four land sides include a river and ford. The remaining two stay water.
	neighbors := hexgrid.Neighbors(c)
	for i, terrain := range []string{"river", "river_ford", "plains", "hills"} {
		n := neighbors[i]
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,$3,$4) ON CONFLICT(world_id,q,r) DO UPDATE SET terrain=EXCLUDED.terrain`, wid, n.Q, n.R, terrain); err != nil {
			t.Fatal(err)
		}
	}
	tiles, err := loadHexRuleTiles(ctx, pool, wid, []hexgrid.Coord{c})
	if err != nil {
		t.Fatal(err)
	}
	if p, m := tiles[c].goodPlaces(nil, "fish"); p != 2 || m != 2 {
		t.Fatalf("river+ford bay places=%d want2", p)
	}
	fx, err := HexBuildEffects(ctx, pool, "harbour", wid, c)
	if err != nil || len(fx) != 0 {
		t.Fatalf("bay effects=%+v err=%v", fx, err)
	}
	// Remove one land neighbor: absent map is not an extra land side.
	n := neighbors[0]
	if _, err := pool.Exec(ctx, `DELETE FROM map_tiles WHERE world_id=$1 AND q=$2 AND r=$3`, wid, n.Q, n.R); err != nil {
		t.Fatal(err)
	}
	tiles, err = loadHexRuleTiles(ctx, pool, wid, []hexgrid.Coord{c})
	if err != nil {
		t.Fatal(err)
	}
	if p, m := tiles[c].goodPlaces(nil, "fish"); p != 2 || m != 1 {
		t.Fatalf("three land+missing map places=%d want2", p)
	}
}

func TestFinalFishingNumbers(t *testing.T) {
	for _, tc := range []struct {
		terrain string
		land    int
		level   int
		places  int
		mult    float64
	}{
		{"coastal_sea", 0, 0, 2, 1}, {"coastal_sea", 0, 3, 2, 1},
		{"coastal_sea", 4, 0, 2, 2}, {"coastal_sea", 4, 3, 2, 2},
		{"deep_sea", 0, 0, 0, 1}, {"deep_sea", 0, 1, 1, 1}, {"deep_sea", 0, 2, 2, 1}, {"deep_sea", 0, 3, 4, 1},
		{"river", 6, 3, 1, 1}, {"river_ford", 6, 3, 1, 1},
	} {
		tile := hexRuleTile{rules: rulesWithLandNeighbors(tc.terrain, false, false, false, false, tc.land)}
		p, m := tile.goodPlaces(map[string]int{"harbour": tc.level}, "fish")
		if p != tc.places || m != tc.mult {
			t.Errorf("%s land%d L%d=%d x%v want%d x%v", tc.terrain, tc.land, tc.level, p, m, tc.places, tc.mult)
		}
	}
}
