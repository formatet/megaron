package economy

// Acceptance tests for megaron_plan_byggnad_pa_hex.md §B (B1, server side).
// Real Postgres, gated by DATABASE_URL (testPool, sitos_conservation_test.go).

import (
	"context"
	"strings"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
)

// findEffect returns the single row matching (good, kind) from a row set,
// failing the test if it's missing or duplicated — every assertion below
// wants exactly one such row.
func findEffect(t *testing.T, rows []EffectRow, good, kind string) EffectRow {
	t.Helper()
	var found []EffectRow
	for _, r := range rows {
		if r.Good == good && r.Kind == kind {
			found = append(found, r)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one %s/%s row, got %d (rows=%+v)", good, kind, len(found), rows)
	}
	return found[0]
}

// TestBuildingEffect_FarmOnPlains_HardcodedNumbers is acceptance criterion 1:
// farm on plains, grain without 4×1.00, levels 1/2/3 = 8/10/12 × 2.70 — no
// level ever changes grain's per-gubbe rate (the grain-cap exception,
// megaron_plan_grain_cap.md, pinned in hexGoodCaps).
func TestBuildingEffect_FarmOnPlains_HardcodedNumbers(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	rows, err := BuildingEffectsForHex(ctx, pool, "farm", "plains", false, false, false, false)
	if err != nil {
		t.Fatalf("BuildingEffectsForHex: %v", err)
	}
	grain := findEffect(t, rows, GoodGrain, "hex")

	if grain.Without.PerGubbe != 1.00 || grain.Without.Gubbar != 4 {
		t.Fatalf("grain without farm = %d × %.2f, want 4 × 1.00", grain.Without.Gubbar, grain.Without.PerGubbe)
	}
	wantGubbar := []int{8, 10, 12}
	for i, lvl := range grain.Levels {
		if lvl.PerGubbe != 2.70 {
			t.Errorf("grain L%d per_gubbe = %.2f, want 2.70 (a level must never move grain's rate)", lvl.Level, lvl.PerGubbe)
		}
		if lvl.Gubbar != wantGubbar[i] {
			t.Errorf("grain L%d gubbar = %d, want %d", lvl.Level, lvl.Gubbar, wantGubbar[i])
		}
	}

	// oil is a real side effect of farm on plains (mig146 finding: farm also
	// bumps oil here) but must never show up misattributed as grain's cap/rate.
	oil := findEffect(t, rows, GoodOil, "hex")
	if oil.Terrain != "plains" {
		t.Fatalf("oil effect terrain = %q, want plains", oil.Terrain)
	}
}

// TestBuildingEffect_LumbermillOnForestOliveGrove is acceptance criterion 3:
// lumbermill on forest_olive_grove shows timber's per-gubbe rate SHRINK
// (0.25 at level 1) because lumbermill raises the HEX'S CAPACITY here
// (terrainCapacityTable's relevantBuilding) without any matching
// production_rules row raising the RATE — the flagship "bad number, shown
// as it is" case the plan names.
func TestBuildingEffect_LumbermillOnForestOliveGrove(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	rows, err := BuildingEffectsForHex(ctx, pool, "lumbermill", "forest_olive_grove", false, false, false, false)
	if err != nil {
		t.Fatalf("BuildingEffectsForHex: %v", err)
	}
	timber := findEffect(t, rows, GoodTimber, "hex")

	if timber.Without.PerGubbe != 1.00 || timber.Without.Gubbar != 1 {
		t.Fatalf("timber without lumbermill = %d × %.2f, want 1 × 1.00", timber.Without.Gubbar, timber.Without.PerGubbe)
	}
	if len(timber.Levels) != 3 {
		t.Fatalf("want 3 levels, got %d", len(timber.Levels))
	}
	if timber.Levels[0].PerGubbe != 0.25 || timber.Levels[0].Gubbar != 4 {
		t.Fatalf("timber L1 = %d × %.2f, want 4 × 0.25", timber.Levels[0].Gubbar, timber.Levels[0].PerGubbe)
	}
	if timber.Levels[0].PerGubbe >= timber.Without.PerGubbe {
		t.Fatalf("timber's per-gubbe rate must SHRINK once a lumbermill stands here (got L1=%.2f vs without=%.2f)",
			timber.Levels[0].PerGubbe, timber.Without.PerGubbe)
	}
}

// TestBuildingEffect_MineOnCopperHex is acceptance criterion 2: mine on a
// copper hex — 1 gubbe without a mine, 5 gubbar at every level once built,
// per-gubbe rate rising with level (Form B: headcount frozen at capL1, the
// RATE carries the level's effect).
func TestBuildingEffect_MineOnCopperHex(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	rows, err := BuildingEffectsForHex(ctx, pool, "mine", "hills", true, false, false, false)
	if err != nil {
		t.Fatalf("BuildingEffectsForHex: %v", err)
	}
	copper := findEffect(t, rows, GoodCopper, "hex")

	if copper.Without.Gubbar != 1 {
		t.Fatalf("copper without mine = %d gubbar, want 1", copper.Without.Gubbar)
	}
	for _, lvl := range copper.Levels {
		if lvl.Gubbar != 5 {
			t.Errorf("copper L%d gubbar = %d, want 5 (Form B freezes headcount at capL1)", lvl.Level, lvl.Gubbar)
		}
	}
	if !(copper.Levels[0].PerGubbe < copper.Levels[1].PerGubbe && copper.Levels[1].PerGubbe < copper.Levels[2].PerGubbe) {
		t.Fatalf("copper per-gubbe rate must strictly rise with level, got L1=%.2f L2=%.2f L3=%.2f",
			copper.Levels[0].PerGubbe, copper.Levels[1].PerGubbe, copper.Levels[2].PerGubbe)
	}
}

// TestBuildingEffect_MarketAndStableProduceNothingActive is a guardrail for
// the "check before writing" note: market's only production_rules row
// (pottery) and stable's (horses) both name a PARKED good, so neither
// building may claim any effect today — the catalogue's purpose line for
// each must not claim a good the effects list never shows.
func TestBuildingEffect_MarketAndStableProduceNothingActive(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	for _, bt := range []string{"market", "stable"} {
		rows, err := BuildingEffectsForCatalogue(ctx, pool, bt)
		if err != nil {
			t.Fatalf("BuildingEffectsForCatalogue(%s): %v", bt, err)
		}
		if len(rows) != 0 {
			t.Fatalf("%s: want zero effect rows (its only production_rules good is parked), got %+v", bt, rows)
		}
	}
}

// TestBuildingEffect_WineryRoutesToRefiningCeiling is the weakest-link case:
// a winery's terrain+building row (e.g. hills) must show up as
// "refining_ceiling" (a ceiling only realized alongside a refining gubbe),
// never as a plain "hex" row claiming the rate outright.
func TestBuildingEffect_WineryRoutesToRefiningCeiling(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	rows, err := BuildingEffectsForHex(ctx, pool, "winery", "hills", false, false, false, false)
	if err != nil {
		t.Fatalf("BuildingEffectsForHex: %v", err)
	}
	for _, r := range rows {
		if r.Good == GoodWine && r.Kind == "hex" {
			t.Fatalf("wine must never appear as a plain hex row for winery, got %+v", r)
		}
	}
	ceiling := findEffect(t, rows, GoodWine, "refining_ceiling")
	if ceiling.Without.PerGubbe != 0 || ceiling.Without.Gubbar != 0 {
		t.Fatalf("wine ceiling without winery = %d × %.2f, want 0 × 0.00", ceiling.Without.Gubbar, ceiling.Without.PerGubbe)
	}
	refining := findEffect(t, rows, GoodWine, "refining")
	if refining.Levels[0].PerGubbe <= 0 {
		t.Fatalf("winery's own refining row must show a positive per-gubbe rate at L1, got %+v", refining)
	}
}

// TestBuildingEffect_ParityWithProductionPath is acceptance criterion 7: a
// real settlement with a real farm on a real hex — the effect row's numbers
// at that hex must equal what LoadHexProductionOptions/placementYield
// itself produces for one placed gubbe, and LoadBuildingProductionOptions
// for a workplace (checked via foundry, whose only row is a pure workplace/
// refining row).
func TestBuildingEffect_ParityWithProductionPath(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	_, center, settlementID := foundingForecastFixture(t, 500, "plains")

	ring := hexgrid.Ring(center, hexgrid.CatchmentRadius)
	hexX := ring[0]
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level, hex_q, hex_r) VALUES ($1, 'farm', 2, $2, $3)`,
		settlementID, hexX.Q, hexX.R,
	); err != nil {
		t.Fatalf("insert farm L2 on hexX: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'foundry', 2)`,
		settlementID,
	); err != nil {
		t.Fatalf("insert foundry L2: %v", err)
	}

	opts, err := LoadHexProductionOptions(ctx, pool, settlementID, nil)
	if err != nil {
		t.Fatalf("LoadHexProductionOptions: %v", err)
	}
	var optX *HexOption
	for i := range opts {
		if opts[i].Coord == hexX {
			optX = &opts[i]
		}
	}
	if optX == nil {
		t.Fatalf("hexX not found in catchment options")
	}
	wantGrainPerGubbe := placementYield(GoodGrain, optX.RatePerGood[GoodGrain], optX.CapL1PerGood[GoodGrain], optX.PlaceCapPerGood[GoodGrain], optX.MultPerGood[GoodGrain], 1)
	wantGrainGubbar := optX.PlaceCapPerGood[GoodGrain]

	effectRows, err := BuildingEffectsForHex(ctx, pool, "farm", "plains", false, false, false, false)
	if err != nil {
		t.Fatalf("BuildingEffectsForHex: %v", err)
	}
	grain := findEffect(t, effectRows, GoodGrain, "hex")
	gotLevel2 := grain.Levels[1] // level index 1 = L2, matches the built farm's level
	if round2(wantGrainPerGubbe) != gotLevel2.PerGubbe || wantGrainGubbar != gotLevel2.Gubbar {
		t.Fatalf("parity mismatch: production path = %d × %.2f, effect row L2 = %d × %.2f",
			wantGrainGubbar, round2(wantGrainPerGubbe), gotLevel2.Gubbar, gotLevel2.PerGubbe)
	}

	buildingOpts, err := LoadBuildingProductionOptions(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("LoadBuildingProductionOptions: %v", err)
	}
	var foundry *BuildingOption
	for i := range buildingOpts {
		if buildingOpts[i].BuildingType == "foundry" {
			foundry = &buildingOpts[i]
		}
	}
	if foundry == nil {
		t.Fatalf("foundry not found in building options")
	}
	wantBronzePerGubbe := placementYield(GoodBronze, foundry.RatePerGood[GoodBronze], foundry.CapL1PerGood[GoodBronze], foundry.PlaceCapPerGood[GoodBronze], foundry.MultPerGood[GoodBronze], 1)
	wantBronzeGubbar := foundry.PlaceCapPerGood[GoodBronze]

	foundryRows, err := BuildingEffectsForHex(ctx, pool, "foundry", "plains", false, false, false, false)
	if err != nil {
		t.Fatalf("BuildingEffectsForHex(foundry): %v", err)
	}
	bronze := findEffect(t, foundryRows, GoodBronze, "refining")
	gotBronzeLevel2 := bronze.Levels[1]
	if round2(wantBronzePerGubbe) != gotBronzeLevel2.PerGubbe || wantBronzeGubbar != gotBronzeLevel2.Gubbar {
		t.Fatalf("foundry parity mismatch: production path = %d × %.2f, effect row L2 = %d × %.2f",
			wantBronzeGubbar, round2(wantBronzePerGubbe), gotBronzeLevel2.Gubbar, gotBronzeLevel2.PerGubbe)
	}
}

