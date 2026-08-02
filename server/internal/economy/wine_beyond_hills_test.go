package economy

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// Regression + proof suite for migration 103 (server/druvor-utanfor-hills,
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
// overhand exactly as it was — migration 103 must not touch the three rows
// migrations 008 and 019 wrote. Values per the contract's kodverifierade tal.
func TestProductionRules_HillsWineRatesUnchanged(t *testing.T) {
	got := wineRulesFor(t, "hills")
	want := map[string]float64{"": 1.2, "farm": 2.4, "winery": 3.0}
	if len(got) != len(want) {
		t.Fatalf("hills wine rules = %v, want %v", got, want)
	}
	for bt, rate := range want {
		if got[bt] != rate {
			t.Errorf("hills/%q wine rate = %.4f, want %.4f", bt, got[bt], rate)
		}
	}
}

// TestProductionRules_WineBeyondHills is the migration's core claim: plains
// (half of hills' rate — the fertile plain bears grapes worse than the
// slope) and scrub_maquis (marginal land, no farm row — nobody plows a
// field in the maquis) now carry their own wine rows. This is the red
// baseline the contract calls for: on unmigrated code (only through 102)
// this map is empty and the test fails — see the process report for that
// failing run's captured output.
func TestProductionRules_WineBeyondHills(t *testing.T) {
	plains := wineRulesFor(t, "plains")
	wantPlains := map[string]float64{"": 0.6, "farm": 1.2, "winery": 1.8}
	if len(plains) != len(wantPlains) {
		t.Fatalf("plains wine rules = %v, want %v", plains, wantPlains)
	}
	for bt, rate := range wantPlains {
		if plains[bt] != rate {
			t.Errorf("plains/%q wine rate = %.4f, want %.4f", bt, plains[bt], rate)
		}
	}

	scrub := wineRulesFor(t, "scrub_maquis")
	wantScrub := map[string]float64{"": 0.4, "winery": 1.0}
	if len(scrub) != len(wantScrub) {
		t.Fatalf("scrub_maquis wine rules = %v, want %v", scrub, wantScrub)
	}
	for bt, rate := range wantScrub {
		if scrub[bt] != rate {
			t.Errorf("scrub_maquis/%q wine rate = %.4f, want %.4f", bt, scrub[bt], rate)
		}
	}

	// No wine anywhere else — sea, rivers, deltas, mountains, semi-desert or
	// either forest. The contract was explicit that only three terrains bore
	// grapes as of migration 103; migration 105 (Timothy 2026-08-02) added a
	// fourth, river_valley — see TestProductionRules_WineInRiverValley. river
	// and river_delta stay in this negative list: only the valley itself
	// bears wine, not the river channel or its delta.
	for _, terrain := range []string{
		"semi_desert", "forest_olive_grove", "forest_cedar",
		"mountain_limestone", "mountain_red", "coastal_sea", "deep_sea",
		"river", "river_delta", "coast_beach",
	} {
		if rules := wineRulesFor(t, terrain); len(rules) != 0 {
			t.Errorf("%s must have no wine production_rules row, found %v", terrain, rules)
		}
	}
}

// TestProductionRules_WineInRiverValley is migration 105's core claim:
// river_valley bears wine at the same level as plains (Timothy 2026-08-02 —
// hills keeps the overhand, the floodplain sits at the plain's level, not
// the marginal scrub_maquis level). Exactly three rows, same shape as
// TestProductionRules_WineBeyondHills's plains assertion.
func TestProductionRules_WineInRiverValley(t *testing.T) {
	got := wineRulesFor(t, "river_valley")
	want := map[string]float64{"": 0.6, "farm": 1.2, "winery": 1.8}
	if len(got) != len(want) {
		t.Fatalf("river_valley wine rules = %v, want %v", got, want)
	}
	for bt, rate := range want {
		if got[bt] != rate {
			t.Errorf("river_valley/%q wine rate = %.4f, want %.4f", bt, got[bt], rate)
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
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"wine-"+uuid.New().String(), "wine-"+uuid.New().String()+"@test.invalid",
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
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"wine-"+uuid.New().String(), "wine-"+uuid.New().String()+"@test.invalid",
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

// TestRecomputeProduction_WineOnRiverValleyOnlyCatchment is migration 105's
// end-to-end proof: a city whose entire 7-hex catchment is river_valley —
// no hills or plains tile anywhere — must produce wine at a positive rate
// after RecomputeProduction. This proves the chain migration → rule →
// actual production holds, not just that a production_rules row exists.
func TestRecomputeProduction_WineOnRiverValleyOnlyCatchment(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	settlementID := riverValleyCatchmentFixture(t, /*tick*/ 100, /*pop*/ 100)

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	var wineRate float64
	var found bool
	err := pool.QueryRow(ctx,
		`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'wine'`,
		settlementID,
	).Scan(&wineRate)
	if err == nil {
		found = true
	}
	if !found {
		t.Fatalf("no wine row written for a river_valley-only-catchment settlement after RecomputeProduction " +
			"— migration 105 should have given river_valley a wine production_rules row")
	}
	if wineRate <= 0 {
		t.Errorf("wine rate for a river_valley-only-catchment settlement = %.6f, want > 0", wineRate)
	}
}

// TestRecomputeProduction_WineOnPlainsOnlyCatchment is AK2's core claim: a
// city whose entire 7-hex catchment is plains — no hills tile anywhere —
// must produce wine at a positive rate after RecomputeProduction, and the
// good must land in settlement_goods as producible (bp > 0), which is what
// the GET .../goods handler and `keryx goods`/`keryx build --list` surface
// (see api/handlers/wine_beyond_hills_test.go for the HTTP-level half of
// this proof).
func TestRecomputeProduction_WineOnPlainsOnlyCatchment(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	settlementID := plainsCatchmentFixture(t, /*tick*/ 100, /*pop*/ 100)

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	var wineRate float64
	var found bool
	err := pool.QueryRow(ctx,
		`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'wine'`,
		settlementID,
	).Scan(&wineRate)
	if err == nil {
		found = true
	}
	if !found {
		t.Fatalf("no wine row written for a plains-only-catchment settlement after RecomputeProduction " +
			"— on unmigrated code (pre-103) this is exactly the bug: wine had no production_rules row " +
			"off hills, so RecomputeProduction never wrote one")
	}
	if wineRate <= 0 {
		t.Errorf("wine rate for a plains-only-catchment settlement = %.6f, want > 0", wineRate)
	}
}
