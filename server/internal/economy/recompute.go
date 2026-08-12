package economy

import (
	"context"
	"fmt"
	"math"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// REF_LABOR is the reference population for the production formula.
// yield_per_worker(g) = base_potential(g) / REF_LABOR.
// A good produced by a settlement at REF_LABOR workers with all citizens
// assigned gets base_potential as its rate — same as the pre-Fas-2 baseline.
const REF_LABOR = 100.0

// NearjordGrainPerTick is the settlement's own hex's flat, unconditional
// grain trickle — Timothy 2026-08-07 answered the taxonomy's §3.2/§16
// divergence (§3.2 recommended +15 as a starting point, §16 later settled it
// FAST at "~50 grain"): "Ska stadshexens lilla grainbidrag vara fast eller
// bero på terräng? FAST - SÄG 50 grain." The settlement's own hex is not a
// normal production tile (no farm workplace, not upgradeable, "ingen gubbe
// krävs") — this is why it is added directly to grain's rate here rather
// than flowing through the labor-weighted catchment potential like every
// other good; hexgrid.Ring (not Disk) already excludes this hex from that
// potential sum, so there is no double-count.
const NearjordGrainPerTick = 50.0

// Workplace capacity (Timothy 2026-07-23, absolute headcount since P2
// 2026-08-07 — megaron_plan_fysisk_gubbemodell.md). A producing building is a
// workplace with a finite number of STATIONS; its LEVEL sets how many citizens
// it can put to work, as a hard count of people, not a share of the city. Labor
// allocated past what the fields and buildings can employ is not served and
// produces nothing — so the only way to devote MORE of a city to a good is a
// bigger workplace. This generalises what kharis.templeDevotionCapacity already
// did for the temple (which is NOT in this table — cult labor is a separate
// path, megaron_cult_ar_ingen_vara_plan.md, deliberately untouched here).
//
// P2 replaces the old share-based BuildingLaborPerLevel (0.5 of the city per
// level — DE2=B, Timothy 2026-08-07: "arbetskraft räknas hädanefter i hela
// gubbar på bestämda platser"). That formula scaled a building's output with
// POPULATION: after P1's catchment 7→19, a single level-1 building granted the
// same 50%-of-the-city cap in a 50-pop hamlet and a 5000-pop metropolis, which
// is exactly the missing brake the P1 postmortem measured ("P1 gav 3x men inte
// bromsen"). An absolute slot count does not scale with population — a level-1
// Foundry employs a handful of smiths whether the city has 50 citizens or 5000.
//
// Canon table, Temenos_varutaxonomi_sol.md §8.2 — NOT invented here (an
// earlier version of this table extrapolated its own two tiers instead of
// reading §8.2 first, and got Mine/Silver mine wrong as a result; corrected
// 2026-08-08). §8.2 also lists Fishing place, Pasture, Barracks, Temple and
// Megaron — omitted below because they have no RecomputeProduction path
// today: Fishing place/Pasture don't exist as buildable types yet, Barracks
// trains units rather than producing a good, Temple is cult labor
// (megaron_cult_ar_ingen_vara_plan.md, a separate path), and Megaron is
// explicitly "Öppet" (uncapped) in the table. Shipyard (3/6/10, §8.2/§11.2,
// megaron_plan_skeppsreparation.md Slice A) IS listed below despite also
// having no production_rules row — it produces no good, but its slots gate
// shipbuilding/repair capacity the same way Temple's gate cult devotion.
// Foundry WAS omitted here (bronze ran through recipe.go craft-events, gated
// only on "foundry built") until P6 (2026-08-08) converted it to the same
// placed-gjutare model as every other good — see the bronze stock-drain step
// in RecomputeProduction.
// Add a building here when it gets a production_rules row that needs the cap.
// Array length 4 mirrors province.MaxBuildingLevel(3)+1 — economy may not
// import province (G1: economy(→clock,events,gossip,hexgrid) only), so the
// bound is a plain literal. If MaxBuildingLevel ever changes, update this
// array's length too (WorkplaceSlots silently returns 0 for any level past it,
// rather than crashing — but a raised level cap would then grant no extra
// slots until this table is widened).
var workplaceSlotTable = map[string][4]int{
	// index 0 unused (level is always ≥1); index = level.
	"farm":        {0, 2, 4, 6},
	"stonequarry": {0, 2, 4, 6},
	"lumbermill":  {0, 2, 4, 6},
	"harbour":     {0, 2, 4, 6},
	"shipyard":    {0, 3, 6, 10},
	"mine":        {0, 2, 4, 6},
	"silver_mine": {0, 2, 4, 6},
	"olive_press": {0, 1, 2, 4},
	"winery":      {0, 1, 2, 4},
	"market":      {0, 1, 2, 4},
	"stable":      {0, 1, 2, 4},
	"foundry":     {0, 1, 2, 4},
}

// WorkplaceSlots returns how many citizens a building of the given type and
// level can employ, as an absolute headcount. Building types missing from
// workplaceSlotTable (temple — cult labor, a separate path) or an unrecognised
// level return 0: an unrecognised building grants no slots rather than a
// silent guess (arbetssätt §7: no fallback that invents a value).
func WorkplaceSlots(buildingType string, level int) int {
	tiers, ok := workplaceSlotTable[buildingType]
	if !ok || level < 1 || level >= len(tiers) {
		return 0
	}
	return tiers[level]
}

// LoadWorkplaceSlots returns, per good_key, the summed absolute worker slots
// across every building in the settlement that produces it (a good made by two
// buildings, e.g. wine from both Farm and Winery, sums both buildings' slots).
// Shared by RecomputeProduction and every read surface that needs to explain a
// capacity (province.go's goods table, db.go's loadLaborCapacities) so the
// building→slots conversion lives in exactly one place — P1's postmortem found
// the same catchment-hex query duplicated at 13 call sites from not doing this
// the first time.
func LoadWorkplaceSlots(ctx context.Context, tx Tx, settlementID uuid.UUID) (map[string]int, error) {
	rows, err := tx.Query(ctx,
		`SELECT DISTINCT pr.good_key, b.building_type, b.level
		 FROM production_rules pr
		 JOIN buildings b ON b.settlement_id = $1 AND b.building_type = pr.building_type`,
		settlementID,
	)
	if err != nil {
		return nil, fmt.Errorf("load workplace slots: %w", err)
	}
	defer rows.Close()
	slots := make(map[string]int)
	for rows.Next() {
		var key, buildingType string
		var level int
		if err := rows.Scan(&key, &buildingType, &level); err != nil {
			return nil, fmt.Errorf("load workplace slots: scan: %w", err)
		}
		slots[key] += WorkplaceSlots(buildingType, level)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load workplace slots: rows: %w", err)
	}
	return slots, nil
}

// hexCapacityRule pairs a catchment condition (a terrain type, or a deposit
// flag) with the good it lets a citizen work there, and the per-hex worker cap
// with and without the relevant production building — Temenos_varutaxonomi_sol.md
// §8.3 (P3, megaron_plan_fysisk_gubbemodell.md). Every value here picks the LOW
// end of §8.3's range (its ranges are explicitly "kalibreringsratt, inte lås" —
// verify against how many gubbar a normal 8–17-gubbe city can place): headroom
// is cheap to raise, expensive to walk back once a Wanax has built around it.
//
// A hex can carry more than one rule — a plains hex is both "slätt" (grain) and
// "betesmark" (livestock; no distinct pasture terrain exists), and a hills hex
// with a copper deposit is both "mager åker" (grain) and "fyndighet" (copper).
// This is a deliberate simplification of the aggregate (pre-P4) model: the
// catchment can support up to N grain-workers AND up to M copper-workers as
// independent ceilings, not "this specific hex is EITHER a farm OR a mine" —
// that exclusivity is P4's job, once a hex holds one placed gubbe at a time.
type hexCapacityRule struct {
	goodKey          string
	capNoBuilding    int
	capWithBuilding  int
	relevantBuilding string // "" = no building in the game boosts this hex's cap
}

// terrainCapacityTable is keyed by map_tiles.terrain. Slätt (plains) carries
// two independent rules (grain AND livestock) so it is handled as a slice,
// like the deposit table below, rather than forced into this single-rule map.
var terrainCapacityTable = map[string]hexCapacityRule{
	"hills":              {"grain", 1, 2, "farm"},        // mager åker (mig 043 hills_grain)
	"river_valley":       {"grain", 2, 5, "farm"},        // floddal
	"river_delta":        {"grain", 3, 6, "farm"},        // delta
	"forest_olive_grove": {"timber", 1, 2, "lumbermill"}, // skog (no separate "forest" terrain exists)
	"forest_cedar":       {"cedar", 1, 2, "lumbermill"},  // cederskog
	"coastal_sea":        {"fish", 1, 2, "harbour"},      // kustfiske
	"river":              {"fish", 1, 2, ""},             // flodfiske
	"river_ford":         {"fish", 1, 2, ""},             // flodfiske
	"deep_sea":           {"fish", 1, 2, ""},             // flodfiske, unenhanced tier
}

// plainsCapacityRules: plains carries grain (slätt) AND livestock (betesmark)
// simultaneously — no distinct pasture terrain exists in the enum.
var plainsCapacityRules = []hexCapacityRule{
	{"grain", 2, 4, "farm"},
	{"livestock", 1, 3, ""}, // no pasture-boosting building exists in the game yet
}

// depositCapacityTable is keyed by the map_tiles deposit-flag column name
// (copper_deposit/tin_deposit/silver_deposit) — fyndighet, independent of the
// hex's terrain. cedar has no deposit flag (it is terrain-gated via
// forest_cedar, already in terrainCapacityTable), so it is not here.
var depositCapacityTable = map[string]hexCapacityRule{
	"copper": {"copper", 1, 3, "mine"},
	"tin":    {"tin", 1, 3, "mine"},
	"silver": {"silver", 1, 3, "silver_mine"},
}

// LoadHexCapacity returns, per good_key, the summed absolute worker slots the
// settlement's catchment hexes can hold for that good — hexCapacityRule's
// per-hex cap (with or without the relevant building, checked once for the
// whole settlement) times how many catchment hexes match. Mirrors
// LoadWorkplaceSlots' shape (P2) applied to hexes instead of buildings.
func LoadHexCapacity(ctx context.Context, tx Tx, settlementID uuid.UUID) (map[string]int, error) {
	var worldID uuid.UUID
	var q, r int
	if err := tx.QueryRow(ctx,
		`SELECT prov.world_id, prov.map_q, prov.map_r
		 FROM settlements s JOIN provinces prov ON prov.id = s.province_id
		 WHERE s.id = $1`,
		settlementID,
	).Scan(&worldID, &q, &r); err != nil {
		return nil, fmt.Errorf("load hex capacity: settlement coords: %w", err)
	}

	builtTypes := make(map[string]bool)
	brows, err := tx.Query(ctx, `SELECT DISTINCT building_type FROM buildings WHERE settlement_id = $1`, settlementID)
	if err != nil {
		return nil, fmt.Errorf("load hex capacity: buildings: %w", err)
	}
	for brows.Next() {
		var bt string
		if err := brows.Scan(&bt); err != nil {
			brows.Close()
			return nil, fmt.Errorf("load hex capacity: scan building: %w", err)
		}
		builtTypes[bt] = true
	}
	brows.Close()
	if err := brows.Err(); err != nil {
		return nil, fmt.Errorf("load hex capacity: building rows: %w", err)
	}
	hasBuilding := func(rule hexCapacityRule) bool {
		return rule.relevantBuilding != "" && builtTypes[rule.relevantBuilding]
	}
	capOf := func(rule hexCapacityRule) int {
		if hasBuilding(rule) {
			return rule.capWithBuilding
		}
		return rule.capNoBuilding
	}

	catchQ, catchR := hexgrid.QRArrays(hexgrid.Ring(hexgrid.Coord{Q: q, R: r}, hexgrid.CatchmentRadius))
	rows, err := tx.Query(ctx,
		`SELECT mt.terrain, COALESCE(mt.copper_deposit, false),
		        COALESCE(mt.tin_deposit, false), COALESCE(mt.silver_deposit, false)
		 FROM unnest($2::int[], $3::int[]) AS catchment(q, r)
		 JOIN map_tiles mt ON mt.world_id = $1 AND mt.q = catchment.q AND mt.r = catchment.r`,
		worldID, catchQ, catchR,
	)
	if err != nil {
		return nil, fmt.Errorf("load hex capacity: catchment tiles: %w", err)
	}
	defer rows.Close()

	slots := make(map[string]int)
	for rows.Next() {
		var terrain string
		var copperDep, tinDep, silverDep bool
		if err := rows.Scan(&terrain, &copperDep, &tinDep, &silverDep); err != nil {
			return nil, fmt.Errorf("load hex capacity: scan tile: %w", err)
		}
		if terrain == "plains" {
			for _, rule := range plainsCapacityRules {
				slots[rule.goodKey] += capOf(rule)
			}
		} else if rule, ok := terrainCapacityTable[terrain]; ok {
			slots[rule.goodKey] += capOf(rule)
		}
		if copperDep {
			rule := depositCapacityTable["copper"]
			slots[rule.goodKey] += capOf(rule)
		}
		if tinDep {
			rule := depositCapacityTable["tin"]
			slots[rule.goodKey] += capOf(rule)
		}
		if silverDep {
			rule := depositCapacityTable["silver"]
			slots[rule.goodKey] += capOf(rule)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load hex capacity: tile rows: %w", err)
	}
	return slots, nil
}

// GoodLaborTerrainBase is the FALLBACK share of a city that can work a good
// straight off the land, for goods §8.3 does not cover yet (oil, wine, stone —
// Temenos_varutaxonomi_sol.md §8.3 lists ten terrain/verksamhet rows and none
// of the three is one of them). Without a fallback, a good with a real
// terrain-only production_rule but no hexCapacityRule entry would silently
// drop to ZERO capacity the moment P3 landed — caught live by
// TestRecomputeProduction_WineOn{RiverValley,Plains}OnlyCatchment going from a
// positive rate to exactly 0. Kept only for the gap; do not add new goods to
// this fallback path — add them to terrainCapacityTable/plainsCapacityRules
// instead, the way P3 covers grain/timber/cedar/livestock/copper/tin/silver/fish.
const GoodLaborTerrainBase = 0.25

// LaborCapacity returns the share of a settlement's population that can
// actually be employed producing a good, given whether it has ANY terrain-only
// production path in this catchment (hasFieldPath — used only for the
// GoodLaborTerrainBase fallback above), the ABSOLUTE worker slots its
// catchment hexes can hold for goods §8.3 covers (see LoadHexCapacity, P3),
// and the ABSOLUTE worker slots its buildings can hold (see WorkplaceSlots,
// P2) — all expressed relative to the settlement's current labor pool
// (population), so multiplying the returned capacity back by laborPool
// reproduces hexSlots+buildingSlots exactly (up to the 1.0 cap), which is what
// keeps a covered good's employment from scaling with population.
//
// Grain is exempt: it is the subsistence good, its consumption is folded into
// its own net rate, and capping how many citizens may farm would starve cities
// that have no room to build. Hunger is not a staffing problem.
func LaborCapacity(goodKey string, hasFieldPath bool, hexSlots int, buildingSlots int, laborPool int) float64 {
	if goodKey == "grain" {
		return 1.0
	}
	var capacity float64
	if laborPool > 0 {
		capacity += float64(hexSlots) / float64(laborPool)
		capacity += float64(buildingSlots) / float64(laborPool)
	}
	if hexSlots == 0 && hasFieldPath {
		capacity += GoodLaborTerrainBase
	}
	if capacity > 1.0 {
		capacity = 1.0
	}
	return capacity
}

// GrainConsumptionPerCitizenPerTick is the grain eaten per citizen each tick
// (a tick is a day), folded into grain's net production rate below. Exported
// so read-only callers (status endpoint's grain-netto breakdown/break-even
// hint, DEL C of megaron_ekonomi_legibilitet_plan.md) can re-derive
// prod/consum from the stored net rate instead of duplicating this number.
const GrainConsumptionPerCitizenPerTick = 0.5

// GrainConsumptionPerTick is the grain a population of pop eats each tick.
// It depends on head-count alone — not on labor weights, terrain or buildings —
// so callers outside a settlement (the founder-phase host, which has people but
// no city yet) can use it without a settlements row. RecomputeProduction folds
// the same call into grain's net rate, so the number lives in exactly one place.
func GrainConsumptionPerTick(pop int) float64 {
	if pop < 0 {
		pop = 0
	}
	return float64(pop) * GrainConsumptionPerCitizenPerTick
}

// livestockFoodValue is the food value of one slaughtered animal — Timothy
// 2026-08-07: "jag tycker nästan att ett kreatur kan få leverera 200 mat om
// det dödas." Explicitly a ratt, not a lock (megaron_plan_foda_konsistens.md
// §Grind) — tune against soak data, not an invariant.
const livestockFoodValue = 200.0

// FoodConsumptionSplit applies the population's food fallback chain to a
// settlement's raw (pre-consumption) grain/fish production and its livestock
// STOCK: the population has ONE food need, demand, covered by grain first,
// then by fish for whatever grain does not reach, and — only once both are
// exhausted — by slaughtering whole animals from the herd (Timothy
// 2026-08-07, megaron_plan_foda_konsistens.md: "när det finns en brist på
// basvaror som vete och fisk så äter befolkningen boskapen"). Livestock is a
// discrete stock, not a rate, so livestockConsumed is a whole-animal count
// for the caller to debit directly from settlement_goods — this function
// itself makes no DB write.
//
//	grainShare = min(demand, grainProd)
//	rest       = demand - grainShare
//	fishShare  = min(rest, fishProd)
//	unmet      = rest - fishShare
//	livestockConsumed = min(floor(livestockStock), ceil(unmet / 200))  // whole animals only
//	unmet      = max(0, unmet - livestockConsumed*200)
//	grainNet   = grainProd - grainShare - unmet  // any shortfall still standing stays on grain
//	fishNet    = fishProd  - fishShare
//
// Pure and side-effect free (no DB, no clock) on purpose: RecomputeProduction,
// FoundingGrainNetPerTick, and read-only callers (the status endpoint's
// grain-netto breakdown, api/handlers/province.go) all share this one
// implementation instead of three copies that can drift apart.
//
// KNOWN LIMIT (Timothy 2026-07-31, confirmed wrong, deliberately deferred —
// booked in megaron_todo.md, NOT this slice): this is production-based, not
// stock-based, for grain/fish. A settlement with zero grain PRODUCTION but a
// full grain STOCK never draws down that stock as long as current-tick fish
// production covers the shortfall — grainNet floors at 0 instead of eating
// the stockpile. Timothy: "Detta måste ju lösas — har man båda ska båda ätas,
// inte nu men skriv i todo." Livestock does not share this limit — its
// livestockStock argument IS the settlement's actual current stock, read by
// the caller, so the herd fallback already draws down the real stockpile.
func FoodConsumptionSplit(demand, grainProd, fishProd, livestockStock float64) (grainNet, fishNet float64, livestockConsumed int) {
	grainShare := demand
	if grainProd < grainShare {
		grainShare = grainProd
	}
	rest := demand - grainShare
	fishShare := rest
	if fishProd < fishShare {
		fishShare = fishProd
	}
	unmet := rest - fishShare

	if unmet > 0 && livestockStock >= 1 {
		needed := math.Ceil(unmet / livestockFoodValue)
		available := math.Floor(livestockStock)
		slaughtered := needed
		if available < slaughtered {
			slaughtered = available
		}
		livestockConsumed = int(slaughtered)
		unmet -= slaughtered * livestockFoodValue
		if unmet < 0 {
			unmet = 0
		}
	}

	grainNet = grainProd - grainShare - unmet
	fishNet = fishProd - fishShare
	return grainNet, fishNet, livestockConsumed
}

// PopCosts mirrors province/training.go:UnitSpecs.PopCost.
// Defined here so economy stays Go-import-free upward (G1).
// galley = standardgalär (mig 084 renamed the canonical units.type key from
// "ship"; the DB army column is still `ship`, legacy). war_galley +
// merchantman = nya skepp-typer (mig 039). war_chariot ersatte cavalry/chariot
// (mig 042); catapult borttagen.
var PopCosts = map[string]int{
	"spearman":       5,
	"war_chariot":    8,
	"galley":         10,
	"elite_infantry": 10,
	"war_galley":     12,
	"merchantman":    8,
}

// Tx is the minimal interface accepted by RecomputeProduction so it can work
// with both pgx.Tx and pgxpool.Pool (the latter satisfies this interface too).
type Tx interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// GoodBaseValue reads a good's base_value from the goods catalog. Shared by the
// genesis-seed paths (handlers.Join, combat.foundColony) so the Sitos capacity
// formula uses the live catalog value rather than a hardcoded constant. Accepts
// the same Tx interface as RecomputeProduction (pgx.Tx or *pgxpool.Pool).
func GoodBaseValue(ctx context.Context, tx Tx, key string) (float64, error) {
	var v float64
	if err := tx.QueryRow(ctx, `SELECT base_value FROM goods WHERE key = $1`, key).Scan(&v); err != nil {
		return 0, fmt.Errorf("good base value %s: %w", key, err)
	}
	return v, nil
}

// RecomputeProduction settles and rewrites settlement_goods.rate for every
// producible good of the given settlement, using the P4 placement formula
// (megaron_plan_fysisk_gubbemodell.md — replaces the pre-P4 weight×laborPool
// aggregate for every good a gubbe can be individually placed on):
//
//	yield_per_worker(hex, g)      = hex's OWN production_rules.rate_per_tick(g) / hex's OWN P3 cap(g)
//	yield_per_worker(building, g) = building's rate_per_tick(g) / WorkplaceSlots(building, level)
//	rate(g) = Σ over every settlement_placement row assigned to g of that row's target's yield_per_worker(g)
//
// Timothy 2026-08-08: "ja varje placering ska ge sitt eget utbyte, varje hex
// har sitt eget utbyte" — a gubbe on hex H doing role g contributes ONLY what
// H itself yields for g, not a catchment-wide average (a plains farmer and a
// river-delta farmer produce different amounts even in the same city). A
// fully staffed hex (placed == cap) reproduces exactly the hex's flat
// rate_per_tick; an understaffed or empty catchment produces proportionally
// less — this (not the old REF_LABOR=100 aggregate) is the brake P1's
// postmortem found missing ("P1 gav inflationen men inte bromsen").
//
// Two additions outside the placement sum, both flat and NOT placeable roles:
//   - NearjordGrainPerTick — the settlement's own hex (§3.2, added below).
//   - UnconditionalPotential — production_rules rows with neither a terrain
//     nor a building gate (today: timber's anti-deadlock trickle, migration
//     033). Its whole purpose is "some output exists even with zero placed
//     workers of the right kind"; turning it into a placeable role would
//     defeat that, so it stays an unconditional constant like nearjord.
//   - The population REMAINDER (pop mod 100, Temenos_varutaxonomi_sol.md
//     §1.1 — only whole hundreds become placeable gubbar) "arbetar alltid
//     automatiskt i jordbruket": it contributes to GRAIN ONLY, via the OLD
//     aggregate formula (catchment-wide basePotential/REF_LABOR × remainder
//     headcount) scoped to just that leftover headcount — see step 5b.
//
// settlement_labor is UNTOUCHED by this function — cult labor (temple
// devotion, good_key='cult') is a separate path (megaron_cult_ar_ingen_vara_plan.md)
// that was never part of this formula even before P4.
//
// Must be called inside an existing transaction. Passes ctx to every DB call.
func RecomputeProduction(ctx context.Context, tx Tx, settlementID uuid.UUID) error {
	// ── 1. Compute labor_pool ────────────────────────────────────────────────
	// Part B: labor_pool = population. Soldiers are extracted from population at
	// recruit time (population -= men), so the army columns are no longer a drain here.
	var population int
	var ownerID uuid.UUID
	var worldID uuid.UUID
	var settlementQ, settlementR int
	err := tx.QueryRow(ctx,
		`SELECT s.population, s.owner_id, prov.world_id, prov.map_q, prov.map_r
		 FROM settlements s JOIN provinces prov ON prov.id = s.province_id
		 WHERE s.id = $1`,
		settlementID,
	).Scan(&population, &ownerID, &worldID, &settlementQ, &settlementR)
	if err != nil {
		return fmt.Errorf("recompute: load settlement: %w", err)
	}

	laborPool := population
	if laborPool < 0 {
		laborPool = 0
	}

	// ── 1b. Belägring S1+S2 (megaron_plan_belagring.md): which of the
	// catchment ring's hexes the settlement still has physical access to,
	// and whether it counts as besieged at all. Computed once here (not
	// inside LoadHexProductionOptions/CatchmentBasePotential separately) so
	// the expensive graph walk runs at most once per RecomputeProduction call
	// — the "billig förkoll" inside ReachableCatchmentHexes already skips it
	// entirely for the common case of no enemy nearby.
	center := hexgrid.Coord{Q: settlementQ, R: settlementR}
	ring := hexgrid.Ring(center, hexgrid.CatchmentRadius)
	reachable, besieged, err := ReachableCatchmentHexes(ctx, tx, worldID, ownerID, center, ring)
	if err != nil {
		return fmt.Errorf("recompute: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET besieged = $2 WHERE id = $1 AND besieged <> $2`,
		settlementID, besieged,
	); err != nil {
		return fmt.Errorf("recompute: write besieged flag: %w", err)
	}

	// ── 2. Load this settlement's placement-eligible production menu ─────────
	// Every catchment ring hex's own production menu, every built workplace
	// building's terrain-free menu, and the flat unconditional trickle — see
	// LoadHexProductionOptions/LoadBuildingProductionOptions/
	// UnconditionalPotential doc comments (placement_yield.go) for exactly
	// what each covers and why they're split three ways. reachable is nil
	// (unfiltered) for an unbesieged settlement — ReachableCatchmentHexes
	// returns the full ring map in that case, not nil, so pass it through
	// unconditionally; LoadHexProductionOptions treats a full map the same as
	// no filtering, just with one extra map lookup per hex.
	hexOptions, err := LoadHexProductionOptions(ctx, tx, settlementID, reachable)
	if err != nil {
		return fmt.Errorf("recompute: %w", err)
	}
	buildingOptions, err := LoadBuildingProductionOptions(ctx, tx, settlementID)
	if err != nil {
		return fmt.Errorf("recompute: %w", err)
	}
	unconditional, err := UnconditionalPotential(ctx, tx)
	if err != nil {
		return fmt.Errorf("recompute: %w", err)
	}

	// ── 3. Load actual placements ─────────────────────────────────────────────
	// Who is standing where doing what, right now (P4) — this, not a weight,
	// is what determines rate below.
	placements, err := LoadPlacementCounts(ctx, tx, settlementID)
	if err != nil {
		return fmt.Errorf("recompute: %w", err)
	}

	// NOTE: no early return when the catchment has no matching hexes/buildings.
	// A settlement that just lost its last producible good (deposit exhausted,
	// building razed, terrain changed) must still fall through to step 6 below,
	// which nulls out any good_key whose production_rule no longer matches —
	// otherwise its old rate is orphaned forever (the exact bug this function
	// exists to fix).

	// ── 4. Sum each placement's OWN target's yield into that good's rate ─────
	// producible mirrors the pre-P4 "potentials" list — every good_key this
	// catchment could ever produce, staffed or not — so a good with zero
	// placed workers still gets an explicit rate=0 row (below) instead of
	// silently vanishing, and step 6 still catches a good that lost its LAST
	// hex/building entirely.
	rawRates := make(map[string]float64)
	producible := make(map[string]bool)
	for _, opt := range hexOptions {
		for good := range opt.RatePerGood {
			producible[good] = true
		}
	}
	for _, opt := range buildingOptions {
		for good := range opt.RatePerGood {
			producible[good] = true
		}
	}
	for good := range unconditional {
		producible[good] = true
	}
	for good := range producible {
		rawRates[good] = 0
	}

	for _, opt := range hexOptions {
		placedHere := placements.Hex[opt.Coord]
		for good, rate := range opt.RatePerGood {
			placed := placedHere[good]
			if placed <= 0 {
				continue
			}
			rawRates[good] += placementYield(good, rate, opt.CapPerGood[good], placed)
		}
	}
	for _, opt := range buildingOptions {
		placedHere := placements.Building[opt.BuildingType]
		for good, rate := range opt.RatePerGood {
			// weakestLinkRefiningBuilding's goods (oil, wine) and bronze are
			// refining-only rows here (P6) — a pressarbetare/vinmakare/
			// gjutare's yield is NOT added directly; oil/wine go through step
			// 4c's min(boost, refining) below, bronze through step 5d's stock
			// drain. Adding it here too would double-count it AND bypass the
			// weakest-link gate entirely (a refining gubbe with zero
			// extraction upstream would otherwise produce for free).
			if _, ok := weakestLinkRefiningBuilding[good]; ok || good == GoodBronze {
				continue
			}
			placed := placedHere[good]
			if placed <= 0 {
				continue
			}
			rawRates[good] += placementYield(good, rate, opt.CapPerGood[good], placed)
		}
	}
	for good, rate := range unconditional {
		rawRates[good] += rate
	}

	// ── 4c. Weakest link: oil/wine's boost tier requires a SEPARATE refining
	// gubbe (P6, megaron_plan_fysisk_gubbemodell.md §P6; Timothy 2026-08-08 —
	// strict, an unstaffed press gives no boost). boostPotential is the SAME
	// extraction gubbar's terrain+building combined rows (opt.BoostRatePerGood
	// — the old "boost from building existing" rate, reinterpreted as a
	// ceiling); refiningCapacity is a genuinely separate placement (a
	// pressarbetare/vinmakare standing IN the building). The realized boost is
	// the smaller of the two — never negative, never more than either side
	// alone could support.
	for good := range weakestLinkRefiningBuilding {
		boostPotential := 0.0
		for _, opt := range hexOptions {
			rate, ok := opt.BoostRatePerGood[good]
			if !ok {
				continue
			}
			placed := placements.Hex[opt.Coord][good]
			if placed <= 0 {
				continue
			}
			boostPotential += placementYield(good, rate, opt.CapPerGood[good], placed)
		}
		refiningCapacity := 0.0
		for _, opt := range buildingOptions {
			rate, ok := opt.RatePerGood[good]
			if !ok {
				continue
			}
			placed := placements.Building[opt.BuildingType][good]
			if placed <= 0 {
				continue
			}
			refiningCapacity += placementYield(good, rate, opt.CapPerGood[good], placed)
		}
		if boostPotential > 0 && refiningCapacity > 0 {
			rawRates[good] += math.Min(boostPotential, refiningCapacity)
		}
	}

	// ── 4b. The population REMAINDER auto-farms grain ─────────────────────────
	// Temenos_varutaxonomi_sol.md §1.1: only whole hundreds become a placeable
	// gubbe (floor(population/100)); "den ofullständiga befolkningsresten
	// arbetar alltid automatiskt i jordbruket" — the leftover heads (population
	// mod 100) are NOT a placement, they use the OLD pre-P4 aggregate formula
	// (catchment-wide basePotential/REF_LABOR), scoped to just their own
	// headcount, and GRAIN ONLY ("jordbruket", not fish/oil/wine/…). This is
	// deliberately the one place REF_LABOR still governs output after P4 — it
	// is what makes population growth feel continuous between hundreds instead
	// of a nothing-then-a-gubbe step function.
	grainBasePotential := 0.0
	for _, opt := range hexOptions {
		grainBasePotential += opt.RatePerGood[GoodGrain]
	}
	remainderCitizens := laborPool % 100
	rawRates[GoodGrain] += (grainBasePotential / REF_LABOR) * float64(remainderCitizens)
	producible[GoodGrain] = true

	// Grain carries a population-consumption term folded into its NET rate:
	// pop × 0.5 per tick = consumption per tick (a tick is a day). Folding it
	// (rather than subtracting a daily lump elsewhere) keeps consumption
	// continuous, so it never exceeds the grain cap and a self-sufficient city
	// sits at a stable positive stock instead of sawtoothing to zero every day.
	// laborPool is the non-negative population (Σ eaters).
	//
	// Fisk-föder-befolkningen (2026-07-31): the population's food need is covered
	// by grain FIRST, then by fish for whatever grain does not reach — see
	// FoodConsumptionSplit. The army's upkeep path never touches fish (it debits
	// settlement_goods good_key='grain'/'silver' only — internal/combat/upkeep.go).
	grainConsumptionPerTick := GrainConsumptionPerTick(laborPool)

	// ── 5. Settle and write new rates ─────────────────────────────────────────
	// 5a. rawRates is already fully populated (step 4/4b) — nothing left to
	// compute here; producibleKeys (derived from the `producible` set) drives
	// the write loop (5c) and step 6's stale cleanup below.
	producibleKeys := make([]string, 0, len(producible))
	for good := range producible {
		producibleKeys = append(producibleKeys, good)
	}
	// Stadshexens närjord (P1, megaron_plan_fysisk_gubbemodell.md §3.2): a
	// flat, unconditional grain trickle from the settlement's own hex, which
	// is not a normal production tile (no gubbe, not upgradeable — excluded
	// from the catchment ring above). Added directly to the rate here, not
	// routed through LaborCapacity/weight scaling like every other good.
	rawRates["grain"] += NearjordGrainPerTick

	// 5b. The food invariant: grain first, fish for the rest, livestock (whole
	// animals) as the last resort. grainProd/fishProd default to 0 via Go's
	// zero value when the catchment has no matching tile for that good (map
	// lookup on a missing key) — exactly the "no water" / "no plains" case
	// AK2/AK1 exercise.
	//
	// Livestock is a discrete stock, not a catchment rate, so it is read here
	// directly rather than coming from rawRates. The `calc_tick =
	// current_world_tick()` guard stops a settlement whose herd was already
	// settled THIS tick (an earlier slaughter this same tick, or an
	// same-tick founding write) from being offered to a second
	// RecomputeProduction call within the same tick — RecomputeProduction is
	// called many times a day (build/train/colonize/collapse), not once, and
	// without the guard each call would independently re-decide to slaughter
	// against the SAME unmet demand.
	var livestockStock float64
	var livestockSettledThisTick bool
	if err := tx.QueryRow(ctx,
		`SELECT settled(amount, rate, calc_tick), calc_tick = current_world_tick()
		 FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'livestock'`,
		settlementID,
	).Scan(&livestockStock, &livestockSettledThisTick); err != nil && err != pgx.ErrNoRows {
		return fmt.Errorf("recompute: load livestock stock: %w", err)
	}
	if livestockSettledThisTick {
		livestockStock = 0
	}

	grainNet, fishNet, livestockConsumed := FoodConsumptionSplit(
		grainConsumptionPerTick, rawRates["grain"], rawRates["fish"], livestockStock)

	if livestockConsumed > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods
			 SET amount    = GREATEST(0, settled(amount, rate, calc_tick) - $2),
			     calc_tick = current_world_tick()
			 WHERE settlement_id = $1 AND good_key = 'livestock'`,
			settlementID, float64(livestockConsumed),
		); err != nil {
			return fmt.Errorf("recompute: slaughter livestock for food: %w", err)
		}
	}

	// 5c. Write every producible good's rate — grain and fish get their
	// food-invariant net; everything else keeps its raw rate unchanged.
	// grainSeen is unused past this point: step 4b unconditionally forces
	// "grain" into producible (nearjord + the population remainder apply to
	// EVERY settlement with a settlements row, farmland or not), so grain
	// always goes through this same loop — the pre-P4 fallback write for a
	// "grain never appeared in potentials" case no longer exists to need one.
	for _, key := range producibleKeys {
		newRate := rawRates[key]
		switch key {
		case "grain":
			newRate = grainNet
		case "fish":
			newRate = fishNet
		}

		// Settle existing amount at old rate, then overwrite rate + calc_tick.
		if _, err := tx.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
			 VALUES ($1, $2, 0, $3, $4, current_world_tick())
			 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
			     amount  = LEAST(settlement_goods.cap,
			                 GREATEST(0, settled(settlement_goods.amount, settlement_goods.rate, settlement_goods.calc_tick))),
			     rate    = $3,
			     calc_tick = current_world_tick()`,
			settlementID, key, newRate, goodCap(key),
		); err != nil {
			return fmt.Errorf("recompute: upsert good %s: %w", key, err)
		}
	}

	// ── 5d. Bronze: a stock drain, not a rate (P6, megaron_plan_fysisk_gubbemodell.md
	// §P6, Timothy 2026-08-08 — foundry converted from manual craft to the same
	// placed-gubbe model as everything else). Copper and tin are real,
	// tradeable goods (§3.3) — diverting their RATE into bronze would make
	// every unit vanish before it ever reached the settlement's sellable
	// stock. Bronze is instead smelted FROM the already-settled stock each
	// time RecomputeProduction runs, capped by whichever ingredient runs out
	// first AND by the foundry's refining capacity (a gjutare placed there —
	// same weakest-link shape as step 4c, minus the "boost on top of a
	// baseline" part: bronze has no unrefined baseline, only recipe.go's
	// existing 9:1 ratio (mig 099), read live so this never drifts from the
	// same number `GET /api/v1/recipes` shows). Mirrors the livestock
	// slaughter above: a discrete stock mutation, not a lazy-eval rate —
	// bronze's own settlement_goods.rate stays 0 (written by the loop above);
	// its amount jumps once per RecomputeProduction call instead of accruing
	// smoothly between calls.
	bronzeRefiningCapacity := 0.0
	for _, opt := range buildingOptions {
		rate, ok := opt.RatePerGood[GoodBronze]
		if !ok {
			continue
		}
		placed := placements.Building[opt.BuildingType][GoodBronze]
		if placed <= 0 {
			continue
		}
		bronzeRefiningCapacity += placementYield(GoodBronze, rate, opt.CapPerGood[GoodBronze], placed)
	}
	if bronzeRefiningCapacity > 0 {
		type bronzeIngredient struct {
			good string
			qty  float64
		}
		ingredientRows, err := tx.Query(ctx,
			`SELECT ri.good_key, ri.quantity
			 FROM recipes r JOIN recipe_ingredients ri ON ri.recipe_id = r.id
			 WHERE r.output_key = $1`,
			GoodBronze,
		)
		if err != nil {
			return fmt.Errorf("recompute: load bronze recipe: %w", err)
		}
		var ingredients []bronzeIngredient
		for ingredientRows.Next() {
			var ing bronzeIngredient
			if err := ingredientRows.Scan(&ing.good, &ing.qty); err != nil {
				ingredientRows.Close()
				return fmt.Errorf("recompute: scan bronze recipe: %w", err)
			}
			ingredients = append(ingredients, ing)
		}
		ingredientRows.Close()
		if err := ingredientRows.Err(); err != nil {
			return fmt.Errorf("recompute: bronze recipe rows: %w", err)
		}

		bronzeProduced := bronzeRefiningCapacity
		for _, ing := range ingredients {
			var stock float64
			if err := tx.QueryRow(ctx,
				`SELECT settled(amount, rate, calc_tick)
				 FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
				settlementID, ing.good,
			).Scan(&stock); err != nil && err != pgx.ErrNoRows {
				return fmt.Errorf("recompute: load %s stock for bronze: %w", ing.good, err)
			}
			if ing.qty > 0 {
				bronzeProduced = math.Min(bronzeProduced, stock/ing.qty)
			}
		}
		if bronzeProduced > 0 {
			for _, ing := range ingredients {
				if _, err := tx.Exec(ctx,
					`UPDATE settlement_goods
					 SET amount    = GREATEST(0, settled(amount, rate, calc_tick) - $2),
					     calc_tick = current_world_tick()
					 WHERE settlement_id = $1 AND good_key = $3`,
					settlementID, bronzeProduced*ing.qty, ing.good,
				); err != nil {
					return fmt.Errorf("recompute: consume %s for bronze: %w", ing.good, err)
				}
			}
			if _, err := tx.Exec(ctx,
				`UPDATE settlement_goods
				 SET amount    = LEAST(cap, settled(amount, rate, calc_tick) + $2),
				     calc_tick = current_world_tick()
				 WHERE settlement_id = $1 AND good_key = 'bronze'`,
				settlementID, bronzeProduced,
			); err != nil {
				return fmt.Errorf("recompute: credit bronze: %w", err)
			}
		}
	}

	// ── 6. Null out stale rates for goods that fell out of the production set ──
	// A good_key can carry a rate from an earlier call whose production_rule no
	// longer matches this catchment/buildings NOW (deposit exhausted, building
	// razed, terrain changed — empirically hit 2026-07-29: a rate=42 cedar row
	// survived on a cedar-less city through a later recompute). Every such row
	// must be zeroed, not left orphaned. touchedKeys is every good_key this call
	// just wrote a real rate for (step 5 above); anything else with a non-zero
	// rate is stale.
	//
	// Settle at the OLD rate before zeroing — same settled()/GREATEST(0)/cap
	// pattern as every other write in this function — so production between the
	// last calc_tick and now is neither lost nor fabricated.
	touchedKeys := producibleKeys
	if _, err := tx.Exec(ctx,
		`UPDATE settlement_goods
		 SET amount    = LEAST(cap, GREATEST(0, settled(amount, rate, calc_tick))),
		     rate      = 0,
		     calc_tick = current_world_tick()
		 WHERE settlement_id = $1
		   AND rate <> 0
		   AND good_key <> ALL($2)`,
		settlementID, touchedKeys,
	); err != nil {
		return fmt.Errorf("recompute: null stale rates: %w", err)
	}

	return nil
}

// goodCap returns the storage cap for a good key (mirrors join.go/arrival.go
// caps). Deliberately loosened to a non-binding ceiling for this dev phase
// (2026-07-05): the old flat per-good values (grain=1000 etc.) were far below
// what population-scaled production reaches within a single day, so every
// growing settlement pegged at cap regardless of terrain — masking the
// geography-driven scarcity/surplus differentiation temenos_ekonomi.md is
// built on. Pricing no longer derives its reference from cap (see
// ProductionReference in price.go), so this value is now purely a technical
// storage ceiling, not a gameplay lever — kept finite for SQL/float safety,
// not calibrated as a balance number.
func goodCap(key string) float64 {
	return 1_000_000
}
