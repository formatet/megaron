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
// hasFarm mirrors whether the founding would seed a starter farm —
// true for an INLAND metropolis (createMetropolis's Demeter's-gift farm; a coastal one gets a galley instead), false for
// a colony (builds its own farm later, unit_arrival.go foundColony). When
// true, ChooseFarmHex picks the SAME hex a real founding would (§A of the
// plan: "prognosen och verkligt grundande måste välja samma hex"). reachable
// is LoadHexProductionOptionsAt's FOW gate: pass the set of hexes the
// requesting Wanax actually knows so an unseen hex never leaks into the
// forecast (api/handlers/world.go ColonizePreview is FOW-critical); nil runs
// the full, unfiltered catchment for trusted internal callers.
//
// Shares automatic grain/fish with RecomputeProduction: near ground plus
// the remainder people's fraction of the best real food worker. Placed
// workers use the same hex yields, and consumption remains population-wide.
type FoundingFoodForecast struct {
	GrainProduction, FishProduction, Consumption, GrainNet, FoodNet float64
}

// FoundingGrainNetPerTick preserves the grain-only forecast for existing callers.
func FoundingGrainNetPerTick(ctx context.Context, tx Tx, worldID uuid.UUID, center hexgrid.Coord, hasFarm bool, reachable map[hexgrid.Coord]bool, pop int) (float64, float64, error) {
	f, err := FoundingFoodForecastAt(ctx, tx, worldID, center, hasFarm, reachable, pop)
	return f.GrainProduction, f.GrainNet, err
}

// FoundingFoodForecastAt exposes the SAME placement forecast and population demand
// separately: grain net alone cannot reveal how much food fish supplies.
func FoundingFoodForecastAt(ctx context.Context, tx Tx, worldID uuid.UUID, center hexgrid.Coord, hasFarm bool, reachable map[hexgrid.Coord]bool, pop int) (FoundingFoodForecast, error) {
	bs := BuildingSet{}
	if hasFarm {
		farmHex, ok, chooseErr := ChooseFarmHex(ctx, tx, worldID, center, reachable)
		if chooseErr != nil {
			return FoundingFoodForecast{}, fmt.Errorf("founding grain net per tick: %w", chooseErr)
		}
		if ok {
			bs = BuildingSet{Hex: map[hexgrid.Coord]map[string]int{farmHex: {"farm": 1}}}
		}
	}
	hexOptions, err := LoadHexProductionOptionsAt(ctx, tx, worldID, center, bs, reachable)
	if err != nil {
		return FoundingFoodForecast{}, fmt.Errorf("founding grain net per tick: %w", err)
	}
	slots := rankSlotsFromOptions(hexOptions, center)

	automatic := AutomaticFoodForPopulation(hexOptions, pop)
	totalGubbar := pop / 100
	guaranteedFood := automatic.FoodPerTick()
	demand := GrainConsumptionPerTick(pop)

	placements, _ := placeStartingGubbar(slots, guaranteedFood, demand, totalGubbar)

	grainProd := automatic.GrainPerTick()
	fishProd := automatic.RemainderFish
	for _, p := range placements {
		if p.good == GoodGrain {
			grainProd += p.yield
		} else {
			fishProd += p.yield
		}
	}

	grainNet, _, _ := FoodConsumptionSplit(demand, grainProd, fishProd, 0)
	return FoundingFoodForecast{GrainProduction: grainProd, FishProduction: fishProd, Consumption: demand, GrainNet: grainNet, FoodNet: FoodNet(grainProd+fishProd, pop)}, nil
}

// ForecastBalanceEpsilon only suppresses floating-point dust in projections;
// it does not change FoodTick debits or the growth gate.
const ForecastBalanceEpsilon = 1e-9

func FoodRunwayTicks(stock, net float64) *float64 {
	if net >= -ForecastBalanceEpsilon {
		return nil
	}
	ticks := stock / -net
	return &ticks
}
