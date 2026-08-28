package economy

// Tests for FoodGubbarRequired (megaron_plan_p4_arvet_i_province.md), P4's
// replacement for province.go's two pre-P4 weight/aggregate-based figures.
// DB integration tests (real Postgres, DATABASE_URL-gated, same pattern as
// the rest of this package).

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// TestFoodGubbarRequired_ParityWithEngine is the plan's §4.3 non-circular
// proof: FoodGubbarRequired's number is checked against RecomputeProduction
// — the REAL production_rules → settlement_placement → settlement_goods
// pipeline — never against the same expression that produced it (the lesson
// from 2026-08-24: "ett test som mäter en konstant mot sig själv bevisar
// intet"). Placing exactly `required` gubbar (via PlaceStartingWorkforce,
// the SAME greedy caller founding uses, run independently of
// FoodGubbarRequired's own internal call) must leave the city fed
// (grainNet>=0 per FoodConsumptionSplit); removing the one gubbe that tipped
// the greedy loop over demand must leave it short.
func TestFoodGubbarRequired_ParityWithEngine(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	const pop = 1000 // totalGubbar = 10, remainderCitizens = 0 (no free lunch)
	settlementID := seedFullRingFixture(t, 100, pop, "plains")

	required, achievable, err := FoodGubbarRequired(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("FoodGubbarRequired: %v", err)
	}
	if !achievable {
		t.Fatalf("expected an 18-plains-hex catchment to feed pop=%d, got achievable=false (required=%d)", pop, required)
	}
	if required < 1 {
		t.Fatalf("required=%d, want at least 1 gubbe (a free lunch makes this test trivial)", required)
	}

	placed, sufficient, err := PlaceStartingWorkforce(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("PlaceStartingWorkforce: %v", err)
	}
	if placed != required {
		t.Fatalf("PlaceStartingWorkforce placed %d gubbar, FoodGubbarRequired says %d required — "+
			"the two callers of placeGreedyOnFoodSlots have drifted apart", placed, required)
	}
	if !sufficient {
		t.Fatalf("PlaceStartingWorkforce reports insufficient, but FoodGubbarRequired says achievable=true")
	}

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}
	if net := readGrainNet(t, ctx, pool, settlementID, pop); net < 0 {
		t.Errorf("with all %d required gubbar placed, the REAL engine still nets %.4f grain/tick short — "+
			"FoodGubbarRequired's estimate does not match RecomputeProduction's real output", required, net)
	}

	// required-1: remove the LAST placed gubbe — the one whose yield tipped
	// the greedy loop's running total over demand — and let the real engine
	// judge again. It must now show a deficit.
	var lastOrdinal int
	if err := pool.QueryRow(ctx,
		`SELECT max(gubbe_ordinal) FROM settlement_placement WHERE settlement_id = $1`, settlementID,
	).Scan(&lastOrdinal); err != nil {
		t.Fatalf("find last placed gubbe: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`DELETE FROM settlement_placement WHERE settlement_id = $1 AND gubbe_ordinal = $2`,
		settlementID, lastOrdinal,
	); err != nil {
		t.Fatalf("remove last gubbe: %v", err)
	}
	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction (required-1): %v", err)
	}
	if net := readGrainNet(t, ctx, pool, settlementID, pop); net >= 0 {
		t.Errorf("with required-1 (%d) gubbar placed, the REAL engine still nets %.4f grain/tick — expected a deficit", required-1, net)
	}
}

// TestFoodGubbarRequired_MatdodStad is the plan's §4.4 acceptance test: a
// catchment with only olive-grove/limestone-class terrain (no grain, no
// fish — Gournia/Zakros in drift) can never feed its population no matter
// how many gubbar stand on it. FoodGubbarRequired must SAY so
// (achievable=false, required=pop/100 — the whole workforce), never fall
// silent the way the old pre-P4 weight figure did (nil when the catchment's
// base grain potential was 0, and the JSON key simply vanished from the response).
func TestFoodGubbarRequired_MatdodStad(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	const pop = 500 // totalGubbar = 5
	settlementID := seedFullRingFixture(t, 100, pop, "mountain_limestone")

	required, achievable, err := FoodGubbarRequired(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("FoodGubbarRequired: %v", err)
	}
	if achievable {
		t.Fatal("expected a mountain_limestone-only catchment (no grain/fish terrain) to be unachievable")
	}
	if wantRequired := pop / 100; required != wantRequired {
		t.Errorf("required = %d, want pop/100 = %d (the whole workforce, not a silent 0)", required, wantRequired)
	}
}

// readGrainNet reads grain+fish rates after RecomputeProduction and returns
// the grain-net figure FoodConsumptionSplit assigns — "any shortfall still
// standing stays on grain" (FoodConsumptionSplit's own doc comment), so a
// negative number here is the unambiguous "not fed" signal.
func readGrainNet(t *testing.T, ctx context.Context, pool Tx, settlementID uuid.UUID, pop int) float64 {
	t.Helper()
	var grainRate float64
	if err := pool.QueryRow(ctx,
		`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'`, settlementID,
	).Scan(&grainRate); err != nil {
		t.Fatalf("read grain rate: %v", err)
	}
	var fishRate float64
	_ = pool.QueryRow(ctx, // fish row may legitimately not exist (inland catchment) — 0 is correct then
		`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'fish'`, settlementID,
	).Scan(&fishRate)
	demand := GrainConsumptionPerTick(pop)
	grainNet, _, _ := FoodConsumptionSplit(demand, grainRate, fishRate, 0)
	return grainNet
}
