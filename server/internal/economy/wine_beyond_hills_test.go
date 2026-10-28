package economy

import (
	"context"
	"math"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// wineRateEps tolerates the float64 non-associativity between migration 071's
// ×60 and migration 109's ×24 rescale: 0.02*60*24 and the decimal literal 28.8
// round to adjacent float64 values (≈3.5e-15 apart here), not identically —
// two sequential runtime multiplications don't associate bit-for-bit with a
// single decimal-literal conversion. Far tighter than any real rate mistake
// (wrong terrain/building) would produce.
const wineRateEps = 1e-9

// Migration 224 (Timothy 2026-10-28) reverses the suite below: wine is hills-only
// again and is no longer food or temple upkeep. Historical context:
// regression + proof suite for migration 103 (server/druvor-utanfor-hills,
// Timothy 2026-07-28: "vi får nog acceptera att det går att odla druvor
// överallt"). Before this migration, wine's ONLY production_rules rows were
// on terrain_type='hills' (mig 008 + 019) — a settlement whose 7-hex
// catchment held zero hills tiles could never produce wine, which in turn
// starved its temple (kharis.OfferWinePerTemple, fed from the settlement's
// OWN stock) and locked kult for good.

// wineRulesFor reads every production_rules row for (terrain, 'wine'),
// keyed by building_type ("" for the NULL/field row), so a test can assert
// the exact rate_per_tick set without caring about row order.
func wineRulesFor(t *testing.T, terrain string) map[string]float64 {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	rows, err := pool.Query(ctx,
		`SELECT COALESCE(building_type, ''), rate_per_tick
		 FROM production_rules WHERE terrain_type = $1 AND good_key = 'wine'`,
		terrain,
	)
	if err != nil {
		t.Fatalf("query wine rules for %s: %v", terrain, err)
	}
	defer rows.Close()
	got := make(map[string]float64)
	for rows.Next() {
		var bt string
		var rate float64
		if err := rows.Scan(&bt, &rate); err != nil {
			t.Fatalf("scan wine rule: %v", err)
		}
		got[bt] = rate
	}
	return got
}

// TestProductionRules_HillsWineRatesUnchanged is AK3: hills keeps the
// overhand exactly as it was RELATIVE TO THE OTHER TERRAINS — migration 103
// must not touch the three rows migrations 008 and 019 wrote (unchanged by
// the *wine* slice). Migration 109 (2026-08-06, tick-is-the-day) later
// scaled every production_rules.rate_per_tick ×24 uniformly, so the absolute
// values below are the mig-008/019 originals ×24; the ratios between hills'
// three building rows are exactly what "unchanged" still asserts.
func TestProductionRules_HillsWineRatesUnchanged(t *testing.T) {
	got := wineRulesFor(t, "hills")
	// mig 155: per gubbe (÷ the fallback cap 2), farm row gone (farm no longer touches wine).
	want := map[string]float64{"": 1.0, "winery": 2.5}
	if len(got) != len(want) {
		t.Fatalf("hills wine rules = %v, want %v", got, want)
	}
	for bt, rate := range want {
		if math.Abs(got[bt]-rate) > wineRateEps {
			t.Errorf("hills/%q wine rate = %.4f, want %.4f", bt, got[bt], rate)
		}
	}
}

// TestProductionRules_WineOnlyOnHills: migration 224 (Timothy 2026-10-28, H1)
// undoes migrations 103/105 — wine grows only on hills again, so it is worth
// trading ("nånting som produceras överallt kommer aldrig att handlas").
func TestProductionRules_WineOnlyOnHills(t *testing.T) {
	for _, terrain := range []string{
		"plains", "river_valley", "scrub_maquis",
		"semi_desert", "forest_olive_grove", "forest_cedar",
		"mountain_limestone", "mountain_red", "coastal_sea", "deep_sea",
		"river", "river_delta", "coast_beach",
	} {
		if rules := wineRulesFor(t, terrain); len(rules) != 0 {
			t.Errorf("%s must have no wine production_rules row, found %v", terrain, rules)
		}
	}
}

// TestProductionRules_OilOutsideGroveIsAThird: migration 224 (Timothy
// 2026-10-28, "samma som med virke") — oil outside the olive grove is a third
// of the grove's, field and press alike; the grove itself is untouched.
func TestProductionRules_OilOutsideGroveIsAThird(t *testing.T) {
	pool := testPool(t)
	for _, c := range []struct {
		terrain, building string
		want              float64
	}{
		{"forest_olive_grove", "", 1}, {"forest_olive_grove", "olive_press", 5.0 / 3},
		{"plains", "", 0.33}, {"plains", "olive_press", 0.55},
		{"hills", "", 0.33}, {"hills", "olive_press", 0.55},
	} {
		var got float64
		if err := pool.QueryRow(context.Background(), `SELECT rate_per_tick FROM production_rules
 WHERE good_key='oil' AND terrain_type=$1 AND COALESCE(building_type,'')=$2`, c.terrain, c.building).Scan(&got); err != nil {
			t.Fatalf("%s/%q: %v", c.terrain, c.building, err)
		}
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s/%q oil rate = %.4f, want %.4f", c.terrain, c.building, got, c.want)
		}
	}
}

