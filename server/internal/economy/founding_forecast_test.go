package economy

// FoundingGrainNetPerTick used to run its own pre-P4 linear estimate
// (0,85×pop / REF_LABOR, uncapped, no per-hex tak) instead of the SAME
// placement formula a real founding uses — megaron_plan_grundningsprognosen.md
// §3 ("en formel, två anrop"). These tests cover the two layers that changed:
// placeGreedyOnFoodSlots (the pure placement decision, extracted out of
// PlaceStartingWorkforce so the forecast can run it too) and
// FoundingGrainNetPerTick itself against a real catchment. The end-to-end
// "prognos = utfall" acceptance test (three catchments, a real founding, ≤1%)
// lives at api/handlers/founding_forecast_parity_test.go — it needs the full
// createMetropolis path (Demeter's farm gift) that this package cannot reach.

import (
	"context"
	"math"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// TestPlaceGreedyOnFoodSlots_StopsAtSelfSufficiency: the loop must stop the
// MOMENT cumulative reaches demand, leaving later (lower-yield) slots and
// remaining gubbar untouched — Timothy 2026-08-08's "om det kräver 5/10
// placeras de" rule (PlaceStartingWorkforce's doc comment).
func TestPlaceGreedyOnFoodSlots_StopsAtSelfSufficiency(t *testing.T) {
	slots := []foodSlot{
		{hex: hexgrid.Coord{Q: 0, R: 0}, good: GoodGrain, yield: 10, cap: 4, ordinal: 1},
		{hex: hexgrid.Coord{Q: 1, R: 0}, good: GoodGrain, yield: 5, cap: 4, ordinal: 2},
	}
	placements, sufficient := placeGreedyOnFoodSlots(slots, 0, 25, 10)
	if !sufficient {
		t.Fatalf("expected self-sufficient, got sufficient=false with placements=%v", placements)
	}
	// 0 -> +10 -> +20 -> +30 (>=25): three gubbar on the first (higher-yield)
	// slot, none on the second.
	if len(placements) != 3 {
		t.Fatalf("expected 3 placements (stops once cumulative>=25), got %d: %v", len(placements), placements)
	}
	for _, p := range placements {
		if p.hex != slots[0].hex {
			t.Errorf("expected every placement on the higher-yield slot %v, got one on %v", slots[0].hex, p.hex)
		}
	}
}

// TestPlaceGreedyOnFoodSlots_ExhaustsWorkforceWithoutSufficiency: when even
// every gubbe on food isn't enough, ALL totalGubbar are placed anyway
// (best effort) and sufficient=false tells the caller to warn — the OTHER
// half of Timothy's rule ("om staden behöver 10/10 och ändå inte försörjer
// sig ska de placeras men spelare måste varnas").
func TestPlaceGreedyOnFoodSlots_ExhaustsWorkforceWithoutSufficiency(t *testing.T) {
	slots := []foodSlot{
		{hex: hexgrid.Coord{Q: 0, R: 0}, good: GoodGrain, yield: 1, cap: 10, ordinal: 1},
	}
	placements, sufficient := placeGreedyOnFoodSlots(slots, 0, 1000, 5)
	if sufficient {
		t.Fatalf("expected NOT self-sufficient (demand 1000 way above 5×1), got sufficient=true")
	}
	if len(placements) != 5 {
		t.Fatalf("expected all 5 gubbar placed (best effort), got %d", len(placements))
	}
}

// TestPlaceGreedyOnFoodSlots_GuaranteedFoodAloneCanAlreadySuffice: a city
// whose nearjord+remainder term already covers demand places ZERO gubbar on
// food and reports sufficient — the P0-UI answer that makes most founding
// sites read "netto ≈ 0" without any placement at all.
func TestPlaceGreedyOnFoodSlots_GuaranteedFoodAloneCanAlreadySuffice(t *testing.T) {
	slots := []foodSlot{
		{hex: hexgrid.Coord{Q: 0, R: 0}, good: GoodGrain, yield: 10, cap: 4, ordinal: 1},
	}
	placements, sufficient := placeGreedyOnFoodSlots(slots, 100, 50, 10)
	if !sufficient {
		t.Fatalf("expected sufficient (guaranteedFood 100 already exceeds demand 50)")
	}
	if len(placements) != 0 {
		t.Fatalf("expected zero placements, got %d", len(placements))
	}
}

// foundingForecastFixture seeds a world + settlement whose FULL 18-hex ring
// carries `terrain` — settlementID exists (rankedFoodSlotsAt/
// LoadHexProductionOptionsAt need worldID+center, not a settlement, but
// PlaceStartingWorkforce and RecomputeProduction — the "real founding" side
// of the comparison — need one). No settlement_placement rows yet: the
// caller places starting workforce itself, AFTER calling the forecast, to
// prove the forecast needs no placement to already exist.
func foundingForecastFixture(t *testing.T, pop int, terrain string) (worldID uuid.UUID, center hexgrid.Coord, settlementID uuid.UUID) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover test worlds: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 0) RETURNING id`,
		"test-founding-forecast-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"founding-forecast-"+uuid.New().String(), "founding-forecast-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	center = hexgrid.Coord{Q: 0, R: 0}
	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, $4) RETURNING id`,
		worldID, center.Q, center.R, terrain,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	for _, hex := range hexgrid.Ring(center, hexgrid.CatchmentRadius) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, hex.Q, hex.R, terrain,
		); err != nil {
			t.Fatalf("seed ring tile (%d,%d): %v", hex.Q, hex.R, err)
		}
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Forecastville', 'achaean', $3, 'capital', true, $4) RETURNING id`,
		worldID, provinceID, ownerID, pop,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'grain', 0, 0, 1000000, 0)`,
		settlementID,
	); err != nil {
		t.Fatalf("seed grain row: %v", err)
	}

	return worldID, center, settlementID
}

// TestFoundingGrainNetPerTick_MatchesPlaceStartingWorkforce is the "en
// formel" half of the acceptance criterion, at the package level where both
// sides of the comparison are directly reachable: forecast a plains
// catchment BEFORE any settlement_placement row exists, then run the SAME
// catchment through PlaceStartingWorkforce + RecomputeProduction and read
// settlement_goods.rate. No Demeter farm-gift logic in the way (that lives in
// api/handlers/create_metropolis.go and is covered by
// TestFoundingGrainForecast_MatchesRealFounding instead) — buildingLevels is
// nil on both sides here, by construction.
func TestFoundingGrainNetPerTick_MatchesPlaceStartingWorkforce(t *testing.T) {
	const pop = 4000
	worldID, center, settlementID := foundingForecastFixture(t, pop, "plains")
	pool := testPool(t)
	ctx := context.Background()

	forecastProd, forecastNet, err := FoundingGrainNetPerTick(ctx, pool, worldID, center, nil, nil, pop)
	if err != nil {
		t.Fatalf("FoundingGrainNetPerTick: %v", err)
	}

	if _, _, err := PlaceStartingWorkforce(ctx, pool, settlementID); err != nil {
		t.Fatalf("PlaceStartingWorkforce: %v", err)
	}
	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	var actualRate float64
	if err := pool.QueryRow(ctx,
		`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'`, settlementID,
	).Scan(&actualRate); err != nil {
		t.Fatalf("load actual grain rate: %v", err)
	}

	// Compare the forecast's PRODUCTION term, not its net: since
	// Utfodringsordningen D1 (megaron_plan_utfodringsordningen.md, 2026-08-26)
	// settlement_goods.rate is RAW production — the population's meal is
	// debited from stock by FoodTick, never folded into the rate — while the
	// forecast still nets, because a founder asks "will this site feed my
	// people?". Comparing net against the stored rate passed only while the
	// rate was itself net; it now differs by the population's whole daily
	// need (2 000/tick at 4 000 invånare). Production is the quantity the two
	// sides genuinely share, and it is the placement math this slice replaced.
	//
	// Tiny epsilon, not exact equality: the forecast and RecomputeProduction
	// sum the same placement yields in different orders (Go slice vs SQL
	// aggregation), so floating-point rounding can differ in the last bit.
	if diff := math.Abs(forecastProd - actualRate); diff > 1e-6 {
		t.Errorf("forecast production %.9f != actual rate %.9f (diff %.2e, net=%.6f) — same formula must give the same number",
			forecastProd, actualRate, diff, forecastNet)
	}
}
