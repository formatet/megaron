package economy

// Acceptance tests for megaron_plan_byggnad_pa_hex.md §A: a hex-bound
// production building's effect (cap AND rate) applies ONLY to its own hex.
// Uses foundingForecastFixture (founding_forecast_test.go) — a settlement
// whose full 18-hex catchment ring is seeded with a single terrain, so two
// ring hexes of the SAME terrain can be compared directly.
//
// Real Postgres, gated by DATABASE_URL (testPool, sitos_conservation_test.go).

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
)

// TestHexBuildingCap_OnlyAffectsItsOwnHex is acceptance criterion 1: a farm
// built on catchment hex X raises grain's cap on X and NOT on a neighbouring
// plains hex Y that has no farm of its own. Before this slice, hexGoodCaps
// read the building level from a SETTLEMENT-WIDE map, so a farm anywhere
// lifted the grain cap on EVERY plains hex in the catchment — this is the
// exact regression this test pins.
func TestHexBuildingCap_OnlyAffectsItsOwnHex(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	_, center, settlementID := foundingForecastFixture(t, 500, "plains")

	ring := hexgrid.Ring(center, hexgrid.CatchmentRadius)
	if len(ring) < 2 {
		t.Fatalf("fixture ring too small: %d hexes", len(ring))
	}
	hexX, hexY := ring[0], ring[1]

	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level, hex_q, hex_r) VALUES ($1, 'farm', 1, $2, $3)`,
		settlementID, hexX.Q, hexX.R,
	); err != nil {
		t.Fatalf("insert farm on hexX: %v", err)
	}

	opts, err := LoadHexProductionOptions(ctx, pool, settlementID, nil)
	if err != nil {
		t.Fatalf("LoadHexProductionOptions: %v", err)
	}
	var optX, optY *HexOption
	for i := range opts {
		switch opts[i].Coord {
		case hexX:
			optX = &opts[i]
		case hexY:
			optY = &opts[i]
		}
	}
	if optX == nil || optY == nil {
		t.Fatalf("expected both hexX=%v and hexY=%v in catchment options, got %d options", hexX, hexY, len(opts))
	}

	// plainsCapacityRules: grain capNoBuilding=4, capWithBuilding=6 +
	// WorkplaceSlots("farm",1)=2 → 8 with a farm actually standing here.
	if optX.CapPerGood[GoodGrain] != 8 {
		t.Errorf("hexX (has the farm): grain cap = %d, want 8 (6 capWithBuilding + 2 farm L1 slots)", optX.CapPerGood[GoodGrain])
	}
	if optY.CapPerGood[GoodGrain] != 4 {
		t.Errorf("hexY (no farm — plain neighbour): grain cap = %d, want 4 (capNoBuilding) — "+
			"a farm on hexX must NOT raise hexY's cap", optY.CapPerGood[GoodGrain])
	}
}

// TestHexBuildingCap_TwoFarmsSameCityDifferentHexesLevelIndependently is
// acceptance criterion 2: a settlement with two farms on two different
// hexes, at DIFFERENT levels, must read each farm's OWN level at each hex —
// never a settlement-wide MAX or a blended value.
func TestHexBuildingCap_TwoFarmsSameCityDifferentHexesLevelIndependently(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	_, center, settlementID := foundingForecastFixture(t, 500, "plains")

	ring := hexgrid.Ring(center, hexgrid.CatchmentRadius)
	if len(ring) < 2 {
		t.Fatalf("fixture ring too small: %d hexes", len(ring))
	}
	hexA, hexB := ring[0], ring[1]

	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level, hex_q, hex_r) VALUES ($1, 'farm', 1, $2, $3)`,
		settlementID, hexA.Q, hexA.R,
	); err != nil {
		t.Fatalf("insert farm L1 on hexA: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level, hex_q, hex_r) VALUES ($1, 'farm', 3, $2, $3)`,
		settlementID, hexB.Q, hexB.R,
	); err != nil {
		t.Fatalf("insert farm L3 on hexB: %v", err)
	}

	// Regression guard for the OLD by-(settlement_id,building_type) bug: MAX(level)
	// across both rows would read 3 for EVERY hex, including hexA's real level-1 farm.
	var maxLevel int
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(level), 0) FROM buildings WHERE settlement_id = $1 AND building_type = 'farm'`,
		settlementID,
	).Scan(&maxLevel); err != nil {
		t.Fatalf("read max farm level: %v", err)
	}
	if maxLevel != 3 {
		t.Fatalf("fixture invariant broken: expected MAX(level)=3 across both farms, got %d", maxLevel)
	}

	opts, err := LoadHexProductionOptions(ctx, pool, settlementID, nil)
	if err != nil {
		t.Fatalf("LoadHexProductionOptions: %v", err)
	}
	var optA, optB *HexOption
	for i := range opts {
		switch opts[i].Coord {
		case hexA:
			optA = &opts[i]
		case hexB:
			optB = &opts[i]
		}
	}
	if optA == nil || optB == nil {
		t.Fatalf("expected both hexA=%v and hexB=%v in catchment options", hexA, hexB)
	}

	// Grain is Form B's pinned exception (hexGoodCaps forces mult=1 for
	// grain unconditionally) — its level effect shows up in the CAP instead,
	// via WorkplaceSlots(level) (megaron_plan_grain_cap.md). hexA: farm L1 →
	// 6 capWithBuilding + WorkplaceSlots("farm",1)=2 = 8. hexB: farm L3 →
	// 6 + WorkplaceSlots("farm",3)=6 = 12.
	if optA.CapPerGood[GoodGrain] != 8 {
		t.Errorf("hexA (farm L1): grain cap = %d, want 8", optA.CapPerGood[GoodGrain])
	}
	if optB.CapPerGood[GoodGrain] != 12 {
		t.Errorf("hexB (farm L3): grain cap = %d, want 12 — the L3 farm's own level must show up on ITS hex", optB.CapPerGood[GoodGrain])
	}
	if optA.CapPerGood[GoodGrain] == optB.CapPerGood[GoodGrain] {
		t.Errorf("hexA and hexB have DIFFERENT farm levels (1 vs 3) but identical grain cap %d — "+
			"each hex must read its OWN farm's level", optA.CapPerGood[GoodGrain])
	}

	// LoadWorkplaceSlots must sum BOTH farms' own slots, not collapse them —
	// the exact regression a DISTINCT-without-building-id would reintroduce.
	slots, err := LoadWorkplaceSlots(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("LoadWorkplaceSlots: %v", err)
	}
	wantSlots := WorkplaceSlots("farm", 1) + WorkplaceSlots("farm", 3)
	if slots[GoodGrain] != wantSlots {
		t.Errorf("LoadWorkplaceSlots[grain] = %d, want %d (farm L1 slots + farm L3 slots, summed separately per hex)",
			slots[GoodGrain], wantSlots)
	}
}

// TestHexBuildingCap_MineOnCopperHexEnablesCopperNotSilver is part of
// acceptance criterion 3: a mine built on a copper-deposit hex enables
// copper there — a silver-only neighbour hex with NO mine of its own must
// stay ungated (silver's build-relevant-building lookup is now "mine",
// scoped per hex the same way as farm/grain above).
func TestHexBuildingCap_MineOnCopperHexEnablesCopperNotSilver(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	_, center, settlementID := foundingForecastFixture(t, 500, "hills")

	ring := hexgrid.Ring(center, hexgrid.CatchmentRadius)
	copperHex := ring[0]
	if _, err := pool.Exec(ctx,
		`UPDATE map_tiles SET copper_deposit = true WHERE world_id = (SELECT world_id FROM settlements WHERE id = $1) AND q = $2 AND r = $3`,
		settlementID, copperHex.Q, copperHex.R,
	); err != nil {
		t.Fatalf("seed copper deposit: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level, hex_q, hex_r) VALUES ($1, 'mine', 1, $2, $3)`,
		settlementID, copperHex.Q, copperHex.R,
	); err != nil {
		t.Fatalf("insert mine on copper hex: %v", err)
	}

	opts, err := LoadHexProductionOptions(ctx, pool, settlementID, nil)
	if err != nil {
		t.Fatalf("LoadHexProductionOptions: %v", err)
	}
	var optCopper *HexOption
	for i := range opts {
		if opts[i].Coord == copperHex {
			optCopper = &opts[i]
		}
	}
	if optCopper == nil {
		t.Fatalf("expected copper hex %v in catchment options", copperHex)
	}
	// depositCapacityTable["copper"]: capNoBuilding=1, capWithBuilding=3 +
	// WorkplaceSlots("mine",1)=2 → 5 with the mine actually standing here.
	if optCopper.CapPerGood["copper"] != 5 {
		t.Errorf("copper hex with its own mine: copper cap = %d, want 5 (3 capWithBuilding + 2 mine L1 slots)", optCopper.CapPerGood["copper"])
	}
}
