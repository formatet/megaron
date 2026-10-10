package economy

import (
	"context"
	"math"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// The "tal före → efter" table of megaron_plan_byggnadsregeln.md, pinned at
// level 1 (per worker x places) and x2.4 / x3.1 at levels 2 / 3. r0 are the
// production_rules base rates (mig 155), places come from hexRules.
func TestRuleTable_LevelOneNumbersAndLevelMultipliers(t *testing.T) {
	cases := []struct {
		name               string
		terrain            string
		cu, tin, ag        bool
		good               string
		r0                 float64
		wantPlacesWithout  int
		wantPlacesWith     int
		wantRateL1         float64 // per worker, rounded to 2 decimals
		wantRateWithoutStr string
	}{
		{"grain plains", "plains", false, false, false, GoodGrain, 1.0, 4, 8, 1.70, "1.0"},
		{"grain hills", "hills", false, false, false, GoodGrain, 1.0 / 3, 1, 5, 0.57, "0.33"},
		{"grain river_valley", "river_valley", false, false, false, GoodGrain, 5.0 / 3, 2, 6, 2.83, "1.67"},
		{"grain river_delta", "river_delta", false, false, false, GoodGrain, 8.0 / 3, 3, 7, 4.53, "2.67"},
		{"cedar", "forest_cedar", false, false, false, "cedar", 1.0, 3, 7, 1.70, "1.0"},
		{"fish coastal", "coastal_sea", false, false, false, "fish", 1.0, 2, 6, 1.70, "1.0"},
		{"copper", "hills", true, false, false, "copper", 1.0, 1, 5, 1.70, "1.0"},
		{"tin", "mountain_limestone", false, true, false, "tin", 1.0, 1, 5, 1.70, "1.0"},
		{"timber olive grove", "forest_olive_grove", false, false, false, "timber", 1.0, 1, 5, 1.70, "1.0"},
		{"stone hills", "hills", false, false, false, "stone", 1.0, 2, 6, 1.70, "1.0"},
		{"stone plains (fieldstone)", "plains", false, false, false, "stone", 0.5, 1, 5, 0.85, "0.5"},
		{"stone limestone", "mountain_limestone", false, false, false, "stone", 2.0, 2, 6, 3.40, "2.0"},
	}
	for _, c := range cases {
		places0, mult0 := hexGoodPlaces(c.terrain, c.cu, c.tin, c.ag, map[string]int{}, c.good)
		if places0 != c.wantPlacesWithout || mult0 != 1.0 {
			t.Errorf("%s unbuilt: %d x %.2f, want %d x 1.0", c.name, places0, mult0, c.wantPlacesWithout)
		}
		var building string
		for _, r := range hexRules(c.terrain, c.cu, c.tin, c.ag) {
			if r.good == c.good {
				building = r.building
			}
		}
		for level, wantMult := range map[int]float64{1: 1.7, 2: 2.4, 3: 3.1} {
			places, mult := hexGoodPlaces(c.terrain, c.cu, c.tin, c.ag, map[string]int{building: level}, c.good)
			if places != c.wantPlacesWith {
				t.Errorf("%s L%d: places %d, want %d (same at every level)", c.name, level, places, c.wantPlacesWith)
			}
			if math.Abs(mult-wantMult) > 1e-9 {
				t.Errorf("%s L%d: mult %.2f, want %.1f", c.name, level, mult, wantMult)
			}
			if level == 1 {
				if got := math.Round(c.r0*mult*100) / 100; math.Abs(got-c.wantRateL1) > 1e-9 {
					t.Errorf("%s L1 rate/worker %.2f, want %.2f", c.name, got, c.wantRateL1)
				}
				if fmtRate(c.r0) != c.wantRateWithoutStr {
					t.Errorf("%s fmtRate(r0) = %s, want %s", c.name, fmtRate(c.r0), c.wantRateWithoutStr)
				}
			}
		}
	}
	// Silver: 1 + 4 places, and the mine row keeps today's level-1 figure
	// (2.304 per worker x 5 places = 11.52, hills).
	places, mult := hexGoodPlaces("hills", false, false, true, map[string]int{"mine": 1}, "silver")
	if places != 5 || math.Abs(11.52/5.0/1.7*mult-2.304) > 1e-9 {
		t.Errorf("silver on hills with mine L1: %d places, rate %.4f, want 5 places and 2.304", places, 11.52/5.0/1.7*mult)
	}
	// A good without a rule keeps the flat fallback and no multiplier.
	if p, m := hexGoodPlaces("hills", false, false, false, map[string]int{"farm": 3}, "oil"); p != HexFallbackCap || m != 1.0 {
		t.Errorf("oil on hills: %d x %.1f, want %d x 1.0", p, m, HexFallbackCap)
	}
}

func TestEffectText_Formats(t *testing.T) {
	farm := []HexBuildEffect{{Good: "grain", PlacesWithout: 4, RateWithout: 1.0, PlacesWith: 8, RateByLevel: [3]float64{1.7, 2.4, 3.1}}}
	if got, want := BuildEffectText(farm), "grain production ×1.7 · space for 4 more workers"; got != want {
		t.Errorf("BuildEffectText = %q, want %q", got, want)
	}
	if got, want := UpgradeEffectText(farm, 1), "grain production ×1.7 → ×2.4"; got != want {
		t.Errorf("UpgradeEffectText L1 = %q, want %q", got, want)
	}
	if got := UpgradeEffectText(farm, 3); got != "" {
		t.Errorf("UpgradeEffectText at max level = %q, want empty", got)
	}
	silver := []HexBuildEffect{{Good: "silver", PlacesWith: 5, RateByLevel: [3]float64{2.304, 3.25, 4.2}, NeedsBuilding: true}}
	if got, want := BuildEffectText(silver), "silver can be mined · space for 5 workers"; got != want {
		t.Errorf("silver BuildEffectText = %q, want %q", got, want)
	}
	two := append(append([]HexBuildEffect{}, farm...), silver...)
	if got, want := BuildEffectText(two), "grain production ×1.7 · space for 4 more workers; silver can be mined · space for 5 workers"; got != want {
		t.Errorf("joined BuildEffectText = %q, want %q", got, want)
	}
}

// Parity: the numbers in the effect line are what RecomputeProduction makes of a
// fully staffed hex (grain, farm L1 on plains: 8 x 1.7, plus the city hex ration).
func TestEffectParity_EffectTextMatchesRecompute(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := seedFullRingFixture(t, 100, 500, "plains")
	hex := hexgrid.Ring(hexgrid.Coord{Q: 0, R: 0}, hexgrid.CatchmentRadius)[0]

	fx, err := HexBuildEffects(ctx, pool, "farm", "plains", false, false, false)
	if err != nil {
		t.Fatalf("HexBuildEffects: %v", err)
	}
	if got, want := BuildEffectText(fx), "grain production ×1.7 · space for 4 more workers"; got != want {
		t.Fatalf("effect = %q, want %q", got, want)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level, hex_q, hex_r) VALUES ($1, 'farm', 1, $2, $3)`,
		settlementID, hex.Q, hex.R); err != nil {
		t.Fatalf("seed farm: %v", err)
	}
	for i := 1; i <= fx[0].PlacesWith; i++ {
		placeHexGubbe(t, pool, settlementID, i, hex, GoodGrain)
	}
	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}
	_, rate := readGood(t, settlementID, GoodGrain)
	want := NearjordGrainPerTick + float64(fx[0].PlacesWith)*fx[0].RateByLevel[0]
	if math.Abs(rate-want) > 1e-6 {
		t.Errorf("grain rate %.4f, want %.4f (ration + %d x %.2f)", rate, want, fx[0].PlacesWith, fx[0].RateByLevel[0])
	}
}

// fynd 2: the starter farm goes where the grain OUTPUT is largest — delta
// (7 x 2.67 x 1.7) before plains (8 x 1.0 x 1.7) — and on plains when there is
// nothing better (so the founding still grants a farm there).
func TestChooseFarmHex_RanksOnOutputNotPlaces(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := seedFullRingFixture(t, 100, 500, "plains")
	ring := hexgrid.Ring(hexgrid.Coord{Q: 0, R: 0}, hexgrid.CatchmentRadius)

	var wid uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT world_id FROM settlements WHERE id = $1`, settlementID).Scan(&wid); err != nil {
		t.Fatalf("world id: %v", err)
	}

	best, ok, err := ChooseFarmHex(ctx, pool, wid, hexgrid.Coord{Q: 0, R: 0}, nil)
	if err != nil || !ok {
		t.Fatalf("plains-only catchment: ok=%v err=%v", ok, err)
	}
	if want := lowestQR(ring); best != want {
		t.Errorf("all plains: picked %v, want the lowest Q then R, %v", best, want)
	}

	delta := ring[7]
	if _, err := pool.Exec(ctx, `UPDATE map_tiles SET terrain = 'river_delta' WHERE world_id = $1 AND q = $2 AND r = $3`, wid, delta.Q, delta.R); err != nil {
		t.Fatalf("set delta: %v", err)
	}
	best, ok, err = ChooseFarmHex(ctx, pool, wid, hexgrid.Coord{Q: 0, R: 0}, nil)
	if err != nil || !ok || best != delta {
		t.Errorf("one delta among plains: picked %v (ok=%v err=%v), want the delta %v", best, ok, err, delta)
	}
}

