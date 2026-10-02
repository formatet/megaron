package economy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// HexOption is one catchment ring hex's production menu — every good it can
// support, its OWN base rate per worker and its OWN worker places
// (byggnadsregeln, hex_rules.go). P4 (megaron_plan_fysisk_gubbemodell.md):
// a gubbe stands on ONE hex doing ONE good — "en skogshuggare gör bara
// virke/ceder (beroende på hex)" (Timothy 2026-08-08) — so production must be
// derivable per hex, not just as a catchment-wide sum.
type HexOption struct {
	Coord   hexgrid.Coord
	Terrain string

	// RatePerGood is r0, the BASE rate per worker (production_rules
	// terrain/deposit rows; silver's mine rows only when a mine stands here).
	RatePerGood map[string]float64

	// MultPerGood is the building multiplier 1 + BuildingRatePerLevel*level when
	// the rule's relevant building stands here, else 1.0. A worker's output is
	// RatePerGood * MultPerGood (hexYield).
	MultPerGood map[string]float64

	// PlaceCapPerGood is the number of worker places: P0, or P0 + BuildingExtraPlaces
	// with the relevant building. What hexYield clips against and what Place()
	// enforces at write time.
	PlaceCapPerGood map[string]int

	// CapPerGood is PlaceCapPerGood again — the field the API reads to show a cap.
	CapPerGood map[string]int

	// BoostRatePerGood holds the SAME extraction gubbe's terrain+building
	// combined rows for weakestLinkRefiningBuilding's goods (oil, wine — P6,
	// megaron_plan_fysisk_gubbemodell.md §P6), per worker — e.g. forest_olive_grove +
	// olive_press. It is "potential" only: RecomputeProduction realizes it as
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
// (grain+farm, timber/cedar+lumbermill, stone+mine/stonequarry, copper/tin/
// silver+mine, fish+harbour, livestock+pasture) are unaffected.
var weakestLinkRefiningBuilding = map[string]string{
	GoodOil:  "olive_press",
	GoodWine: "winery",
}

// BuildingSet carries a settlement's (or a founding forecast's hypothetical)
// building levels, split by scope (megaron_plan_byggnad_pa_hex.md §A). City
// holds settlement-wide buildings (harbour, market, …) — presence anywhere
// gates every catchment hex, exactly as before this slice. Hex holds
// hex-bound production buildings (farm/mine/lumbermill/stonequarry — see
// HexBoundBuildingTypes, recompute.go), keyed by the EXACT hex they stand
// on — presence gates ONLY that hex, which is the whole point of this slice
// ("en farm lyfte hela catchmenten" is the bug this closes).
type BuildingSet struct {
	City map[string]int
	Hex  map[hexgrid.Coord]map[string]int
}

// AllTypes returns every building type present anywhere (City ∪ every Hex
// entry) — LoadHexProductionOptionsAt's coarse SQL pre-filter (which rows to
// even fetch); the precise per-hex/per-city gate is then applied in Go
// (megaron_plan_byggnad_pa_hex.md §A: "filtrera raderna i Go efter queryn är
// enklast").
func (bs BuildingSet) AllTypes() []string {
	seen := make(map[string]bool)
	for t := range bs.City {
		seen[t] = true
	}
	for _, hx := range bs.Hex {
		for t := range hx {
			seen[t] = true
		}
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	return out
}

// levelsAt returns the flat building-level map hexGoodCaps already expects,
// scoped to hex c: City merged with Hex[c]. A hex-bound type only ever
// appears under Hex, a city-wide type only ever under City, so there is no
// real collision — this is a plain union.
func (bs BuildingSet) levelsAt(c hexgrid.Coord) map[string]int {
	out := make(map[string]int, len(bs.City)+len(bs.Hex[c]))
	for k, v := range bs.City {
		out[k] = v
	}
	for k, v := range bs.Hex[c] {
		out[k] = v
	}
	return out
}

// builtAt reports whether buildingType counts as built FOR HEX c: Hex[c]'s
// own presence for a hex-bound type, City's settlement-wide presence for
// everything else (harbour's coastal_sea+fish row is the one HexOption case
// that reaches this — a terrain-gated row naming a city-wide building).
func (bs BuildingSet) builtAt(c hexgrid.Coord, buildingType string) bool {
	if HexBoundBuildingTypes[buildingType] {
		return bs.Hex[c][buildingType] > 0
	}
	return bs.City[buildingType] > 0
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

	bs, err := loadBuildingSet(ctx, tx, settlementID)
	if err != nil {
		return nil, fmt.Errorf("load hex production options: %w", err)
	}

	return LoadHexProductionOptionsAt(ctx, tx, worldID, hexgrid.Coord{Q: q, R: r}, bs, reachable)
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
// bs is an ASSUMED set here, not a live lookup — for a real settlement it is
// whatever loadBuildingSet found; for a forecast it is the hypothetical
// building(s) the founding would seed (e.g. a farm on one specific hex for a
// metropolis, empty for a colony, which builds its own farm later). A
// building's mere PRESENCE (City or Hex[c]) is enough to satisfy the gate
// below, mirroring the settlement path's `EXISTS (SELECT 1 FROM buildings
// ...)` check exactly (that check never looked at level either) — now scoped
// per hex for hex-bound types instead of settlement-wide.
func LoadHexProductionOptionsAt(ctx context.Context, tx Tx, worldID uuid.UUID, center hexgrid.Coord, bs BuildingSet, reachable map[hexgrid.Coord]bool) ([]HexOption, error) {
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
	return loadHexOptionsForHexes(ctx, tx, worldID, ring, bs)
}

// loadHexOptionsForHexes is LoadHexProductionOptionsAt's core over an EXPLICIT
// hex list (already FOW/siege filtered by the caller) — also what the colonize
// preview's CatchmentBasePotentialAt runs on, so the preview and a real
// settlement share one rule.
func loadHexOptionsForHexes(ctx context.Context, tx Tx, worldID uuid.UUID, ring []hexgrid.Coord, bs BuildingSet) ([]HexOption, error) {
	catchQ, catchR := hexgrid.QRArrays(ring)

	builtTypes := bs.AllTypes()

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

	type hexFlags struct{ copper, tin, silver bool }
	flags := make(map[hexgrid.Coord]hexFlags)
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
			opt = &HexOption{
				Coord:            c,
				Terrain:          terrain,
				RatePerGood:      make(map[string]float64),
				BoostRatePerGood: make(map[string]float64),
				CapPerGood:       make(map[string]int),
				MultPerGood:      make(map[string]float64),
				PlaceCapPerGood:  make(map[string]int),
			}
			byCoord[c] = opt
			flags[c] = hexFlags{copperDep, tinDep, silverDep}
			order = append(order, c)
		}
		// A row naming a building only counts on THIS hex if that building is
		// actually built here (builtAt — hex-scoped for farm/mine/lumbermill/
		// stonequarry, settlement-wide for everything else). After mig 155 that
		// is silver's mine rows and the olive press / winery boost rows.
		if buildingType != nil && !bs.builtAt(c, *buildingType) {
			continue
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
		fl := flags[c]
		levels := bs.levelsAt(c)
		fill := func(good string) {
			if _, done := opt.PlaceCapPerGood[good]; done {
				return
			}
			places, mult := hexGoodPlaces(opt.Terrain, fl.copper, fl.tin, fl.silver, levels, good)
			opt.PlaceCapPerGood[good] = places
			opt.CapPerGood[good] = places
			opt.MultPerGood[good] = mult
		}
		for good := range opt.RatePerGood {
			fill(good)
		}
		for good := range opt.BoostRatePerGood {
			fill(good)
		}
		out = append(out, *opt)
	}
	return out, nil
}

// placementYield is the rate a BUILDING workplace (olive press, winery,
// foundry, …) contributes for one good, given how many gubbar are placed
// there: rate per capL1 slot, times the building-level multiplier (Form B,
// megaron_plan_byggnadsniva_takt.md), clamped at placeCap. Hex production does
// not use it — see hexYield (hex_rules.go).
func placementYield(good string, rate float64, capL1 int, placeCap int, mult float64, placed int) float64 {
	if capL1 <= 0 || placeCap <= 0 {
		return 0
	}
	if placed > placeCap {
		placed = placeCap // defensive — Place() enforces the cap at write time, never trust a stale read
	}
	return (rate / float64(capL1)) * mult * float64(placed)
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
// building's terrain-free production menu, POOLED across every building of
// that type — city buildings are one-per-settlement (unaffected), but a
// hex-bound type's PRODUCTION can still route through here when its
// production_rules row is terrain-free (mine+stone and stonequarry+stone
// both are: only mine's copper/tin/silver rows are terrain+deposit-gated
// HexOption rows). megaron_plan_byggnad_pa_hex.md §A made several buildings
// of the same type possible (one per hex); this function's old single-row
// assumption (`byType[buildingType]` overwriting Level/CapPerGood on a
// second row instead of summing) silently corrupted a settlement with two
// mines or two stonequarries — RatePerGood double-counted while CapPerGood
// only reflected the LAST row's level. Fixed by summing rate and slots per
// building ROW, the same way LoadWorkplaceSlots (recompute.go) already sums
// across hex-bound buildings.
//
// Level is the MAX level across every building of this type — informational
// display only (same convention db.go's loadLaborCapacities/province.go's
// Goods handler already use for their own per-good level sums); with two
// stonequarries at different levels there is no single "the" level, and the
// pooled capacity numbers below (not this field) are what actually gates
// placement.
func LoadBuildingProductionOptions(ctx context.Context, tx Tx, settlementID uuid.UUID) ([]BuildingOption, error) {
	rows, err := tx.Query(ctx,
		`SELECT b.id, b.building_type, b.level, pr.good_key, pr.rate_per_tick
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
	ensure := func(buildingType string) *BuildingOption {
		opt, ok := byType[buildingType]
		if !ok {
			opt = &BuildingOption{
				BuildingType:    buildingType,
				RatePerGood:     make(map[string]float64),
				CapPerGood:      make(map[string]int),
				CapL1PerGood:    make(map[string]int),
				MultPerGood:     make(map[string]float64),
				PlaceCapPerGood: make(map[string]int),
			}
			byType[buildingType] = opt
			order = append(order, buildingType)
		}
		return opt
	}
	for rows.Next() {
		var buildingID uuid.UUID
		var buildingType, goodKey string
		var level int
		var rate float64
		if err := rows.Scan(&buildingID, &buildingType, &level, &goodKey, &rate); err != nil {
			return nil, fmt.Errorf("load building production options: scan: %w", err)
		}
		opt := ensure(buildingType)
		if level > opt.Level {
			opt.Level = level
		}
		opt.RatePerGood[goodKey] += rate
		// Sum PER BUILDING ROW, not per good — each row here is one distinct
		// building instance contributing its own WorkplaceSlots(level) and
		// WorkplaceSlots(1) to this good's pool, mirroring LoadWorkplaceSlots'
		// per-row summation.
		opt.CapPerGood[goodKey] += WorkplaceSlots(buildingType, level)
		opt.CapL1PerGood[goodKey] += WorkplaceSlots(buildingType, 1)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load building production options: rows: %w", err)
	}

	out := make([]BuildingOption, 0, len(order))
	for _, bt := range order {
		opt := byType[bt]
		for good, capL1 := range opt.CapL1PerGood {
			opt.PlaceCapPerGood[good] = capL1
			if capL1 > 0 {
				opt.MultPerGood[good] = float64(opt.CapPerGood[good]) / float64(capL1)
			} else {
				opt.MultPerGood[good] = 1.0
			}
		}
		out = append(out, *opt)
	}
	return out, nil
}

// loadBuildingSet returns every building the settlement has built, split
// into BuildingSet's City/Hex scopes by its hex_q/hex_r columns (NULL = a
// city building) — hexGoodCaps needs the LEVEL (not just presence) to add
// the building's own WorkplaceSlots on top of the hex tier.
func loadBuildingSet(ctx context.Context, tx Tx, settlementID uuid.UUID) (BuildingSet, error) {
	rows, err := tx.Query(ctx, `SELECT building_type, level, hex_q, hex_r FROM buildings WHERE settlement_id = $1`, settlementID)
	if err != nil {
		return BuildingSet{}, fmt.Errorf("load building set: %w", err)
	}
	defer rows.Close()
	bs := BuildingSet{City: make(map[string]int), Hex: make(map[hexgrid.Coord]map[string]int)}
	for rows.Next() {
		var bt string
		var level int
		var hq, hr *int
		if err := rows.Scan(&bt, &level, &hq, &hr); err != nil {
			return BuildingSet{}, fmt.Errorf("load building set: scan: %w", err)
		}
		if hq != nil && hr != nil {
			c := hexgrid.Coord{Q: *hq, R: *hr}
			if bs.Hex[c] == nil {
				bs.Hex[c] = make(map[string]int)
			}
			bs.Hex[c][bt] = level
		} else {
			bs.City[bt] = level
		}
	}
	if err := rows.Err(); err != nil {
		return BuildingSet{}, fmt.Errorf("load building set: rows: %w", err)
	}
	return bs, nil
}

// ChooseFarmHex picks the ring hex a founding's free starter farm goes on —
// the grain hex whose level-1 farm gives the largest grain OUTPUT per full crew,
// (P0 + BuildingExtraPlaces) * r0 * (1 + BuildingRatePerLevel), ties broken by
// lowest Q then lowest R (byggnadsregeln fynd 2: ranking on places alone would
// prefer plains (8 places) over delta (7 places at 2.7x the rate)). Both the real
// founding (create_metropolis.go) and its forecast (FoundingGrainNetPerTick)
// call this SAME function so they pick the SAME hex. reachable is the same
// FOW/siege gate LoadHexProductionOptionsAt takes (nil = unfiltered). ok=false
// means no catchment ring hex can grow grain at all.
func ChooseFarmHex(ctx context.Context, tx Tx, worldID uuid.UUID, center hexgrid.Coord, reachable map[hexgrid.Coord]bool) (best hexgrid.Coord, ok bool, err error) {
	ring := hexgrid.Ring(center, hexgrid.CatchmentRadius)
	catchQ, catchR := hexgrid.QRArrays(ring)
	rows, err := tx.Query(ctx,
		`SELECT mt.q, mt.r, mt.terrain, pr.rate_per_tick
		 FROM unnest($2::int[], $3::int[]) AS catchment(q, r)
		 JOIN map_tiles mt ON mt.world_id = $1 AND mt.q = catchment.q AND mt.r = catchment.r
		 JOIN production_rules pr ON pr.good_key = 'grain' AND pr.building_type IS NULL
		      AND pr.terrain_type = mt.terrain`,
		worldID, catchQ, catchR,
	)
	if err != nil {
		return hexgrid.Coord{}, false, fmt.Errorf("choose farm hex: query: %w", err)
	}
	defer rows.Close()

	type cand struct {
		c       hexgrid.Coord
		terrain string
		r0      float64
	}
	var cands []cand
	for rows.Next() {
		var qq, rr int
		var terrain string
		var r0 float64
		if err := rows.Scan(&qq, &rr, &terrain, &r0); err != nil {
			return hexgrid.Coord{}, false, fmt.Errorf("choose farm hex: scan: %w", err)
		}
		c := hexgrid.Coord{Q: qq, R: rr}
		if reachable != nil && !reachable[c] {
			continue
		}
		cands = append(cands, cand{c, terrain, r0})
	}
	if err := rows.Err(); err != nil {
		return hexgrid.Coord{}, false, fmt.Errorf("choose farm hex: rows: %w", err)
	}
	rows.Close()

	// One building per hex: a neighbour's building on a shared catchment hex rules it out.
	occupied, err := HexOccupants(ctx, tx, worldID, ring)
	if err != nil {
		return hexgrid.Coord{}, false, fmt.Errorf("choose farm hex: %w", err)
	}
	bestYield := -1.0
	for _, cd := range cands {
		c, terrain, r0 := cd.c, cd.terrain, cd.r0
		if _, taken := occupied[c]; taken {
			continue
		}
		var places int
		var mult float64
		found := false
		for _, rule := range hexRules(terrain, false, false, false) {
			if rule.good == GoodGrain && rule.building == "farm" {
				places, mult = rule.placesAndMult(1)
				found = true
			}
		}
		if !found {
			continue
		}
		y := float64(places) * r0 * mult
		if !ok || y > bestYield || (y == bestYield && (c.Q < best.Q || (c.Q == best.Q && c.R < best.R))) {
			ok, bestYield, best = true, y, c
		}
	}
	return best, ok, nil
}

// HexSupportsBuilding reports whether hex could host a production building of
// buildingType: the rule table (hex_rules.go) names buildingType as the relevant
// building for some good on this hex's terrain/deposits. Build-time validation
// (POST .../build) and every read surface listing valid hexes share this gate.
func HexSupportsBuilding(ctx context.Context, tx Tx, worldID uuid.UUID, hex hexgrid.Coord, buildingType string) (bool, error) {
	var terrain string
	var copperDep, tinDep, silverDep bool
	err := tx.QueryRow(ctx,
		`SELECT mt.terrain, COALESCE(mt.copper_deposit, false), COALESCE(mt.tin_deposit, false), COALESCE(mt.silver_deposit, false)
		 FROM map_tiles mt WHERE mt.world_id = $1 AND mt.q = $2 AND mt.r = $3`,
		worldID, hex.Q, hex.R,
	).Scan(&terrain, &copperDep, &tinDep, &silverDep)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("hex supports building: %w", err)
	}
	return RuleBuildingSupportsHex(buildingType, terrain, copperDep, tinDep, silverDep), nil
}

// HexOccupant is the hex-bound building standing on a hex, or queued for it.
type HexOccupant struct {
	BuildingType string
	SettlementID uuid.UUID
}

// HexOccupants returns the hex-bound building (standing or queued) on each of
// coords that has one, across every settlement in the world: one building per
// hex (Timothy 2026-09-30), and two settlements' catchments may share a hex.
func HexOccupants(ctx context.Context, tx Tx, worldID uuid.UUID, coords []hexgrid.Coord) (map[hexgrid.Coord]HexOccupant, error) {
	qs, rs := hexgrid.QRArrays(coords)
	types := make([]string, 0, len(HexBoundBuildingTypes))
	for t := range HexBoundBuildingTypes {
		types = append(types, t)
	}
	rows, err := tx.Query(ctx,
		`SELECT x.hex_q, x.hex_r, x.building_type, x.settlement_id
		 FROM (SELECT settlement_id, building_type, hex_q, hex_r FROM buildings
		       UNION ALL
		       SELECT settlement_id, building_type, hex_q, hex_r FROM build_queue) x
		 JOIN settlements s ON s.id = x.settlement_id AND s.world_id = $1
		 JOIN unnest($2::int[], $3::int[]) AS c(q, r) ON c.q = x.hex_q AND c.r = x.hex_r
		 WHERE x.building_type = ANY($4)`,
		worldID, qs, rs, types,
	)
	if err != nil {
		return nil, fmt.Errorf("hex occupants: %w", err)
	}
	defer rows.Close()
	out := make(map[hexgrid.Coord]HexOccupant)
	for rows.Next() {
		var q, r int
		var o HexOccupant
		if err := rows.Scan(&q, &r, &o.BuildingType, &o.SettlementID); err != nil {
			return nil, fmt.Errorf("hex occupants: scan: %w", err)
		}
		out[hexgrid.Coord{Q: q, R: r}] = o
	}
	return out, rows.Err()
}

// ValidHexesForBuilding returns every catchment ring hex where buildingType
// could be built RIGHT NOW — HexSupportsBuilding's per-hex gate, plus no
// hex-bound building standing or queued there (any type, any settlement: one
// building per hex). A hex already carrying this settlement's building of the
// same type is the upgrade path, offered via placement-options' hexes[].building.
// This is the list the web build picker needs to offer the player valid hexes
// (megaron_plan_byggnad_pa_hex.md §A2) without duplicating the gate
// client-side.
func ValidHexesForBuilding(ctx context.Context, tx Tx, worldID uuid.UUID, settlementID uuid.UUID, center hexgrid.Coord, buildingType string) ([]hexgrid.Coord, error) {
	ring := hexgrid.Ring(center, hexgrid.CatchmentRadius)
	occupied, err := HexOccupants(ctx, tx, worldID, ring)
	if err != nil {
		return nil, fmt.Errorf("valid hexes for building: %w", err)
	}

	var out []hexgrid.Coord
	for _, c := range ring {
		if _, taken := occupied[c]; taken {
			continue
		}
		ok, err := HexSupportsBuilding(ctx, tx, worldID, c, buildingType)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, c)
		}
	}
	return out, nil
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
// PlaceCapPerGood must be checked against now that two settlements'
// catchments can overlap (§3, landed — CatchmentClearanceHexes lowered the
// minimum founding distance below 2*CatchmentRadius+1). For any hex that
// still belongs to only one settlement's catchment this returns EXACTLY what
// LoadPlacementCounts would have for that settlement alone; the two stay
// indistinguishable there.
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

// MarkHeldHexesFullyOccupied overrides occupancy so every good on a hex
// flagged in heldByOther reads as fully staffed (occupied == its own
// PlaceCapPerGood), regardless of that good's own remaining room. §2b gave
// the hex a single owner, so a hex held by someone else offers NO good at
// all — not "whatever headroom that good's own cap happens to have left".
// Shared by PlacementOptions' per-hex grid and /goods' aggregate marginal
// yield (api/handlers/settlement_placement.go, province.go) — one
// adjustment, not two copies. Returns occupancy unchanged (same map, no
// copy) when heldByOther is empty, the ordinary no-overlap case.
func MarkHeldHexesFullyOccupied(hexOptions []HexOption, heldByOther map[hexgrid.Coord]bool, occupancy map[hexgrid.Coord]map[string]int) map[hexgrid.Coord]map[string]int {
	if len(heldByOther) == 0 {
		return occupancy
	}
	out := make(map[hexgrid.Coord]map[string]int, len(occupancy))
	for hex, goods := range occupancy {
		out[hex] = goods
	}
	for _, opt := range hexOptions {
		if !heldByOther[opt.Coord] {
			continue
		}
		full := make(map[string]int, len(opt.PlaceCapPerGood))
		for good, cap := range opt.PlaceCapPerGood {
			full[good] = cap
		}
		out[opt.Coord] = full
	}
	return out
}

// MarginalYieldForSlot is the per-slot marginal yield of a BUILDING workplace
// (rate per capL1 slot times the building-level mult). A hex slot's marginal
// yield is HexYieldPerWorker(rate, mult) — one worker's output, hex_rules.go.
// Shared by PlacementOptions' buildGoods (api/handlers/settlement_placement.go,
// itemised per hex/building) and MarginalYieldPerGood below — one formula,
// two shapes, never a second formula.
func MarginalYieldForSlot(good string, rate float64, capL1 int, mult float64) float64 {
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
	consider := func(good string, yield float64, placeCap, occupied int) {
		if placeCap <= 0 || occupied >= placeCap {
			return
		}
		if cur, ok := best[good]; !ok || yield > cur {
			best[good] = yield
		}
	}
	for _, opt := range hexOptions {
		occ := placed.Hex[opt.Coord]
		for good, rate := range opt.RatePerGood {
			consider(good, HexYieldPerWorker(rate, opt.MultPerGood[good]), opt.PlaceCapPerGood[good], occ[good])
		}
	}
	for _, opt := range buildingOptions {
		occ := placed.Building[opt.BuildingType]
		for good, rate := range opt.RatePerGood {
			if capL1 := opt.CapL1PerGood[good]; capL1 > 0 {
				consider(good, MarginalYieldForSlot(good, rate, capL1, opt.MultPerGood[good]), opt.PlaceCapPerGood[good], occ[good])
			}
		}
	}
	return best
}

// FullCrewPotential is the per-good output a catchment gives with EVERY worker
// place filled (places * rate per worker * building multiplier), the same rule
// RecomputeProduction applies to placed workers. It is the "potential" the
// colonize preview and the goods tables show; boost rows (olive press / winery
// terrain rows) count as potential too.
func FullCrewPotential(hexOptions []HexOption) map[string]float64 {
	out := make(map[string]float64)
	for _, opt := range hexOptions {
		for good, rate := range opt.RatePerGood {
			out[good] += float64(opt.PlaceCapPerGood[good]) * rate * opt.MultPerGood[good]
		}
		for good, rate := range opt.BoostRatePerGood {
			out[good] += float64(opt.PlaceCapPerGood[good]) * rate * opt.MultPerGood[good]
		}
	}
	return out
}
