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
	CapPerGood  map[string]int     // P3 worker cap (hexCapacityRule) AT THE ACTUAL building level, gated the same way

	// CapL1PerGood is CapPerGood's Form A sibling (megaron_byggnadsniva_produktion.md,
	// Timothy 2026-08-22): the SAME cap frozen at building level 1
	// (capWithBuilding + WorkplaceSlots(b, 1), or capNoBuilding when no
	// relevant building exists at all). Form B (megaron_plan_byggnadsniva_takt.md,
	// Timothy 2026-08-24) keeps this as placementYield's RATE denominator —
	// unchanged role from Form A. What moved is the CLIP ceiling: see
	// PlaceCapPerGood below.
	CapL1PerGood map[string]int

	// MultPerGood is Form B's per-good multiplier: cap(actualLevel)/capL1,
	// i.e. the SAME ratio Form A used to apply to headcount, now applied to
	// the per-gubbe rate instead. At building level 1 (or no gating building
	// at all) cap==capL1 so this is 1.0 — unchanged from Form A/pre-Form-A.
	// A higher level raises this above 1.0, so the SAME capL1 gubbar produce
	// more. grain's entry is pinned to 1.0 unconditionally (see hexGoodCaps)
	// — the grain-cap plan's numbers must not move because of this slice.
	MultPerGood map[string]float64

	// PlaceCapPerGood is the number of gubbar that may actually be PLACED for
	// this good here — what placementYield clips against, and what Place()
	// enforces at write time. For every good EXCEPT grain this is CapL1PerGood
	// (Form B's whole point: headcount frozen at level 1, the level's effect
	// moved to MultPerGood instead). Grain is the one deliberate exception
	// (hexGoodCaps): its capL1 is a purely mathematical trick (pinned to 1 so
	// rate/capL1 reproduces rate×placed) that was NEVER a real headcount
	// limit, so grain's PlaceCapPerGood stays CapPerGood — the REAL,
	// level-actual cap from megaron_plan_grain_cap.md (4/8/10/12 per plains
	// hex by farm level), completely untouched by this slice. Blindly using
	// CapL1PerGood as the clip for grain too would silently cut every grain
	// hex down to ONE placeable gubbe — caught by
	// TestPlaceGubbe_GrainHexRejectsOverCapacity during this slice's build.
	PlaceCapPerGood map[string]int

	// BoostRatePerGood holds the SAME extraction gubbe's terrain+building
	// combined rows for weakestLinkRefiningBuilding's goods (oil, wine — P6,
	// megaron_plan_fysisk_gubbemodell.md §P6) — e.g. forest_olive_grove +
	// olive_press → 72.0 oil/tick. Before P6 this flowed straight into
	// RatePerGood the moment the building merely EXISTED, no second worker
	// required. Now it is "potential" only: RecomputeProduction realizes it as
	// min(boostPotential, refiningCapacity), where refiningCapacity comes from
	// a SEPARATE gubbe placed IN the building (LoadBuildingProductionOptions).
	BoostRatePerGood map[string]float64
}

