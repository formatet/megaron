package economy

// Byggnadsregeln (megaron_plan_byggnadsregeln.md, Timothy 2026-09-30).
//
// For a hex h and a good g that h can give:
//   - basePlaces P0(h,g) comes from the terrain/deposit (hexRules below) and the
//     base rate per worker r0(h,g) from production_rules (building NULL rows,
//     plus the silver/mine rows, which only apply when a mine stands on the hex);
//   - when the rule's relevant building stands on h (harbour: the city's harbour,
//     acting on deep_sea fishing only; fish has no building rate multiplier) at level L >= 1, places = P0 + BuildingExtraPlaces
//     (same at every level) and rate per worker = r0 * (1 + BuildingRatePerLevel*L);
//   - otherwise places = P0 and rate per worker = r0.
//
// Output = rate per worker * min(placed, places). Fishing places and intrinsic
// bay yield are resolved by this owner before consumers see the options.
// Silver requires its mine row; deep-water fish requires harbour places. Silver
// production is additionally gated by its catalogue rows: its
// production_rules rows are the mine rows, which only load when a mine stands
// on the hex.

import (
	"context"
	"fmt"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// Deep-water places are the harbour's entire fishing effect. Coastal water
// remains two places; bays raise the intrinsic rate, not the headcount.
var deepFishingPlaces = [4]int{0, 1, 2, 4}

// BuildingExtraPlaces is how many worker places a hex-bound building adds to its hex.
const BuildingExtraPlaces = 4

// BuildingRatePerLevel is the per-level raise of the hex's per-worker rate:
// multiplier = 1 + BuildingRatePerLevel * level.
const BuildingRatePerLevel = 0.7

// HexFallbackCap is the per-hex worker places for a good with a real
// production_rules rate but no rule in hexRules (oil, wine). No building raises them.
const HexFallbackCap = 2

// hexRule is one row of the rule table.
type hexRule struct {
	good       string
	basePlaces int
	building   string  // "" = no building raises this (good, hex)
	rateMult   float64 // 0 means 1; bays have intrinsic rate 2
}

// fieldStone is the stone every land hex without a quarry-grade deposit gives
// (mig 156, Timothy 2026-10-02): one place, raised by the stonequarry. Its base
// rate (0,5) is the production_rules row; hills and limestone keep their own.
var fieldStone = hexRule{"stone", 1, "stonequarry", 0}

// hexRules returns the rule rows for a hex of this terrain and deposits.
func hexRules(terrain string, copperDep, tinDep, silverDep, incenseDep bool) []hexRule {
	var out []hexRule
	switch terrain {
	case "plains":
		out = append(out, hexRule{GoodGrain, 4, "farm", 0}, fieldStone)
	case "hills":
		out = append(out, hexRule{GoodGrain, 1, "farm", 0}, hexRule{"stone", 2, "stonequarry", 0}, hexRule{"timber", 1, "lumbermill", 0})
	case "mountain_limestone":
		out = append(out, hexRule{"stone", 2, "stonequarry", 0})
	case "river_valley":
		out = append(out, hexRule{GoodGrain, 2, "farm", 0}, fieldStone, hexRule{"timber", 1, "lumbermill", 0})
	case "river_delta":
		out = append(out, hexRule{GoodGrain, 3, "farm", 0}, fieldStone)
	case "forest_olive_grove":
		out = append(out, hexRule{"timber", 1, "lumbermill", 0}, fieldStone)
	case "forest_cedar":
		out = append(out, hexRule{"cedar", 3, "lumbermill", 0}, fieldStone)
	case "scrub_maquis":
		out = append(out, fieldStone, hexRule{"timber", 1, "lumbermill", 0})
	case "semi_desert", "mountain_red":
		out = append(out, fieldStone)
	case "coastal_sea":
		out = append(out, hexRule{"fish", 2, "", 0}) // 2 places: fisket ska kunna ge mycket mat (Timothy 2026-10-01); rate stays 1,0 — the dagsverkesskala
	case "deep_sea":
		out = append(out, hexRule{"fish", 0, "harbour", 0})
	case "river", "river_ford":
		out = append(out, hexRule{"fish", 1, "", 0})
	}
	if copperDep {
		out = append(out, hexRule{"copper", 1, "mine", 0})
	}
	if tinDep {
		out = append(out, hexRule{"tin", 1, "mine", 0})
	}
	if silverDep {
		out = append(out, hexRule{"silver", 1, "mine", 0})
	}
	if incenseDep {
		// Two places, no building (Timothy 2026-10-19): one hex, two gubbar. The
		// production_rules row (mig 193) carries requires_deposit='incense'.
		out = append(out, hexRule{GoodIncense, 2, "", 0})
	}
	return out
}

// hexRuleTile is the context-bearing rule owner. Consumers load it before
// calculating options, aggregate capacity, building effects or affected goods.
// Neighbor terrain stays inside economy; only the resulting local capacity is exposed.
type hexRuleTile struct {
	terrain                      string
	copper, tin, silver, incense bool
	rules                        []hexRule
}

func loadHexRuleTiles(ctx context.Context, tx Tx, worldID uuid.UUID, coords []hexgrid.Coord) (map[hexgrid.Coord]hexRuleTile, error) {
	qs, rs := hexgrid.QRArrays(coords)
	dirs := hexgrid.Neighbors(hexgrid.Coord{})
	dqs, drs := hexgrid.QRArrays(dirs[:])
	rows, err := tx.Query(ctx, `SELECT mt.q,mt.r,mt.terrain,COALESCE(mt.copper_deposit,false),
 COALESCE(mt.tin_deposit,false),COALESCE(mt.silver_deposit,false),COALESCE(mt.incense_deposit,false),
 CASE WHEN mt.terrain='coastal_sea' THEN (SELECT count(*) FROM unnest($4::int[],$5::int[]) d(q,r)
 JOIN map_tiles n ON n.world_id=mt.world_id AND n.q=mt.q+d.q AND n.r=mt.r+d.r
 WHERE n.terrain NOT IN ('sea','coastal_sea','deep_sea')) ELSE 0 END
 FROM unnest($2::int[],$3::int[]) wanted(q,r)
 JOIN map_tiles mt ON mt.world_id=$1 AND mt.q=wanted.q AND mt.r=wanted.r`, worldID, qs, rs, dqs, drs)
	if err != nil {
		return nil, fmt.Errorf("load hex rules: %w", err)
	}
	defer rows.Close()
	out := make(map[hexgrid.Coord]hexRuleTile, len(coords))
	for rows.Next() {
		var c hexgrid.Coord
		var tile hexRuleTile
		var land int
		if err := rows.Scan(&c.Q, &c.R, &tile.terrain, &tile.copper, &tile.tin, &tile.silver, &tile.incense, &land); err != nil {
			return nil, fmt.Errorf("load hex rules: scan: %w", err)
		}
		tile.rules = rulesWithLandNeighbors(tile.terrain, tile.copper, tile.tin, tile.silver, tile.incense, land)
		out[c] = tile
	}
	return out, rows.Err()
}

func rulesWithLandNeighbors(terrain string, copper, tin, silver, incense bool, land int) []hexRule {
	rules := hexRules(terrain, copper, tin, silver, incense)
	if terrain == "coastal_sea" && land >= 4 {
		for i := range rules {
			if rules[i].good == "fish" {
				rules[i].rateMult = 2
			}
		}
	}
	return rules
}

func (tile hexRuleTile) goodPlaces(levels map[string]int, good string) (int, float64) {
	for _, rule := range tile.rules {
		if rule.good == good {
			return rule.placesAndMult(levels[rule.building])
		}
	}
	return HexFallbackCap, 1
}

// RuleBuildingSupportsHex reports whether the rule table names buildingType as
// the relevant building for some good on a hex of this terrain/deposits — the
// "may this building stand here" gate.
func RuleBuildingSupportsHex(buildingType, terrain string, copperDep, tinDep, silverDep, incenseDep bool) bool {
	for _, r := range hexRules(terrain, copperDep, tinDep, silverDep, incenseDep) {
		if r.building == buildingType {
			return true
		}
	}
	return false
}

// placesAndMult applies the rule at a building level (0 = not built).
func (r hexRule) placesAndMult(level int) (places int, mult float64) {
	if r.good == "fish" {
		if r.building == "harbour" {
			return deepFishingPlaces[min(3, max(0, level))], 1
		}
		mult := r.rateMult
		if mult == 0 {
			mult = 1
		}
		return r.basePlaces, mult
	}

	if r.building == "" || level <= 0 {
		return r.basePlaces, 1.0
	}
	return r.basePlaces + BuildingExtraPlaces, 1.0 + BuildingRatePerLevel*float64(level)
}

// hexGoodPlaces returns places and rate multiplier for one good on one hex.
// A good without a rule (oil, wine) gets HexFallbackCap
// places and no multiplier. levels is the building-level map scoped to the hex
// (BuildingSet.levelsAt).
func hexGoodPlaces(terrain string, copperDep, tinDep, silverDep, incenseDep bool, levels map[string]int, good string) (places int, mult float64) {
	for _, r := range hexRules(terrain, copperDep, tinDep, silverDep, incenseDep) {
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

// ruleTerrains lists every terrain hexRules has a row for.
var ruleTerrains = []string{"plains", "hills", "mountain_limestone", "river_valley", "river_delta",
	"forest_olive_grove", "forest_cedar", "scrub_maquis", "semi_desert", "mountain_red", "coastal_sea", "river", "river_ford", "deep_sea"}

// RuleBuildingTypes are the buildings the rule table ties to hexes.
var RuleBuildingTypes = []string{"farm", "mine", "lumbermill", "stonequarry", "harbour"}

// RuleBuildingGate returns where buildingType can stand according to the rule
// table: the terrains that carry a rule naming it, and the deposits (copper, tin,
// silver) that do. A building needs ANY of them; a deposit-only building (mine)
// returns no terrains. The catalogue shows this as requires_terrain /
// requires_deposits.
func RuleBuildingGate(buildingType string) (terrains, deposits []string) {
	for _, t := range ruleTerrains {
		if RuleBuildingSupportsHex(buildingType, t, false, false, false, false) {
			terrains = append(terrains, t)
		}
	}
	for _, d := range []string{"copper", "tin", "silver"} {
		if RuleBuildingSupportsHex(buildingType, "", d == "copper", d == "tin", d == "silver", false) {
			deposits = append(deposits, d)
		}
	}
	return terrains, deposits
}

// RuleBuildingGoods lists the goods the rule table ties buildingType to on the
// hex it was built on (hexQ/hexR), or — for a city-scope rule building (harbour)
// — on any hex of the settlement's catchment. Only goods the world has a
// production row for are listed (the same filter HexBuildEffects applies is not
// needed here: the list feeds a "staff it" hint, not a number).
func RuleBuildingGoods(ctx context.Context, tx Tx, settlementID uuid.UUID, buildingType string, hexQ, hexR *int) ([]string, error) {
	var worldID uuid.UUID
	var cq, cr int
	if err := tx.QueryRow(ctx,
		`SELECT prov.world_id, prov.map_q, prov.map_r FROM settlements s
		 JOIN provinces prov ON prov.id = s.province_id WHERE s.id = $1`, settlementID,
	).Scan(&worldID, &cq, &cr); err != nil {
		return nil, fmt.Errorf("rule building goods: %w", err)
	}
	hexes := hexgrid.Ring(hexgrid.Coord{Q: cq, R: cr}, hexgrid.CatchmentRadius)
	if hexQ != nil && hexR != nil {
		hexes = []hexgrid.Coord{{Q: *hexQ, R: *hexR}}
	}
	tiles, err := loadHexRuleTiles(ctx, tx, worldID, hexes)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var out []string
	for _, c := range hexes {
		for _, rule := range tiles[c].rules {
			if rule.building == buildingType && !seen[rule.good] {
				seen[rule.good] = true
				out = append(out, rule.good)
			}
		}
	}

	return out, nil
}

// IsRuleBuilding reports whether buildingType is one of RuleBuildingTypes.
func IsRuleBuilding(buildingType string) bool {
	for _, b := range RuleBuildingTypes {
		if b == buildingType {
			return true
		}
	}
	return false
}
