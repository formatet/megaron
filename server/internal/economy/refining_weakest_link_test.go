package economy

import (
	"context"
	"math"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

const weakestLinkEps = 1e-9

// groveCatchmentFixture mirrors plainsCatchmentFixture (wine_beyond_hills_test.go)
// but seeds the 19-hex catchment (own hex + full ring) as forest_olive_grove —
// oil's terrain. Own hex uses the settlement's actual coordinates so
// hexgrid.Ring finds the neighbours; RecomputeProduction only reads the ring,
// not the settlement's own hex, so its terrain is irrelevant here but seeded
// for completeness.
func groveCatchmentFixture(t *testing.T, currentTick, pop int) (settlementID uuid.UUID) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-refine-%'`,
	); err != nil {
		t.Fatalf("archive leftover test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-refine-"+uuid.New().String(), currentTick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"refine-"+uuid.New().String(), "refine-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'forest_olive_grove') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 0, 0, 'forest_olive_grove')`,
		worldID,
	); err != nil {
		t.Fatalf("seed own-hex tile: %v", err)
	}
	for _, hex := range hexgrid.Ring(hexgrid.Coord{Q: 0, R: 0}, hexgrid.CatchmentRadius) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'forest_olive_grove')`,
			worldID, hex.Q, hex.R,
		); err != nil {
			t.Fatalf("seed catchment tile (%d,%d): %v", hex.Q, hex.R, err)
		}
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Elaiopolis', 'achaean', $3, 'capital', true, $4) RETURNING id`,
		worldID, provinceID, ownerID, pop,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}
	return settlementID
}

func goodRate(t *testing.T, settlementID uuid.UUID, good string) float64 {
	t.Helper()
	pool := testPool(t)
	var rate float64
	err := pool.QueryRow(context.Background(),
		`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
		settlementID, good,
	).Scan(&rate)
	if err != nil {
		return 0 // no row = never produced = rate 0, same as the player sees
	}
	return rate
}

func goodAmount(t *testing.T, settlementID uuid.UUID, good string) float64 {
	t.Helper()
	pool := testPool(t)
	var amount float64
	err := pool.QueryRow(context.Background(),
		`SELECT settled(amount, rate, calc_tick) FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
		settlementID, good,
	).Scan(&amount)
	if err != nil {
		return 0
	}
	return amount
}

// ---- oil: extraction + refining, strict weakest link (P6, Timothy 2026-08-08) ----

// TestRecomputeProduction_OilBaselineWithoutPress: no olive_press built at all
// — the extraction gubbe's unrefined baseline (mig 092, untouched by P6) must
// still flow. This is the "never below the pre-P6 floor" guard.
func TestRecomputeProduction_OilBaselineWithoutPress(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := groveCatchmentFixture(t, 100, 100)
	placeHexGubbe(t, pool, settlementID, 1, hexgrid.Coord{Q: 1, R: 0}, GoodOil)

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	// baseline rate 43.2 (mig 092's 1.8, ×24 by mig 109 — read live, not from
	// 092's own comment), HexFallbackCap 2, 1 placed -> 21.6.
	// 21.6 → 1.0 (mig 136, oil ÷21.6): baseline rate is now 2.0 (43.2/21.6),
	// (2.0/2)*1 = 1.0.
	want := 1.0
	if got := goodRate(t, settlementID, GoodOil); math.Abs(got-want) > weakestLinkEps {
		t.Errorf("oil rate without any press = %.6f, want %.6f (baseline only)", got, want)
	}
}

