package economy

import (
	"context"
	"fmt"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// HexOption is one catchment ring hex's production menu — every good it can
// support, its OWN rate_per_tick (not the catchment aggregate) and its OWN
// worker cap (P3, hexCapacityRule). P4 (megaron_plan_fysisk_gubbemodell.md):
// a gubbe stands on ONE hex doing ONE good — "en skogshuggare gör bara
// virke/ceder (beroende på hex)" (Timothy 2026-08-08) — so production must be
// derivable per hex, not just as a catchment-wide sum.
type HexOption struct {
	Coord       hexgrid.Coord
	Terrain     string
	RatePerGood map[string]float64 // production_rules rate_per_tick, gated by settlement's actual buildings
	CapPerGood  map[string]int     // P3 worker cap (hexCapacityRule), gated the same way
}

// LoadHexProductionOptions returns every catchment ring hex's own production
// menu. Mirrors CatchmentBasePotential's join (same terrain/deposit/coastal/
// building gating, same water-tile exclusion) but keeps each hex's row
// separate instead of collapsing into one SUM per good — RecomputeProduction
// needs the per-hex rate to divide by that hex's own cap (yield_per_worker),
// not the catchment total. If this ever drifts from CatchmentBasePotential /
// RecomputeProduction's old aggregate query, treat this one as source of
// truth for PLACED production (recompute.go no longer uses the aggregate for
// placeable goods after P4).
func LoadHexProductionOptions(ctx context.Context, tx Tx, settlementID uuid.UUID) ([]HexOption, error) {
	var worldID uuid.UUID
	var q, r int
	if err := tx.QueryRow(ctx,
		`SELECT prov.world_id, prov.map_q, prov.map_r
		 FROM settlements s JOIN provinces prov ON prov.id = s.province_id
		 WHERE s.id = $1`,
		settlementID,
	).Scan(&worldID, &q, &r); err != nil {
		return nil, fmt.Errorf("load hex production options: settlement coords: %w", err)
	}

	buildingLevels, err := loadBuildingLevels(ctx, tx, settlementID)
	if err != nil {
		return nil, fmt.Errorf("load hex production options: %w", err)
	}

	catchQ, catchR := hexgrid.QRArrays(hexgrid.Ring(hexgrid.Coord{Q: q, R: r}, hexgrid.CatchmentRadius))
	rows, err := tx.Query(ctx,
		`SELECT mt.q, mt.r, mt.terrain,
		        COALESCE(mt.copper_deposit, false), COALESCE(mt.tin_deposit, false), COALESCE(mt.silver_deposit, false),
		        pr.good_key, pr.rate_per_tick
		 FROM unnest($2::int[], $3::int[]) AS catchment(q, r)
		 JOIN map_tiles mt ON mt.world_id = $1 AND mt.q = catchment.q AND mt.r = catchment.r
		 JOIN production_rules pr ON
		     (pr.terrain_type IS NULL OR pr.terrain_type = mt.terrain)
		     AND (NOT pr.requires_coastal OR mt.coastal)
		     AND (pr.building_type IS NULL OR EXISTS (
		             SELECT 1 FROM buildings b
		             WHERE b.settlement_id = $4 AND b.building_type = pr.building_type))
		     AND (pr.requires_deposit IS NULL
		          OR (pr.requires_deposit = 'copper' AND mt.copper_deposit)
		          OR (pr.requires_deposit = 'tin'    AND mt.tin_deposit)
		          OR (pr.requires_deposit = 'silver' AND COALESCE(mt.silver_deposit, false))
		          OR (pr.requires_deposit = 'cedar'  AND COALESCE(mt.cedar_deposit, false)))
		 JOIN goods g ON g.key = pr.good_key AND g.status = 'active'
		 WHERE mt.terrain NOT IN ('deep_sea','coastal_sea','river','river_ford')
		        OR pr.terrain_type = mt.terrain`,
		worldID, catchQ, catchR, settlementID,
	)
	if err != nil {
		return nil, fmt.Errorf("load hex production options: query: %w", err)
	}
	defer rows.Close()

	byCoord := make(map[hexgrid.Coord]*HexOption)
	var order []hexgrid.Coord
	for rows.Next() {
		var qq, rr int
		var terrain, goodKey string
		var copperDep, tinDep, silverDep bool
		var rate float64
		if err := rows.Scan(&qq, &rr, &terrain, &copperDep, &tinDep, &silverDep, &goodKey, &rate); err != nil {
			return nil, fmt.Errorf("load hex production options: scan: %w", err)
		}
		c := hexgrid.Coord{Q: qq, R: rr}
		opt, ok := byCoord[c]
		if !ok {
			opt = &HexOption{
				Coord:       c,
				Terrain:     terrain,
				RatePerGood: make(map[string]float64),
				CapPerGood:  hexGoodCaps(terrain, copperDep, tinDep, silverDep, buildingLevels),
			}
			byCoord[c] = opt
			order = append(order, c)
		}
		opt.RatePerGood[goodKey] += rate
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load hex production options: rows: %w", err)
	}

	out := make([]HexOption, 0, len(order))
	for _, c := range order {
		opt := byCoord[c]
		// Fallback cap (GoodLaborTerrainBase's placement-era sibling): a good
		// can have a real production_rules rate on this hex (RatePerGood) with
		// no matching P3 hexCapacityRule entry — oil/wine/stone are the three
		// currently uncovered goods (Temenos_varutaxonomi_sol.md §8.3 lists ten
		// rows, none of them). P3 hit exactly this gap live
		// (TestRecomputeProduction_WineOn{RiverValley,Plains}OnlyCatchment went
		// to 0) and kept GoodLaborTerrainBase as an explicit share-based
		// fallback rather than silently dropping them; HexFallbackCap is the
		// same fallback translated to an absolute per-hex placement cap. A
		// good that's here ONLY because a required building doesn't exist
		// never reaches this point — the SQL's building EXISTS-gate already
		// excludes it from RatePerGood entirely, so every key seen here is
		// legitimately placeable.
		for good := range opt.RatePerGood {
			if _, covered := opt.CapPerGood[good]; !covered {
				opt.CapPerGood[good] = HexFallbackCap
			}
		}
		out = append(out, *opt)
	}
	return out, nil
}

// HexFallbackCap is the per-hex worker cap for a good with a real
// production_rules rate but no P3 hexCapacityRule entry (oil, wine, stone —
// see the fallback comment above). A placeholder calibration ratt, not a
// lock — matches the low end of P3's own capNoBuilding tiers (most are 1-2).
const HexFallbackCap = 2

// placementYield is the rate a target contributes for one good, given how
// many gubbar are placed there. Every good except grain is capacity-clamped
// (yield_per_worker = rate/cap, placed clamped to cap — a physically full
// hex/building produces no more no matter how many more gubbar queue up).
//
// Grain is exempt — carried over from the pre-P4 LaborCapacity's identical
// special case ("Grain is exempt: it is the subsistence good... capping how
// many citizens may farm would starve cities that have no room to build.
// Hunger is not a staffing problem"). This is not cosmetic: production_rules'
// grain rates were calibrated against an UNCAPPED effective-workers term
// (weight × population, historically up to thousands) — the P3 hex caps
// (e.g. plains+farm = 6 gubbar) were never meant to gate grain too, and doing
// so starves every city, caught live by
// TestApplyDecay_GrainFundedGrowth_MinimalCitySelfSufficient going from a
// thin-but-positive minimum grain (16.6) to hard 0 the moment grain's yield
// used the same rate/cap division as every other good.
// yield = rate × placed, uncapped: since REF_LABOR (100) is exactly one
// gubbe's citizen-count (Temenos_varutaxonomi_sol.md §1.1), a single placed
// grain-gubbe reproduces the OLD model's per-100-citizens contribution
// exactly, and stacking more gubbar on the same grain hex is deliberately
// allowed — the hex's PHYSICAL capacity was never the food constraint,
// population's total food NEED is.
func placementYield(good string, rate float64, cap int, placed int) float64 {
	if good == GoodGrain {
		return rate * float64(placed)
	}
	if cap <= 0 {
		return 0
	}
	if placed > cap {
		placed = cap // defensive — Place() enforces the cap at write time, never trust a stale read
	}
	return (rate / float64(cap)) * float64(placed)
}

// hexGoodCaps returns every good_key a single hex (given its terrain and
// deposit flags) can cap, and that good's worker cap — the per-hex sibling of
// LoadHexCapacity's aggregate sum (P3). A hex can independently cap MULTIPLE
// goods (a plains hex is both "slätt" (grain) and "betesmark" (livestock)).
//
// capOf adds the gating building's OWN P2 workplace slots (WorkplaceSlots) on
// top of P3's capWithBuilding tier when that building is present. Restores a
// real regression caught building this: the pre-P4 aggregate model summed
// hexSlots (P3) AND buildingSlots (P2) as two INDEPENDENT capacity pools for
// the same farm-gated grain hex (LoadWorkplaceSlots' JOIN matches ANY
// production_rules row for building_type='farm', including plains+farm,
// regardless of that row also naming a terrain) — so a farm didn't just raise
// a hex's tier, it separately added its own 2 workers on top. Dropping that
// second pool at P4 broke TestApplyDecay_GrainFundedGrowth_MinimalCitySelfSufficient
// (a previously-green hard invariant: a neglected 5000-pop start city with
// exactly one farmable hex must never starve — min grain observed on master
// was already a thin 16.6, so losing farm's +2 workers pushed it to 0).
func hexGoodCaps(terrain string, copperDep, tinDep, silverDep bool, buildingLevels map[string]int) map[string]int {
	capOf := func(rule hexCapacityRule) int {
		if rule.relevantBuilding == "" {
			return rule.capNoBuilding
		}
		level := buildingLevels[rule.relevantBuilding]
		if level <= 0 {
			return rule.capNoBuilding
		}
		return rule.capWithBuilding + WorkplaceSlots(rule.relevantBuilding, level)
	}
	caps := make(map[string]int)
	if terrain == "plains" {
		for _, rule := range plainsCapacityRules {
			caps[rule.goodKey] += capOf(rule)
		}
	} else if rule, ok := terrainCapacityTable[terrain]; ok {
		caps[rule.goodKey] += capOf(rule)
	}
	if copperDep {
		rule := depositCapacityTable["copper"]
		caps[rule.goodKey] += capOf(rule)
	}
	if tinDep {
		rule := depositCapacityTable["tin"]
		caps[rule.goodKey] += capOf(rule)
	}
	if silverDep {
		rule := depositCapacityTable["silver"]
		caps[rule.goodKey] += capOf(rule)
	}
	return caps
}

// BuildingOption is one settlement-wide workplace building's production menu
// — mirrors HexOption for target_kind='building' placements (P0-UI's
// "STADENS ARBETSPLATSER"). Only production_rules rows with NO terrain gate
// (pr.terrain_type IS NULL) qualify: a rule that also needs a terrain (e.g.
// today's farm+plains→grain) is catchment-hex production with a building
// REQUIREMENT, not a pure building workplace — that stays a HexOption. No
// currently-active good has a terrain-free building rule yet (bronze via
// foundry runs through recipe.go/craft-events, not RecomputeProduction;
// market/stable's goods are parked) — this exists so P5/P6 don't need a
// second migration when one lands.
type BuildingOption struct {
	BuildingType string
	Level        int
	RatePerGood  map[string]float64
	CapPerGood   map[string]int
}

// LoadBuildingProductionOptions returns every built (level >= 1) workplace
// building's terrain-free production menu.
func LoadBuildingProductionOptions(ctx context.Context, tx Tx, settlementID uuid.UUID) ([]BuildingOption, error) {
	rows, err := tx.Query(ctx,
		`SELECT b.building_type, b.level, pr.good_key, pr.rate_per_tick
		 FROM buildings b
		 JOIN production_rules pr ON pr.building_type = b.building_type AND pr.terrain_type IS NULL
		 JOIN goods g ON g.key = pr.good_key AND g.status = 'active'
		 WHERE b.settlement_id = $1 AND b.level >= 1`,
		settlementID,
	)
	if err != nil {
		return nil, fmt.Errorf("load building production options: query: %w", err)
	}
	defer rows.Close()

	byType := make(map[string]*BuildingOption)
	var order []string
	for rows.Next() {
		var buildingType, goodKey string
		var level int
		var rate float64
		if err := rows.Scan(&buildingType, &level, &goodKey, &rate); err != nil {
			return nil, fmt.Errorf("load building production options: scan: %w", err)
		}
		opt, ok := byType[buildingType]
		if !ok {
			opt = &BuildingOption{
				BuildingType: buildingType,
				Level:        level,
				RatePerGood:  make(map[string]float64),
				CapPerGood:   make(map[string]int),
			}
			byType[buildingType] = opt
			order = append(order, buildingType)
		}
		opt.RatePerGood[goodKey] += rate
		opt.CapPerGood[goodKey] = WorkplaceSlots(buildingType, level)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load building production options: rows: %w", err)
	}

	out := make([]BuildingOption, 0, len(order))
	for _, bt := range order {
		out = append(out, *byType[bt])
	}
	return out, nil
}

// loadBuildingLevels returns every building type the settlement has built,
// mapped to its level — hexGoodCaps needs the level (not just presence) to
// add the building's own WorkplaceSlots on top of the hex tier.
func loadBuildingLevels(ctx context.Context, tx Tx, settlementID uuid.UUID) (map[string]int, error) {
	rows, err := tx.Query(ctx, `SELECT building_type, level FROM buildings WHERE settlement_id = $1`, settlementID)
	if err != nil {
		return nil, fmt.Errorf("load building levels: %w", err)
	}
	defer rows.Close()
	built := make(map[string]int)
	for rows.Next() {
		var bt string
		var level int
		if err := rows.Scan(&bt, &level); err != nil {
			return nil, fmt.Errorf("load building levels: scan: %w", err)
		}
		built[bt] = level
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load building levels: rows: %w", err)
	}
	return built, nil
}

// UnconditionalPotential returns every good's flat, unconditional trickle —
// production_rules rows with NEITHER a terrain gate NOR a building gate
// (currently just timber, migration 033: "anti-deadlock" — SOME timber
// production must exist even in a catchment with no forest). This is added
// directly to a good's rate regardless of placement, the same way
// NearjordGrainPerTick is: its entire purpose is a guaranteed minimum, not a
// meaningful worker choice, so P4 does not turn it into a placeable role.
func UnconditionalPotential(ctx context.Context, tx Tx) (map[string]float64, error) {
	rows, err := tx.Query(ctx,
		`SELECT pr.good_key, SUM(pr.rate_per_tick)
		 FROM production_rules pr
		 JOIN goods g ON g.key = pr.good_key AND g.status = 'active'
		 WHERE pr.terrain_type IS NULL AND pr.building_type IS NULL
		 GROUP BY pr.good_key`,
	)
	if err != nil {
		return nil, fmt.Errorf("unconditional potential: query: %w", err)
	}
	defer rows.Close()
	out := make(map[string]float64)
	for rows.Next() {
		var k string
		var v float64
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("unconditional potential: scan: %w", err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("unconditional potential: rows: %w", err)
	}
	return out, nil
}

// PlacementCounts is a settlement's placed workforce, grouped for
// RecomputeProduction's lookup: how many gubbar stand on hex Coord doing
// good_key, and how many work in building_type doing good_key.
type PlacementCounts struct {
	Hex      map[hexgrid.Coord]map[string]int // coord -> good_key -> count
	Building map[string]map[string]int        // building_type -> good_key -> count
	Total    int                              // every placed gubbe, any target — for the pool-size calculation
}

// LoadPlacementCounts reads every settlement_placement row for settlementID
// and groups it for yield computation. Does not validate against caps — caps
// are enforced at placement time (Place); a row existing here is assumed
// already legal.
func LoadPlacementCounts(ctx context.Context, tx Tx, settlementID uuid.UUID) (PlacementCounts, error) {
	out := PlacementCounts{
		Hex:      make(map[hexgrid.Coord]map[string]int),
		Building: make(map[string]map[string]int),
	}
	rows, err := tx.Query(ctx,
		`SELECT target_kind, hex_q, hex_r, building_type, good_key
		 FROM settlement_placement WHERE settlement_id = $1`,
		settlementID,
	)
	if err != nil {
		return out, fmt.Errorf("load placement counts: query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind, goodKey string
		var hexQ, hexR *int
		var buildingType *string
		if err := rows.Scan(&kind, &hexQ, &hexR, &buildingType, &goodKey); err != nil {
			return out, fmt.Errorf("load placement counts: scan: %w", err)
		}
		out.Total++
		switch kind {
		case "hex":
			c := hexgrid.Coord{Q: *hexQ, R: *hexR}
			if out.Hex[c] == nil {
				out.Hex[c] = make(map[string]int)
			}
			out.Hex[c][goodKey]++
		case "building":
			if out.Building[*buildingType] == nil {
				out.Building[*buildingType] = make(map[string]int)
			}
			out.Building[*buildingType][goodKey]++
		}
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("load placement counts: rows: %w", err)
	}
	return out, nil
}
