package economy

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
)

// TestPrunePlacementsToPopulation_S1bOrder is S1b's own acceptance criterion
// (megaron_plan_placeringsbeskarning.md AC2, megaron_plan_foda_konsistens.md
// §S1b LOCKED): a settlement with 1 hantverk/förädling placement, 1 tempel
// placement and 2 food placements, pruned down to floor(pop/100)=2, loses
// the craft and temple rows and keeps BOTH food rows.
func TestPrunePlacementsToPopulation_S1bOrder(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	settlementID := seedFullRingFixture(t, 100 /*pop*/, 250, "plains") // floor(250/100) = 2
	placeBuildingGubbe(t, pool, settlementID, 1, "foundry", GoodBronze)
	placeBuildingGubbe(t, pool, settlementID, 2, "temple", GoodCult)
	placeHexGubbe(t, pool, settlementID, 3, hexRing0(t)[0], GoodGrain)
	placeHexGubbe(t, pool, settlementID, 4, hexRing0(t)[1], GoodGrain)

	pruned, err := PrunePlacementsToPopulation(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("PrunePlacementsToPopulation: %v", err)
	}
	if pruned != 2 {
		t.Errorf("pruned = %d, want 2 (bronze + cult)", pruned)
	}

	rows, err := pool.Query(ctx,
		`SELECT gubbe_ordinal, good_key FROM settlement_placement WHERE settlement_id = $1 ORDER BY gubbe_ordinal`,
		settlementID,
	)
	if err != nil {
		t.Fatalf("read survivors: %v", err)
	}
	defer rows.Close()
	var survivors []struct {
		ordinal int
		good    string
	}
	for rows.Next() {
		var s struct {
			ordinal int
			good    string
		}
		if err := rows.Scan(&s.ordinal, &s.good); err != nil {
			t.Fatalf("scan survivor: %v", err)
		}
		survivors = append(survivors, s)
	}
	if len(survivors) != 2 {
		t.Fatalf("survivors = %d, want 2", len(survivors))
	}
	for _, s := range survivors {
		if s.good != GoodGrain {
			t.Errorf("survivor ordinal %d has good_key %q, want %q — the fallback chain must never eat the last food",
				s.ordinal, s.good, GoodGrain)
		}
	}
}

// TestPrunePlacementsToPopulation_ConvergesInOnePass is AC3: an already
// badly-broken settlement (5 non-food placements against a cap of 2) is
// fixed in a SINGLE call, not one row per tick — the fix for the 6 live
// CT126 cities that had accumulated up to 300 phantom placements.
func TestPrunePlacementsToPopulation_ConvergesInOnePass(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	settlementID := seedFullRingFixture(t, 100 /*pop*/, 200, "plains") // floor(200/100) = 2
	hex := hexRing0(t)[0]
	for ordinal := 1; ordinal <= 5; ordinal++ {
		placeHexGubbe(t, pool, settlementID, ordinal, hex, GoodTimber)
	}

	pruned, err := PrunePlacementsToPopulation(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("PrunePlacementsToPopulation: %v", err)
	}
	if pruned != 3 {
		t.Errorf("pruned = %d, want 3 (5 placements down to cap 2, in one pass)", pruned)
	}

	var remaining int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM settlement_placement WHERE settlement_id = $1`, settlementID,
	).Scan(&remaining); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if remaining != 2 {
		t.Fatalf("remaining = %d, want 2 (cap for pop 200)", remaining)
	}

	var survivingOrdinals []int
	rows, err := pool.Query(ctx,
		`SELECT gubbe_ordinal FROM settlement_placement WHERE settlement_id = $1 ORDER BY gubbe_ordinal`,
		settlementID,
	)
	if err != nil {
		t.Fatalf("read surviving ordinals: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var o int
		if err := rows.Scan(&o); err != nil {
			t.Fatalf("scan ordinal: %v", err)
		}
		survivingOrdinals = append(survivingOrdinals, o)
	}
	if len(survivingOrdinals) != 2 || survivingOrdinals[0] != 1 || survivingOrdinals[1] != 2 {
		t.Errorf("surviving ordinals = %v, want [1 2] (lowest ordinal within a tier survives longest)", survivingOrdinals)
	}
}

// TestPrunePlacementsToPopulation_IdempotentWhenWithinCap is AC4: a
// settlement already within its cap is left completely untouched — this
// function runs every tick for every active settlement, so a healthy city
// must never see a row disappear.
func TestPrunePlacementsToPopulation_IdempotentWhenWithinCap(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	settlementID := seedFullRingFixture(t, 100 /*pop*/, 500, "plains") // floor(500/100) = 5
	hexes := hexRing0(t)
	placeHexGubbe(t, pool, settlementID, 1, hexes[0], GoodGrain)
	placeHexGubbe(t, pool, settlementID, 2, hexes[1], GoodGrain)
	placeHexGubbe(t, pool, settlementID, 3, hexes[2], GoodGrain)

	pruned, err := PrunePlacementsToPopulation(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("PrunePlacementsToPopulation: %v", err)
	}
	if pruned != 0 {
		t.Errorf("pruned = %d, want 0 (3 placements, cap 5 — already within bounds)", pruned)
	}

	var remaining int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM settlement_placement WHERE settlement_id = $1`, settlementID,
	).Scan(&remaining); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if remaining != 3 {
		t.Errorf("remaining = %d, want 3 (untouched)", remaining)
	}
}

// TestPlaceNextGubbeOnBestFoodHex_OrdinalAlreadyTakenFallsToPoolWithoutError
// covers §Kända fällor #1 (megaron_plan_placeringsbeskarning.md): growth's
// oldGubbar+1..newGubbar loop (kharis/tick.go, settlement_placement.go
// SlaughterLivestock) can hand PlaceNextGubbeOnBestFoodHex an ordinal a
// prune left occupied on a surviving row. That must degrade to "falls to
// the pool", the same as "every food slot full" — never a hard error that
// would abort the whole tick loop or 500 a player's slaughter request.
func TestPlaceNextGubbeOnBestFoodHex_OrdinalAlreadyTakenFallsToPoolWithoutError(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	settlementID := seedFullRingFixture(t, 100 /*pop*/, 500, "plains")
	placeHexGubbe(t, pool, settlementID, 7, hexRing0(t)[0], GoodGrain) // ordinal 7 already occupied

	placed, err := PlaceNextGubbeOnBestFoodHex(ctx, pool, settlementID, 7)
	if err != nil {
		t.Fatalf("PlaceNextGubbeOnBestFoodHex with a colliding ordinal must not error, got: %v", err)
	}
	if placed {
		t.Error("expected placed=false (ordinal already taken by a surviving row), got true")
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM settlement_placement WHERE settlement_id = $1 AND gubbe_ordinal = 7`, settlementID,
	).Scan(&count); err != nil {
		t.Fatalf("count ordinal 7: %v", err)
	}
	if count != 1 {
		t.Errorf("rows at ordinal 7 = %d, want 1 (unchanged, no duplicate/overwrite attempt)", count)
	}
}

// hexRing0 returns the standard catchment ring around (0,0) —
// seedFullRingFixture always plants its settlement at province (0,0) — for
// tests that just need real, distinct hex_q/hex_r values and don't care
// which ring position they land on.
func hexRing0(t *testing.T) []hexgrid.Coord {
	t.Helper()
	ring := hexgrid.Ring(hexgrid.Coord{Q: 0, R: 0}, hexgrid.CatchmentRadius)
	if len(ring) < 3 {
		t.Fatalf("catchment ring too small for fixture use: %d hexes", len(ring))
	}
	return ring
}