// TestRecomputeProduction_OilBoostRequiresPlacedPressWorker is the core
// regression guard for Timothy's 2026-08-08 decision: a BUILT but UNSTAFFED
// olive_press must contribute NOTHING beyond the baseline — pre-P6 this same
// setup gave the full boosted rate from the building's mere existence.
func TestRecomputeProduction_OilBoostRequiresPlacedPressWorker(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := groveCatchmentFixture(t, 100, 100)
	placeHexGubbe(t, pool, settlementID, 1, hexgrid.Coord{Q: 1, R: 0}, GoodOil)
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'olive_press', 1)`,
		settlementID,
	); err != nil {
		t.Fatalf("build olive_press: %v", err)
	}

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	want := 1.0 // 21.6 → 1.0 (mig 136, oil ÷21.6) — same baseline as the no-press case, an unstaffed press adds zero
	if got := goodRate(t, settlementID, GoodOil); math.Abs(got-want) > weakestLinkEps {
		t.Errorf("oil rate with an UNSTAFFED press = %.6f, want %.6f — a built-but-empty press "+
			"must not boost production (Timothy 2026-08-08: strict weakest link)", got, want)
	}
}

// TestRecomputeProduction_OilBoostExtractionLimited staffs the press FULLY
// (level 2, both slots filled — refining capacity at its ceiling, 72.0) but
// leaves extraction at half capacity (1 of 2 hex slots) — the boost realized
// must be capped by the WEAKER side, extraction, not the (over-provisioned)
// refining side.
func TestRecomputeProduction_OilBoostExtractionLimited(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := groveCatchmentFixture(t, 100, 100)
	placeHexGubbe(t, pool, settlementID, 1, hexgrid.Coord{Q: 1, R: 0}, GoodOil)
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'olive_press', 2)`,
		settlementID,
	); err != nil {
		t.Fatalf("build olive_press L2: %v", err)
	}
	placeBuildingGubbe(t, pool, settlementID, 2, "olive_press", GoodOil)
	placeBuildingGubbe(t, pool, settlementID, 3, "olive_press", GoodOil)

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	// baseline (43.2/2)*1=21.6 ; boostPotential (72.0/2)*1=36.0 ;
	// refiningCapacity (72.0/2)*2=72.0 ; realized boost = min(36.0,72.0)=36.0
	// -> total 57.6.
	// 57.6 → 2.6666666666666665 (mig 136, oil ÷21.6): baseline (2.0/2)*1=1.0 ;
	// boostPotential (3.3333.../2)*1=1.6666... ; refiningCapacity
	// (3.3333.../2)*2=3.3333... ; realized boost = min(1.6666...,3.3333...)=1.6666...
	// -> total 2.6666666666666665.
	want := 57.6 / 21.6
	if got := goodRate(t, settlementID, GoodOil); math.Abs(got-want) > weakestLinkEps {
		t.Errorf("oil rate with over-provisioned refining = %.6f, want %.6f "+
			"(boost must be capped by the WEAKER extraction side, not the refining side)", got, want)
	}
}

// TestRecomputeProduction_OilFullyStaffedMatchesPreP6Ceiling: with BOTH sides
// fully staffed (2 extraction, 2 refining at level 2), the total must equal
// the exact pre-P6 combined rate (baseline 43.2 + boost 72.0 = 115.2 — mig
// 092's worked example, 4.8, ×24 by mig 109) — P6 changes WHO must be placed
// to reach that ceiling, not the ceiling itself.
func TestRecomputeProduction_OilFullyStaffedMatchesPreP6Ceiling(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := groveCatchmentFixture(t, 100, 100)
	placeHexGubbe(t, pool, settlementID, 1, hexgrid.Coord{Q: 1, R: 0}, GoodOil)
	placeHexGubbe(t, pool, settlementID, 2, hexgrid.Coord{Q: 1, R: 0}, GoodOil)
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'olive_press', 2)`,
		settlementID,
	); err != nil {
		t.Fatalf("build olive_press L2: %v", err)
	}
	placeBuildingGubbe(t, pool, settlementID, 3, "olive_press", GoodOil)
	placeBuildingGubbe(t, pool, settlementID, 4, "olive_press", GoodOil)

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	want := 115.2 / 21.6 // 115.2 → 5.333333333333333 (mig 136, oil ÷21.6)
	if got := goodRate(t, settlementID, GoodOil); math.Abs(got-want) > weakestLinkEps {
		t.Errorf("fully staffed oil rate = %.6f, want %.6f (migration 092's pre-P6 ceiling)", got, want)
	}
}

// ---- wine: winery gets the same strict gate; farm's wine tier is untouched ----

// TestRecomputeProduction_WineWineryBoostRequiresPlacedWorker mirrors the oil
// regression guard on wine's winery tier, and proves farm's independent wine
// bonus (mig 008/103 — unrelated to §10.2's press/winery pair) is NOT gated:
// a farm alone still gives its boost with no vinmakare involved at all.
func TestRecomputeProduction_WineWineryBoostRequiresPlacedWorker(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := plainsCatchmentFixture(t, 100, 100)
	placeHexGubbe(t, pool, settlementID, 1, hexgrid.Coord{Q: 1, R: 0}, GoodWine)
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'farm', 1), ($1, 'winery', 1)`,
		settlementID,
	); err != nil {
		t.Fatalf("build farm+winery: %v", err)
	}

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	// plains wine (live rates, mig 103+109): baseline 14.4, +farm 28.8 (both
	// terrain+building rows, UNGATED — only winery is in weakestLinkGoods),
	// cap=HexFallbackCap=2, 1 placed: baseline (14.4/2)*1=7.2, farm-boost
	// (28.8/2)*1=14.4. winery-boost is 0 (unstaffed) -> total 21.6.
	// 21.6 → 1.5 (mig 136, wine ÷14.4): baseline+farm-boost = 1.0+0.5 = 1.5.
	want := 21.6 / 14.4
	if got := goodRate(t, settlementID, GoodWine); math.Abs(got-want) > weakestLinkEps {
		t.Errorf("wine rate with unstaffed winery = %.6f, want %.6f "+
			"(farm's own wine bonus must still apply; winery's must not)", got, want)
	}
}