// plainsCatchmentFixture builds an active world + one settlement whose
// 7-hex catchment is entirely plains (own hex + 6 neighbours) — the exact
// "inland city, no hills anywhere" scenario the contract's player truth
// names. Mirrors recomputeFixture (recompute_floor_test.go) but on plains.
func plainsCatchmentFixture(t *testing.T, currentTick, pop int) (settlementID uuid.UUID) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-wine-%'`,
	); err != nil {
		t.Fatalf("archive leftover test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-wine-"+uuid.New().String(), currentTick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"wine-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}

	// Own hex + 6 neighbours, ALL plains — no hills anywhere in reach.
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 0, 0, 'plains')`,
		worldID,
	); err != nil {
		t.Fatalf("seed own-hex tile: %v", err)
	}
	for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'plains')`,
			worldID, d[0], d[1],
		); err != nil {
			t.Fatalf("seed catchment tile: %v", err)
		}
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Ampelopolis', 'achaean', $3, 'capital', true, $4) RETURNING id`,
		worldID, provinceID, ownerID, pop,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}
	return settlementID
}

// riverValleyCatchmentFixture mirrors plainsCatchmentFixture exactly but
// seeds the 7-hex catchment (own hex + 6 neighbours) as river_valley instead
// of plains — migration 105's "inland city on the floodplain" scenario.
func riverValleyCatchmentFixture(t *testing.T, currentTick, pop int) (settlementID uuid.UUID) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-wine-%'`,
	); err != nil {
		t.Fatalf("archive leftover test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-wine-"+uuid.New().String(), currentTick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"wine-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'river_valley') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}

	// Own hex + 6 neighbours, ALL river_valley — no hills or plains anywhere in reach.
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 0, 0, 'river_valley')`,
		worldID,
	); err != nil {
		t.Fatalf("seed own-hex tile: %v", err)
	}
	for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'river_valley')`,
			worldID, d[0], d[1],
		); err != nil {
			t.Fatalf("seed catchment tile: %v", err)
		}
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Potamopolis', 'achaean', $3, 'capital', true, $4) RETURNING id`,
		worldID, provinceID, ownerID, pop,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}
	return settlementID
}

// TestRecomputeProduction_NoWineOnRiverValleyOnlyCatchment: since migration 224 a
// city with no hills in reach makes no wine, even with a gubbe set on it.
func TestRecomputeProduction_NoWineOnRiverValleyOnlyCatchment(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := riverValleyCatchmentFixture(t, 100, 100)
	placeHexGubbe(t, pool, settlementID, 1, hexgrid.Coord{Q: 1, R: 0}, "wine")
	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}
	var wineRate float64
	_ = pool.QueryRow(ctx, `SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'wine'`, settlementID).Scan(&wineRate)
	if wineRate != 0 {
		t.Errorf("wine rate without hills = %.6f, want 0", wineRate)
	}
}

// TestRecomputeProduction_NoWineOnPlainsOnlyCatchment: since migration 224 a
// city with no hills in reach makes no wine, even with a gubbe set on it.
func TestRecomputeProduction_NoWineOnPlainsOnlyCatchment(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := plainsCatchmentFixture(t, 100, 100)
	placeHexGubbe(t, pool, settlementID, 1, hexgrid.Coord{Q: 1, R: 0}, "wine")
	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}
	var wineRate float64
	_ = pool.QueryRow(ctx, `SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'wine'`, settlementID).Scan(&wineRate)
	if wineRate != 0 {
		t.Errorf("wine rate without hills = %.6f, want 0", wineRate)
	}
}
