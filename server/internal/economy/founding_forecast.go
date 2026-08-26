package economy

import (
	"context"
	"fmt"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// FoundingGrainNetPerTick forecasts the grain balance a founding on
// (worldID, center) would get, by running the EXACT SAME catchment math a
// real founding uses — rankedFoodSlotsAt + placeGreedyOnFoodSlots, the same
// two calls PlaceStartingWorkforce makes — against a hypothetical catchment
// (no settlement row exists yet at forecast time). This replaces the old
// pre-P4 linear estimate (0,85×pop / REF_LABOR, uncapped, no per-hex tak) —
// a second formula that answered a different question than "what will this
// city actually produce" and drifted from the real outcome by 34× at 4 000
// invånare (megaron_plan_grundningsprognosen.md §1, kodläst 2026-08-25).
// "En formel, två anrop" (§3): PlaceStartingWorkforce INSERTs the placement
// list this function only sums.
//
// ⭐ NETTO, and deliberately so after Utfodringsordningen D1
// (megaron_plan_utfodringsordningen.md, 2026-08-26). The engine no longer
// folds the population's meal into grain's stored rate — FoodTick debits it
// once a day from STOCK, so settlement_goods.rate is now RAW production.
// This forecast keeps netting anyway, because the question a founder asks is
// "will this site feed my people?", not "how much grain grows here". It is
// the one deliberate FoodConsumptionSplit caller left outside the engine:
// there is no settlement, and therefore no stock, to draw down yet.
// ⚠️ Consequence for anyone comparing the two: forecast (net) and
// settlement_goods.rate (gross) are no longer the same quantity — net them
// through economy.GrainBalance before asserting parity. The acceptance test
// in api/handlers/founding_forecast_parity_test.go does exactly that.
//
// buildingLevels is the hypothetical building set the founding would seed —
// {"farm": 1} for a metropolis (createMetropolis's starter farm), empty for
// a colony (builds its own farm later, unit_arrival.go foundColony). reachable
// is LoadHexProductionOptionsAt's FOW gate: pass the set of hexes the
// requesting Wanax actually knows so an unseen hex never leaks into the
// forecast (api/handlers/world.go ColonizePreview is FOW-critical); nil runs
// the full, unfiltered catchment for trusted internal callers.
//
// Mirrors RecomputeProduction's three grain terms exactly (recompute.go
// steps 3/4b/5): the placement sum, the population remainder's old aggregate
// term (grainBasePotential/REF_LABOR × pop%100 — this is the one place
// REF_LABOR still governs output after P4, and it is the SAME expression as
// recompute.go's step 4b, not a second one), and NearjordGrainPerTick.
// FoodConsumptionSplit and the fish math are unchanged from before this
// slice. livestockStock is passed as 0: a real founding's herd
// (FoundingHerdLivestock) is seeded in the SAME transaction as the founding,
// so its calc_tick equals current_world_tick() and RecomputeProduction's own
// livestockSettledThisTick guard forces livestockStock=0 for that very FIRST
// recompute too (recompute.go) — verified against the code 2026-08-26, this
// is not a stale simplification, it matches the real first-tick behaviour.
func FoundingGrainNetPerTick(ctx context.Context, tx Tx, worldID uuid.UUID, center hexgrid.Coord, buildingLevels map[string]int, reachable map[hexgrid.Coord]bool, pop int) (prodPerTick, netPerTick float64, err error) {
	hexOptions, err := LoadHexProductionOptionsAt(ctx, tx, worldID, center, buildingLevels, reachable)
	if err != nil {
		return 0, 0, fmt.Errorf("founding grain net per tick: %w", err)
	}
	slots := rankSlotsFromOptions(hexOptions, center)

	grainBasePotential := 0.0
	for _, opt := range hexOptions {
		grainBasePotential += opt.RatePerGood[GoodGrain]
	}
	remainderCitizens := pop % 100
	totalGubbar := pop / 100
	remainderTerm := (grainBasePotential / REF_LABOR) * float64(remainderCitizens)
	guaranteedFood := NearjordGrainPerTick + remainderTerm
	demand := GrainConsumptionPerTick(pop)

	placements, _ := placeGreedyOnFoodSlots(slots, guaranteedFood, demand, totalGubbar)

	grainProd := NearjordGrainPerTick + remainderTerm
	fishProd := 0.0
	for _, p := range placements {
		if p.good == GoodGrain {
			grainProd += p.yield
		} else {
			fishProd += p.yield
		}
	}

	grainNet, _, _ := FoodConsumptionSplit(demand, grainProd, fishProd, 0)
	return grainProd, grainNet, nil
}