func lowestQR(hexes []hexgrid.Coord) hexgrid.Coord {
	best := hexes[0]
	for _, c := range hexes {
		if c.Q < best.Q || (c.Q == best.Q && c.R < best.R) {
			best = c
		}
	}
	return best
}

// Lifecycle (fynd 6): ReconcilePlacements deletes placements inside a mine /
// stonequarry / lumbermill, trims a hex above its new places (highest ordinal
// first) and re-places the freed gubbar.
func TestReconcilePlacements_TrimsOverCapAndDropsBuildingPlacements(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := seedFullRingFixture(t, 100, 1500, "plains")
	hex := hexgrid.Ring(hexgrid.Coord{Q: 0, R: 0}, hexgrid.CatchmentRadius)[0]
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level, hex_q, hex_r) VALUES ($1, 'farm', 2, $2, $3), ($1, 'stonequarry', 1, $2, $3)`,
		settlementID, hex.Q, hex.R); err != nil {
		t.Fatalf("seed buildings: %v", err)
	}
	// Old world: 10 grain on the farm-L2 hex, plus 2 gubbar inside the quarry.
	for i := 1; i <= 10; i++ {
		placeHexGubbe(t, pool, settlementID, i, hex, GoodGrain)
	}
	placeBuildingGubbe(t, pool, settlementID, 11, "stonequarry", "stone")
	placeBuildingGubbe(t, pool, settlementID, 12, "stonequarry", "stone")

	removed, replaced, err := ReconcilePlacements(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("ReconcilePlacements: %v", err)
	}
	if removed != 4 {
		t.Errorf("removed %d placements, want 4 (2 over-cap grain + 2 quarry)", removed)
	}
	var grainOnHex, inQuarry, total int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM settlement_placement WHERE settlement_id = $1 AND hex_q = $2 AND hex_r = $3 AND good_key = 'grain'`, settlementID, hex.Q, hex.R).Scan(&grainOnHex)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM settlement_placement WHERE settlement_id = $1 AND target_kind = 'building'`, settlementID).Scan(&inQuarry)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM settlement_placement WHERE settlement_id = $1`, settlementID).Scan(&total)
	if grainOnHex < 8 {
		t.Errorf("grain on the farm hex = %d, want at least the 8 kept", grainOnHex)
	}
	if inQuarry != 0 {
		t.Errorf("%d placements remain inside a stonequarry, want 0", inQuarry)
	}
	if total != 12-removed+replaced {
		t.Errorf("total placements %d, want %d", total, 12-removed+replaced)
	}
	var over int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM (SELECT hex_q, hex_r FROM settlement_placement WHERE settlement_id = $1 AND good_key = 'grain' GROUP BY hex_q, hex_r HAVING count(*) > 8) x`, settlementID).Scan(&over)
	if over != 0 {
		t.Errorf("%d grain hexes still above 8 gubbar", over)
	}
	// Idempotent.
	if r2, _, err := ReconcilePlacements(ctx, pool, settlementID); err != nil || r2 != 0 {
		t.Errorf("second run removed %d (err %v), want 0", r2, err)
	}
}