// TestRecomputeProduction_WineWineryBoostRealizedWhenStaffed: staffing the
// winery adds its own min(boost,refining) term on top of baseline+farm.
func TestRecomputeProduction_WineWineryBoostRealizedWhenStaffed(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := plainsCatchmentFixture(t, 100, 100)
	placeHexGubbe(t, pool, settlementID, 1, hexgrid.Coord{Q: 1, R: 0}, GoodWine)
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'farm', 1), ($1, 'winery', 1)`,
		settlementID,
	); err != nil {
		t.Fatalf("build farm+winery: %v", err)
	}
	placeBuildingGubbe(t, pool, settlementID, 2, "winery", GoodWine)

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	// baseline+farm = 21.6 (as above). winery-boostPotential (43.2/2)*1=21.6;
	// refiningCapacity (72.0/1)*1=72.0 (level 1, 1 slot);
	// realized = min(21.6,72.0)=21.6 -> total 43.2.
	// 43.2 → 3.0 (mig 136, wine ÷14.4): baseline+farm 1.5 + winery-boost 1.5 = 3.0.
	want := 43.2 / 14.4
	if got := goodRate(t, settlementID, GoodWine); math.Abs(got-want) > weakestLinkEps {
		t.Errorf("wine rate with staffed winery = %.6f, want %.6f", got, want)
	}
}

// ---- bronze: stock drain, not a rate — copper/tin stay real, tradeable goods ----

// TestRecomputeProduction_BronzeZeroWithoutFoundry: no foundry at all —
// bronze must stay untouched (no row / rate 0), and copper/tin stock must be
// completely unaffected (they remain sellable even though bronze can't be
// made from them here).
func TestRecomputeProduction_BronzeZeroWithoutFoundry(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := plainsCatchmentFixture(t, 100, 100)
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'copper', 90, 0, 1000000, 100), ($1, 'tin', 10, 0, 1000000, 100)`,
		settlementID,
	); err != nil {
		t.Fatalf("seed copper/tin stock: %v", err)
	}

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	if got := goodAmount(t, settlementID, GoodCopper); math.Abs(got-90) > weakestLinkEps {
		t.Errorf("copper stock without a foundry = %.6f, want 90 (untouched)", got)
	}
	if got := goodAmount(t, settlementID, GoodTin); math.Abs(got-10) > weakestLinkEps {
		t.Errorf("tin stock without a foundry = %.6f, want 10 (untouched)", got)
	}
	if got := goodAmount(t, settlementID, GoodBronze); got != 0 {
		t.Errorf("bronze amount without a foundry = %.6f, want 0", got)
	}
}

