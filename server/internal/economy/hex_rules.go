package economy

// Byggnadsregeln (megaron_plan_byggnadsregeln.md, Timothy 2026-09-30).
//
// For a hex h and a good g that h can give:
//   - basePlaces P0(h,g) comes from the terrain/deposit (hexRules below) and the
//     base rate per worker r0(h,g) from production_rules (building NULL rows,
//     plus the silver/mine rows, which only apply when a mine stands on the hex);
//   - when the rule's relevant building stands on h (harbour: the city's harbour,
//     acting on every coastal_sea hex) at level L >= 1, places = P0 + BuildingExtraPlaces
//     (same at every level) and rate per worker = r0 * (1 + BuildingRatePerLevel*L);
//   - otherwise places = P0 and rate per worker = r0.
//
// Output = rate per worker * min(placed, places). No good is an exception.
// Silver is the one good the rule cannot give without its building: its
// production_rules rows are the mine rows, which only load when a mine stands
// on the hex.


// BuildingExtraPlaces is how many worker places a hex-bound building adds to its hex.
const BuildingExtraPlaces = 4

// BuildingRatePerLevel is the per-level raise of the hex's per-worker rate:
// multiplier = 1 + BuildingRatePerLevel * level.
const BuildingRatePerLevel = 0.7

// HexFallbackCap is the per-hex worker places for a good with a real
// production_rules rate but no rule in hexRules (oil, wine, timber on cedar
// forest). No building raises them.
const HexFallbackCap = 2

// hexRule is one row of the rule table.
type hexRule struct {
	good       string
	basePlaces int
	building   string // "" = no building raises this (good, hex)
}

// hexRules returns the rule rows for a hex of this terrain and deposits.
func hexRules(terrain string, copperDep, tinDep, silverDep bool) []hexRule {
	var out []hexRule
	switch terrain {
	case "plains":
		out = append(out, hexRule{GoodGrain, 4, "farm"}, hexRule{"livestock", 1, ""})
	case "hills":
		out = append(out, hexRule{GoodGrain, 1, "farm"}, hexRule{"stone", 2, "stonequarry"})
	case "mountain_limestone":
		out = append(out, hexRule{"stone", 2, "stonequarry"})
	case "river_valley":
		out = append(out, hexRule{GoodGrain, 2, "farm"})
	case "river_delta":
		out = append(out, hexRule{GoodGrain, 3, "farm"})
	case "forest_olive_grove":
		out = append(out, hexRule{"timber", 1, "lumbermill"})
	case "forest_cedar":
		out = append(out, hexRule{"cedar", 1, "lumbermill"})
	case "coastal_sea":
		out = append(out, hexRule{"fish", 1, "harbour"})
	case "river", "river_ford", "deep_sea":
		out = append(out, hexRule{"fish", 1, ""})
	}
	if copperDep {
		out = append(out, hexRule{"copper", 1, "mine"})
	}
	if tinDep {
		out = append(out, hexRule{"tin", 1, "mine"})
	}
	if silverDep {
		out = append(out, hexRule{"silver", 1, "mine"})
	}
	return out
}

// RuleBuildingSupportsHex reports whether the rule table names buildingType as
// the relevant building for some good on a hex of this terrain/deposits — the
// "may this building stand here" gate.
func RuleBuildingSupportsHex(buildingType, terrain string, copperDep, tinDep, silverDep bool) bool {
	for _, r := range hexRules(terrain, copperDep, tinDep, silverDep) {
		if r.building == buildingType {
			return true
		}
	}
	return false
}

// placesAndMult applies the rule at a building level (0 = not built).
func (r hexRule) placesAndMult(level int) (places int, mult float64) {
	if r.building == "" || level <= 0 {
		return r.basePlaces, 1.0
	}
	return r.basePlaces + BuildingExtraPlaces, 1.0 + BuildingRatePerLevel*float64(level)
}

// hexGoodPlaces returns places and rate multiplier for one good on one hex.
// A good without a rule (oil, wine, timber on cedar forest) gets HexFallbackCap
// places and no multiplier. levels is the building-level map scoped to the hex
// (BuildingSet.levelsAt).
func hexGoodPlaces(terrain string, copperDep, tinDep, silverDep bool, levels map[string]int, good string) (places int, mult float64) {
	for _, r := range hexRules(terrain, copperDep, tinDep, silverDep) {
		if r.good == good {
			return r.placesAndMult(levels[r.building])
		}
	}
	return HexFallbackCap, 1.0
}

// hexYield is the ONE hex-yield formula: rate per worker (already r0) times the
// building multiplier times the workers that fit. Building workplaces (olive
// press, winery, foundry, …) do not use it — they keep placementYield.
func hexYield(rate float64, places int, mult float64, placed int) float64 {
	if places <= 0 || rate <= 0 {
		return 0
	}
	if placed > places {
		placed = places // defensive — Place() enforces the cap at write time
	}
	return rate * mult * float64(placed)
}

// HexYieldPerWorker is the per-worker output of one option entry (rate * mult).
func HexYieldPerWorker(rate, mult float64) float64 { return rate * mult }