// weakestLinkRefiningBuilding maps a good to the ONE building type whose
// terrain+building combined production_rules row is P6's weakest-link boost
// tier — keyed by BUILDING, not just good, because wine also carries a
// farm-boost row (mig 008/103: tilled land also grows grapes, unrelated to
// §10.2's press/winery pair) that must keep flowing straight into
// RatePerGood, ungated — only the row naming THIS specific building routes
// into BoostRatePerGood. Every other good's building-boosted rows
// (grain+farm, timber/cedar+lumbermill, stone+mine/stonequarry, copper/tin+
// mine, silver+silver_mine, fish+harbour, livestock+pasture) are unaffected.
var weakestLinkRefiningBuilding = map[string]string{
	GoodOil:  "olive_press",
	GoodWine: "winery",
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
//
// reachable implements belägring S1 (megaron_plan_belagring.md
// §Implementeringskontrakt step 4): when non-nil, a ring hex is included ONLY
// if reachable[hex] is true — a denied hex is dropped before the catchment
// SQL runs at all, so it contributes zero to every placement's yield
// (RecomputeProduction's placements.Hex lookup simply finds no HexOption for
// that coord). Pass nil for the ordinary unfiltered catchment (every caller
// except RecomputeProduction — a settlement isn't besieged while a Wanax is
// merely previewing where to place a founding gubbe). The SAME parameter
// doubles as a FOW gate for the colonize/settle forecast (api/handlers/world.go
// ColonizePreview), which has no settlement yet to be besieged: pass a set of
// the hexes the requesting Wanax actually knows.
func LoadHexProductionOptions(ctx context.Context, tx Tx, settlementID uuid.UUID, reachable map[hexgrid.Coord]bool) ([]HexOption, error) {
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

	return LoadHexProductionOptionsAt(ctx, tx, worldID, hexgrid.Coord{Q: q, R: r}, buildingLevels, reachable)
}

// LoadHexProductionOptionsAt is LoadHexProductionOptions' settlement-free
// core: it needs (worldID, centre hex, building levels) and nothing else —
// LoadHexProductionOptions is a thin wrapper that looks those three up from a
// settlementID and calls straight through
// (megaron_plan_grundningsprognosen.md §3: "en formel, två anrop"). This is
// what lets the colonize/settle forecast (FoundingGrainNetPerTick) run the
// EXACT SAME catchment math a real founding does, before any settlement row
// exists to hang a settlementID off of.
//
// buildingLevels is an ASSUMED set here, not a live lookup — for a real
// settlement it is whatever loadBuildingLevels found; for a forecast it is
// the hypothetical building the founding would seed (e.g. {"farm": 1} for a
// metropolis, empty for a colony, which builds its own farm later). A
// building's mere PRESENCE as a map key is enough to satisfy the gate below,
// mirroring the settlement path's `EXISTS (SELECT 1 FROM buildings ...)`
// check exactly (that check never looked at level either).
func LoadHexProductionOptionsAt(ctx context.Context, tx Tx, worldID uuid.UUID, center hexgrid.Coord, buildingLevels map[string]int, reachable map[hexgrid.Coord]bool) ([]HexOption, error) {
	ring := hexgrid.Ring(center, hexgrid.CatchmentRadius)
	if reachable != nil {
		filtered := ring[:0:0]
		for _, c := range ring {
			if reachable[c] {
				filtered = append(filtered, c)
			}
		}
		ring = filtered
	}
	catchQ, catchR := hexgrid.QRArrays(ring)

	builtTypes := make([]string, 0, len(buildingLevels))
	for bt := range buildingLevels {
		builtTypes = append(builtTypes, bt)
	}

	rows, err := tx.Query(ctx,
		`SELECT mt.q, mt.r, mt.terrain,
		        COALESCE(mt.copper_deposit, false), COALESCE(mt.tin_deposit, false), COALESCE(mt.silver_deposit, false),
		        pr.good_key, pr.rate_per_tick, pr.building_type
		 FROM unnest($2::int[], $3::int[]) AS catchment(q, r)
		 JOIN map_tiles mt ON mt.world_id = $1 AND mt.q = catchment.q AND mt.r = catchment.r
		 JOIN production_rules pr ON
		     (pr.terrain_type IS NULL OR pr.terrain_type = mt.terrain)
		     -- A row with terrain_type IS NULL AND building_type IS NOT NULL is
		     -- a pure building workplace (P6's refining-capacity rows —
		     -- olive_press/winery/foundry) — LoadBuildingProductionOptions'
		     -- exclusive territory, never a hex's own production. Without this
		     -- exclusion such a row matches EVERY hex in the catchment (NULL
		     -- terrain = "any terrain") and double-counts against
		     -- BoostRatePerGood on top of the genuine per-hex boost row.
		     -- terrain_type IS NULL AND building_type IS NULL (the timber
		     -- anti-deadlock trickle, mig 033) is unaffected — it still matches
		     -- every hex, same as before P6.
		     AND NOT (pr.terrain_type IS NULL AND pr.building_type IS NOT NULL)
		     AND (NOT pr.requires_coastal OR mt.coastal)
		     AND (pr.building_type IS NULL OR pr.building_type = ANY($4::text[]))
		     AND (pr.requires_deposit IS NULL
		          OR (pr.requires_deposit = 'copper' AND mt.copper_deposit)
		          OR (pr.requires_deposit = 'tin'    AND mt.tin_deposit)
		          OR (pr.requires_deposit = 'silver' AND COALESCE(mt.silver_deposit, false))
		          OR (pr.requires_deposit = 'cedar'  AND COALESCE(mt.cedar_deposit, false)))
		 JOIN goods g ON g.key = pr.good_key AND g.status = 'active'
		 WHERE mt.terrain NOT IN ('deep_sea','coastal_sea','river','river_ford')
		        OR pr.terrain_type = mt.terrain`,
		worldID, catchQ, catchR, builtTypes,
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
		var buildingType *string
		if err := rows.Scan(&qq, &rr, &terrain, &copperDep, &tinDep, &silverDep, &goodKey, &rate, &buildingType); err != nil {
			return nil, fmt.Errorf("load hex production options: scan: %w", err)
		}
		c := hexgrid.Coord{Q: qq, R: rr}
		opt, ok := byCoord[c]
		if !ok {
			caps, capsL1, mult, placeCap := hexGoodCaps(terrain, copperDep, tinDep, silverDep, buildingLevels)
			opt = &HexOption{
				Coord:            c,
				Terrain:          terrain,
				RatePerGood:      make(map[string]float64),
				BoostRatePerGood: make(map[string]float64),
				CapPerGood:       caps,
				CapL1PerGood:     capsL1,
				MultPerGood:      mult,
				PlaceCapPerGood:  placeCap,
			}
			byCoord[c] = opt
			order = append(order, c)
		}
		if refiningBuilding, ok := weakestLinkRefiningBuilding[goodKey]; ok && buildingType != nil && *buildingType == refiningBuilding {
			opt.BoostRatePerGood[goodKey] += rate
		} else {
			opt.RatePerGood[goodKey] += rate
		}
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
			if _, covered := opt.CapL1PerGood[good]; !covered {
				// The fallback cap has no level term at all (no hexCapacityRule
				// entry exists for these goods), so its L1 sibling is the same
				// flat constant — no scaling to freeze, nothing changes here.
				opt.CapL1PerGood[good] = HexFallbackCap
			}
			if _, covered := opt.MultPerGood[good]; !covered {
				// cap == capL1 above, so the multiplier is trivially 1.0.
				opt.MultPerGood[good] = 1.0
			}
			if _, covered := opt.PlaceCapPerGood[good]; !covered {
				// None of oil/wine/stone is grain, so the clip ceiling is the
				// same flat constant too — no grain exception applies here.
				opt.PlaceCapPerGood[good] = HexFallbackCap
			}
		}
		for good := range opt.BoostRatePerGood {
			if _, covered := opt.CapPerGood[good]; !covered {
				opt.CapPerGood[good] = HexFallbackCap
			}
			if _, covered := opt.CapL1PerGood[good]; !covered {
				opt.CapL1PerGood[good] = HexFallbackCap
			}
			if _, covered := opt.MultPerGood[good]; !covered {
				opt.MultPerGood[good] = 1.0
			}
			if _, covered := opt.PlaceCapPerGood[good]; !covered {
				opt.PlaceCapPerGood[good] = HexFallbackCap
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
// many gubbar are placed there. Every good is capacity-clamped (placed is
// never allowed above placeCap — a physically full hex/building produces no
// more no matter how many more gubbar queue up).
//
// Form B (megaron_plan_byggnadsniva_takt.md, Timothy 2026-08-24) moved the
// building level's effect from the CEILING to the RATE for every good except
// grain. Form A (2026-08-22) divided by capL1 but still let placed climb to
// the actual (level-grown) cap — so a level raised how many gubbar fit, at
// the same per-gubbe rate. Form B instead freezes headcount at capL1 on
// EVERY level and multiplies the per-gubbe rate by mult = cap(actualLevel)/
// capL1 (hexGoodCaps) — placeCap==capL1 for these goods. Both forms share
// the same max output (rate/capL1 × cap), proven by:
//
//	Form A max  = (rate/capL1) × cap
//	Form B max  = (rate/capL1) × mult × capL1 = (rate/capL1) × (cap/capL1) × capL1 = (rate/capL1) × cap
//
// — identical. What changes is how many gubbar reach that max: Form A needed
// `cap` (grows with level); Form B needs only `capL1` (fixed) at every
// level, because the SAME gubbar now carry more each. This is the whole
// point: an upgrade lowers the population cost of the same ceiling instead
// of raising it.
//
// Grain is the deliberate exception to the paragraph above: hexGoodCaps pins
// mult=1 for grain unconditionally, AND passes placeCap=cap (the REAL,
// level-actual physical cap from megaron_plan_grain_cap.md), not capL1 (a
// pure division trick, pinned to 1, that was never a real headcount limit).
// So grain's formula reduces to exactly its pre-Form-B shape: rate × placed,
// clamped at the real per-hex cap that still grows 4/8/10/12 with farm
// level — untouched by this slice, on purpose (§4 of the plan: a naive mult
// applied to grain would multiply the whole world's food supply 4-12×).
func placementYield(good string, rate float64, capL1 int, placeCap int, mult float64, placed int) float64 {
	if capL1 <= 0 || placeCap <= 0 {
		return 0
	}
	if placed > placeCap {
		placed = placeCap // defensive — Place() enforces the cap at write time, never trust a stale read
	}
	return (rate / float64(capL1)) * mult * float64(placed)
}

// hexGoodCaps returns every good_key a single hex (given its terrain and
// deposit flags) can cap, and that good's worker cap — the per-hex sibling of
// LoadHexCapacity's aggregate sum (P3) — TWICE: once at the building's ACTUAL
// level (caps, unchanged since P3) and once frozen at level 1 (capL1). A hex
// can independently cap MULTIPLE goods (a plains hex is both "slätt" (grain)
// and "betesmark" (livestock)).
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
//
// Grain is the one goodKey whose capL1 is forced to 1 regardless of the rule
// above (see placementYield's doc comment) — its rate was never calibrated
// per-cap, so it must keep the old rate×placed shape.
//
// mult is Form B's third map (megaron_plan_byggnadsniva_takt.md §5 step 1):
// good → cap/capL1, i.e. how much bigger the ACTUAL cap is than the frozen
// level-1 one. Deliberately computed AFTER the apply loop, from the SUMMED
// caps/capsL1 per good — NOT inside apply from each individual rule's own
// (cap, capL1) pair. A hex can carry more than one rule for the SAME good
// (mager åker + a copper deposit both feed "grain" is not a real case today,
// but plains' two rules already prove a hex can accumulate one good's caps
// across multiple apply() calls in principle); taking the ratio of the
// PER-RULE pair and overwriting mult[good] on a second apply would silently
// drop the first rule's contribution. Ratio-of-sums is exact for the actual
// game data (today, no good is fed by two building-gated rules on the same
// hex) and never needs a weighted average to stay correct if that changes.
//
// placeCap is the fourth map: the number of gubbar that may actually be
// PLACED (placementYield's clip ceiling, and Place()'s write-time ceiling).
// For every good except grain this equals capL1 — Form B's whole point,
// headcount frozen at level 1. Grain keeps placeCap=cap (the real,
// level-actual cap) because its capL1 is a pure division trick (pinned to 1
// two lines above), never a real headcount limit — see PlaceCapPerGood's and
// placementYield's doc comments for why conflating the two would silently
// cut every grain hex down to one placeable gubbe.
func hexGoodCaps(terrain string, copperDep, tinDep, silverDep bool, buildingLevels map[string]int) (caps map[string]int, capsL1 map[string]int, mult map[string]float64, placeCap map[string]int) {
	capOf := func(rule hexCapacityRule) (cap, capL1 int) {
		if rule.relevantBuilding == "" {
			return rule.capNoBuilding, rule.capNoBuilding
		}
		level := buildingLevels[rule.relevantBuilding]
		if level <= 0 {
			return rule.capNoBuilding, rule.capNoBuilding
		}
		cap = rule.capWithBuilding + WorkplaceSlots(rule.relevantBuilding, level)
		capL1 = rule.capWithBuilding + WorkplaceSlots(rule.relevantBuilding, 1)
		return cap, capL1
	}
	apply := func(rule hexCapacityRule) {
		cap, capL1 := capOf(rule)
		if rule.goodKey == GoodGrain {
			capL1 = 1
		}
		caps[rule.goodKey] += cap
		capsL1[rule.goodKey] += capL1
	}
	caps = make(map[string]int)
	capsL1 = make(map[string]int)
	if terrain == "plains" {
		for _, rule := range plainsCapacityRules {
			apply(rule)
		}
	} else if rule, ok := terrainCapacityTable[terrain]; ok {
		apply(rule)
	}
	if copperDep {
		apply(depositCapacityTable["copper"])
	}
	if tinDep {
		apply(depositCapacityTable["tin"])
	}
	if silverDep {
		apply(depositCapacityTable["silver"])
	}

	mult = make(map[string]float64)
	placeCap = make(map[string]int)
	for good, cap := range caps {
		if capL1 := capsL1[good]; capL1 > 0 {
			mult[good] = float64(cap) / float64(capL1)
			placeCap[good] = capL1
		} else {
			mult[good] = 1.0
			placeCap[good] = cap
		}
	}
	// Pinned last, unconditionally, in the SAME function that already forces
	// grain's capL1 to 1 — so the three can never drift apart (§4 of the
	// plan: a naive mult would multiply the whole world's food supply 4-12×,
	// and a naive placeCap=capL1 would cut every grain hex to ONE gubbe).
	mult[GoodGrain] = 1.0
	placeCap[GoodGrain] = caps[GoodGrain] // the real, level-actual grain cap — untouched by Form B
	return caps, capsL1, mult, placeCap
}

// BuildingOption is one settlement-wide workplace building's production menu
// — mirrors HexOption for target_kind='building' placements (P0-UI's
// "STADENS ARBETSPLATSER"). Only production_rules rows with NO terrain gate
// (pr.terrain_type IS NULL) qualify: a rule that also needs a terrain (e.g.
// today's farm+plains→grain) is catchment-hex production with a building
// REQUIREMENT, not a pure building workplace — that stays a HexOption. Since
// P6 (megaron_plan_fysisk_gubbemodell.md §P6, 2026-08-08) this is where a
// pressarbetare/vinmakare/gjutare's refining capacity lives — olive_press,
// winery and foundry each carry one terrain-free row; RecomputeProduction
// combines it with the matching HexOption via the weakest-link formula (oil/
// wine: min(boost potential, refining capacity)) or a stock drain (bronze —
// see the bronze stock-drain step). market/stable's goods are still parked.
type BuildingOption struct {
	BuildingType string
	Level        int
	RatePerGood  map[string]float64
	CapPerGood   map[string]int

	// CapL1PerGood is CapPerGood's Form A sibling — WorkplaceSlots(BuildingType, 1),
	// frozen regardless of Level. See HexOption.CapL1PerGood's doc comment;
	// this is the same idea for the pure-building workplaces (mine/stonequarry/
	// olive_press/winery/foundry/…).
	CapL1PerGood map[string]int

	// MultPerGood is HexOption.MultPerGood's sibling for pure-building
	// workplaces: CapPerGood/CapL1PerGood, i.e. WorkplaceSlots(BuildingType,
	// Level)/WorkplaceSlots(BuildingType, 1). No good reaching this map is
	// grain (grain is always HexOption/farm-gated), so there is no pinned
	// exception to carry here.
	MultPerGood map[string]float64

	// PlaceCapPerGood is HexOption.PlaceCapPerGood's sibling — always equal
	// to CapL1PerGood here, since grain never reaches a BuildingOption (it is
	// always terrain-gated, HexOption's territory), so the grain exception
	// that field carries never applies.
	PlaceCapPerGood map[string]int
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
				BuildingType:    buildingType,
				Level:           level,
				RatePerGood:     make(map[string]float64),
				CapPerGood:      make(map[string]int),
				CapL1PerGood:    make(map[string]int),
				MultPerGood:     make(map[string]float64),
				PlaceCapPerGood: make(map[string]int),
			}
			byType[buildingType] = opt
			order = append(order, buildingType)
		}
		opt.RatePerGood[goodKey] += rate
		opt.CapPerGood[goodKey] = WorkplaceSlots(buildingType, level)
		capL1 := WorkplaceSlots(buildingType, 1)
		opt.CapL1PerGood[goodKey] = capL1
		opt.PlaceCapPerGood[goodKey] = capL1
		if capL1 > 0 {
			opt.MultPerGood[goodKey] = float64(opt.CapPerGood[goodKey]) / float64(capL1)
		} else {
			opt.MultPerGood[goodKey] = 1.0
		}
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

// GlobalHexOccupancy is LoadPlacementCounts' cross-settlement counterpart for
// hex targets ONLY (megaron_plan_hexagarskap_och_stadsavstand.md §2: "en hex
// ska bära ett bestämt antal gubbar TOTALT, oavsett hur många städer som har
// den i sin catchment"). It answers "how many gubbar, from EVERY settlement
// in this world, already stand on hex H doing good G" — the number a hex's
// PlaceCapPerGood must be checked against once two settlements' catchments
// can overlap (§3, not yet built — CatchmentClearanceHexes still forbids it
// today, so for any hex that belongs to only one settlement's catchment this
// returns EXACTLY what LoadPlacementCounts would have for that settlement
// alone; the two are indistinguishable until overlap is possible).
//
// Deliberately NOT used by RecomputeProduction/placementYield: those stay on
// LoadPlacementCounts (settlement-scoped) because placementYield's formula is
// already correct per-gubbe — rate/capL1×mult is a FIXED contribution per
// worker, so as long as the global occupancy check below (PlaceGubbe) never
// lets a hex's total exceed PlaceCapPerGood, two settlements sharing a hex
// simply split that hex's one ceiling between their own placed headcounts;
// summing each settlement's own RecomputeProduction output already adds up to
// no more than a single settlement fully staffing it would have produced. The
// capacity CHECK is the only thing that needs to become global — the
// production FORMULA does not (this is the plan's step 2: "det är den enda
// verkliga kodändringen — resten är följd").
func GlobalHexOccupancy(ctx context.Context, tx Tx, worldID uuid.UUID, hexes []hexgrid.Coord) (map[hexgrid.Coord]map[string]int, error) {
	out := make(map[hexgrid.Coord]map[string]int)
	if len(hexes) == 0 {
		return out, nil
	}
	q, r := hexgrid.QRArrays(hexes)
	rows, err := tx.Query(ctx,
		`SELECT sp.hex_q, sp.hex_r, sp.good_key, COUNT(*)
		 FROM settlement_placement sp
		 JOIN settlements s ON s.id = sp.settlement_id
		 JOIN unnest($2::int[], $3::int[]) AS wanted(q, r) ON wanted.q = sp.hex_q AND wanted.r = sp.hex_r
		 WHERE s.world_id = $1 AND sp.target_kind = 'hex'
		 GROUP BY sp.hex_q, sp.hex_r, sp.good_key`,
		worldID, q, r,
	)
	if err != nil {
		return nil, fmt.Errorf("global hex occupancy: query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var qq, rr, n int
		var good string
		if err := rows.Scan(&qq, &rr, &good, &n); err != nil {
			return nil, fmt.Errorf("global hex occupancy: scan: %w", err)
		}
		c := hexgrid.Coord{Q: qq, R: rr}
		if out[c] == nil {
			out[c] = make(map[string]int)
		}
		out[c][good] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("global hex occupancy: rows: %w", err)
	}
	return out, nil
}

// MarginalYieldForSlot is the ONE per-slot marginal-yield formula — grain
// keeps placementYield's rate × placed shape (no capacity division, see
// placementYield's doc comment); every other good divides by capL1 and
// multiplies by the building-level mult (megaron_plan_byggnadsniva_takt.md).
// Shared by PlacementOptions' buildGoods (api/handlers/settlement_placement.go,
// itemised per hex/building — a client needs a DIFFERENT number per hex to
// choose where to place) and MarginalYieldPerGood below (aggregated per
// good, whole-catchment) — one formula, two shapes, never a second formula
// (megaron_plan_p4_arvet_i_province.md §3 step C: the same class of bug as
// the /goods-lögnen and the grundningsprognosen's "en formel, två anrop").
func MarginalYieldForSlot(good string, rate float64, capL1 int, mult float64) float64 {
	if good == GoodGrain {
		return rate
	}
	return (rate / float64(capL1)) * mult
}

// MarginalYieldPerGood returns, for each good, the yield the NEXT gubbe
// placed on it would produce — the best (highest) MarginalYieldForSlot
// among every hex/building option for that good that still has room
// (occupied < PlaceCapPerGood). A good with no available slot anywhere
// (every hex/building for it already full, or the catchment cannot produce
// it at all) is simply absent from the map: there IS no next gubbe to
// place, and a stale non-zero number would claim otherwise — exactly what
// the old province.go bp/REF_LABOR did (it never went to zero at capacity).
// This is the shared computation the plan's finding calls for — the old
// per-good aggregate figure was a worse duplicate of PlacementOptions'
// marginal_yield: /goods calls this directly for its one number per good;
// PlacementOptions keeps its own per-slot MarginalYieldForSlot calls for
// the itemised grid (see buildGoods) — both ultimately the same formula.
func MarginalYieldPerGood(hexOptions []HexOption, buildingOptions []BuildingOption, placed PlacementCounts) map[string]float64 {
	best := make(map[string]float64)
	consider := func(good string, rate float64, capL1, placeCap int, mult float64, occupied int) {
		if capL1 <= 0 || placeCap <= 0 || occupied >= placeCap {
			return
		}
		yield := MarginalYieldForSlot(good, rate, capL1, mult)
		if cur, ok := best[good]; !ok || yield > cur {
			best[good] = yield
		}
	}
	for _, opt := range hexOptions {
		occ := placed.Hex[opt.Coord]
		for good, rate := range opt.RatePerGood {
			consider(good, rate, opt.CapL1PerGood[good], opt.PlaceCapPerGood[good], opt.MultPerGood[good], occ[good])
		}
	}
	for _, opt := range buildingOptions {
		occ := placed.Building[opt.BuildingType]
		for good, rate := range opt.RatePerGood {
			consider(good, rate, opt.CapL1PerGood[good], opt.PlaceCapPerGood[good], opt.MultPerGood[good], occ[good])
		}
	}
	return best
}