// A silver deposit has a capacity rule but no production_rules row without a
// mine — nothing is placeable there, so "without" must read none, not 1 × 0.00.
// Harbour's row is catchment-wide and must say so in its text.
func TestBuildingEffect_SilverWithoutMineIsNone_HarbourSaysEveryHex(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	rows, err := BuildingEffectsForHex(ctx, pool, "mine", "hills", false, false, true, false)
	if err != nil {
		t.Fatalf("BuildingEffectsForHex: %v", err)
	}
	silver := findEffect(t, rows, GoodSilver, "hex")
	if silver.Without.Gubbar != 0 || silver.Without.PerGubbe != 0 {
		t.Fatalf("silver without mine = %d × %.2f, want none", silver.Without.Gubbar, silver.Without.PerGubbe)
	}
	if !strings.Contains(silver.Text, ": none →") {
		t.Fatalf("silver text %q, want it to read none before the arrow", silver.Text)
	}

	harbour, err := BuildingEffectsForHex(ctx, pool, "harbour", "coastal_sea", false, false, false, true)
	if err != nil {
		t.Fatalf("BuildingEffectsForHex(harbour): %v", err)
	}
	fish := findEffect(t, harbour, GoodFish, "hex")
	if !strings.Contains(fish.Text, "on every coastal sea hex") {
		t.Fatalf("harbour fish text %q, want it to say every coastal sea hex", fish.Text)
	}
}