// TestRecomputeProduction_BronzeZeroWithUnstaffedFoundry: foundry built but
// no gjutare placed — refiningCapacity is 0, so bronze must stay 0 and
// copper/tin stock must stay untouched (the stock-drain must never run
// without a placed refining gubbe, matching the strict weakest link decided
// for oil/wine).
func TestRecomputeProduction_BronzeZeroWithUnstaffedFoundry(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := plainsCatchmentFixture(t, 100, 100)
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'foundry', 1)`,
		settlementID,
	); err != nil {
		t.Fatalf("build foundry: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'copper', 90, 0, 1000000, 100), ($1, 'tin', 10, 0, 1000000, 100)`,
		settlementID,
	); err != nil {
		t.Fatalf("seed copper/tin stock: %v", err)
	}

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	if got := goodAmount(t, settlementID, GoodCopper); math.Abs(got-90) > weakestLinkEps {
		t.Errorf("copper stock with an unstaffed foundry = %.6f, want 90 (untouched)", got)
	}
	if got := goodAmount(t, settlementID, GoodBronze); got != 0 {
		t.Errorf("bronze amount with an unstaffed foundry = %.6f, want 0", got)
	}
}

// TestRecomputeProduction_BronzeRefiningLimited: plenty of copper AND tin,
// but only a level-1 foundry (1 slot, refiningCapacity=1.0/tick) — bronze
// produced must be capped at the refining ceiling, and ingredients consumed
// at exactly the 9:1 ratio (mig 099, read live from recipes/recipe_ingredients).
func TestRecomputeProduction_BronzeRefiningLimited(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := plainsCatchmentFixture(t, 100, 100)
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'foundry', 1)`,
		settlementID,
	); err != nil {
		t.Fatalf("build foundry: %v", err)
	}
	placeBuildingGubbe(t, pool, settlementID, 1, "foundry", GoodBronze)
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'copper', 900, 0, 1000000, 100), ($1, 'tin', 100, 0, 1000000, 100)`,
		settlementID,
	); err != nil {
		t.Fatalf("seed copper/tin stock: %v", err)
	}

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	// refiningCapacity = (1.0/1)*1 = 1.0 bronze; copper allows 900/9=100, tin
	// allows 100/1=100 — refining is the binding constraint at 1.0.
	if got := goodAmount(t, settlementID, GoodBronze); math.Abs(got-1.0) > weakestLinkEps {
		t.Errorf("bronze produced = %.6f, want 1.0 (refining-capacity limited)", got)
	}
	if got := goodAmount(t, settlementID, GoodCopper); math.Abs(got-891) > weakestLinkEps {
		t.Errorf("copper stock after smelting = %.6f, want 891 (900 - 9*1.0)", got)
	}
	if got := goodAmount(t, settlementID, GoodTin); math.Abs(got-99) > weakestLinkEps {
		t.Errorf("tin stock after smelting = %.6f, want 99 (100 - 1*1.0)", got)
	}
}

// TestRecomputeProduction_BronzeIngredientLimited: refining capacity and tin
// are abundant, but copper is scarce (5, less than the 9 one bronze needs) —
// bronze produced must be capped by copper/9, proving the weakest link picks
// whichever ingredient (or refining) is smallest, not always refining.
func TestRecomputeProduction_BronzeIngredientLimited(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := plainsCatchmentFixture(t, 100, 100)
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'foundry', 1)`,
		settlementID,
	); err != nil {
		t.Fatalf("build foundry: %v", err)
	}
	placeBuildingGubbe(t, pool, settlementID, 1, "foundry", GoodBronze)
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'copper', 5, 0, 1000000, 100), ($1, 'tin', 100, 0, 1000000, 100)`,
		settlementID,
	); err != nil {
		t.Fatalf("seed copper/tin stock: %v", err)
	}

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	want := 5.0 / 9.0
	if got := goodAmount(t, settlementID, GoodBronze); math.Abs(got-want) > weakestLinkEps {
		t.Errorf("bronze produced = %.6f, want %.6f (copper-limited: 5/9)", got, want)
	}
	if got := goodAmount(t, settlementID, GoodCopper); math.Abs(got) > weakestLinkEps {
		t.Errorf("copper stock after smelting = %.6f, want ~0 (fully consumed)", got)
	}
	if got := goodAmount(t, settlementID, GoodTin); math.Abs(got-(100-want)) > weakestLinkEps {
		t.Errorf("tin stock after smelting = %.6f, want %.6f", got, 100-want)
	}
}
