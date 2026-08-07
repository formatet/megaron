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

// Workplace capacity (Timothy 2026-07-23). A producing building is a workplace
// with a finite number of stations; its LEVEL is how many citizens it can put to
// work. Labor allocated past what the fields and buildings can employ is not
// served and produces nothing — so the only way to devote MORE of a city to a
// good is a bigger workplace. This generalises what kharis.templeDevotionCapacity
// already did for the temple, which until now was the one building whose level
// meant anything (and which could not actually be levelled — see the
// `upgradeable` set in api/handlers/province.go).
//
// STRAWMAN CALIBRATION, not invariants — tune against soak data. Both numbers
// are chosen so that NOTHING in the live world regresses on the day this lands:
// the highest field-good allocation in drift is 0.25 (timber) and the highest
// building-only allocation is 0.50 (silver). Level 1 therefore already covers
// every existing city, exactly as Timothy calibrated the temple's level 1 onto
// the 0.15 LaborAlloc floor. Levels buy headroom nobody is yet using.
const (
	// GoodLaborTerrainBase is the share of a city that can work a good straight
	// off the land, with no building at all. Fields need no stations.
	GoodLaborTerrainBase = 0.25
	// BuildingLaborPerLevel is the share of a city each level of a producing
	// building can additionally employ.
	BuildingLaborPerLevel = 0.5
)

// LaborCapacity returns the share of a settlement's population that can actually
// be employed producing a good, given whether the good has a field (terrain-only)
// production path in this catchment and the summed level of the settlement's
// buildings that produce it.
//
// Grain is exempt: it is the subsistence good, its consumption is folded into its
// own net rate, and capping how many citizens may farm would starve cities that
// have no room to build. Hunger is not a staffing problem.
func LaborCapacity(goodKey string, hasFieldPath bool, buildingLevels int) float64 {
	if goodKey == "grain" {
		return 1.0
	}
	var capacity float64
	if hasFieldPath {
		capacity += GoodLaborTerrainBase
	}
	if buildingLevels > 0 {
		capacity += BuildingLaborPerLevel * float64(buildingLevels)
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
// producible good of the given settlement, using the citizen-allocation formula:
//
//	labor_pool        = population (Part B: soldiers are drawn from population at recruit time,
//	                    so the integer army columns no longer represent a labor drain)
//	base_potential(g) = SUM(production_rules matching terrain/deposits/buildings)
//	yield_per_worker(g) = base_potential(g) / REF_LABOR
//	rate(g)           = yield_per_worker(g) × citizens(g)
//
// Σ citizens ≤ labor_pool is validated at the allocation endpoint; here we
// only recompute rates from whatever citizens are stored.
//
// The unconditional timber trickle (NULL terrain, NULL building rule) is
// included in base_potential so the anti-deadlock property is preserved for
// any non-zero population and citizen allocation.
//
// Must be called inside an existing transaction. Passes ctx to every DB call.
func RecomputeProduction(ctx context.Context, tx Tx, settlementID uuid.UUID) error {
	// ── 1. Compute labor_pool ────────────────────────────────────────────────
	// Part B: labor_pool = population. Soldiers are extracted from population at
	// recruit time (population -= men), so the army columns are no longer a drain here.
	var population int
	err := tx.QueryRow(ctx,
		`SELECT population FROM settlements WHERE id = $1`,
		settlementID,
	).Scan(&population)
	if err != nil {
		return fmt.Errorf("recompute: load settlement: %w", err)
	}

	laborPool := population
	if laborPool < 0 {
		laborPool = 0
	}

	// ── 2. Gather catchment coordinates for this settlement ──────────────────
	// Catchment = the settlement's own hex plus hexgrid.CatchmentRadius around
	// it (P1, megaron_plan_fysisk_gubbemodell.md — 19 hexes total, 18 worked;
	// own hex handled separately in step 3b below, not a normal production tile).
	var worldID uuid.UUID
	var q, r int
	err = tx.QueryRow(ctx,
		`SELECT prov.world_id, prov.map_q, prov.map_r
		 FROM settlements s
		 JOIN provinces prov ON prov.id = s.province_id
		 WHERE s.id = $1`,
		settlementID,
	).Scan(&worldID, &q, &r)
	if err != nil {
		return fmt.Errorf("recompute: load province coords: %w", err)
	}

	// ── 3. Compute base_potential per producible good from catchment ──────────
	// Each of the 18 productive catchment map_tiles (the ring, NOT the
	// settlement's own hex — see step 3b) contributes based on its own
	// terrain, deposits, and coastal flag. The settlement's buildings gate
	// building-gated rules. Water tiles (deep_sea, coastal_sea, river,
	// river_ford) only match rules keyed to their own terrain — otherwise a
	// water tile would match every terrain_type IS NULL rule (timber, stone
	// via mine, pottery via market), which is exactly the silent-fallback
	// production CLAUDE.md forbids (megaron_floden_plan.md §4, Timothy
	// 2026-07-29; river_ford added megaron_plan_flodbudget_och_vadstalle.md,
	// same reasoning). The `g.status = 'active'` join excludes parked goods
	// (purple/pottery/horses, mig 114, Temenos_varutaxonomi_sol.md §4.2) from
	// base_potential entirely — step 6 below then nulls any stale rate a
	// parked good already carried.
	catchQ, catchR := hexgrid.QRArrays(hexgrid.Ring(hexgrid.Coord{Q: q, R: r}, hexgrid.CatchmentRadius))
	rows, err := tx.Query(ctx,
		`SELECT pr.good_key, SUM(pr.rate_per_tick) AS base_potential,
		        bool_or(pr.building_type IS NULL) AS has_field_path
		 FROM unnest($3::int[], $4::int[]) AS catchment(q, r)
		 JOIN map_tiles mt ON mt.world_id = $2 AND mt.q = catchment.q AND mt.r = catchment.r
		 JOIN production_rules pr ON
		     (pr.terrain_type IS NULL OR pr.terrain_type = mt.terrain)
		     AND (NOT pr.requires_coastal OR mt.coastal)
		     AND (pr.building_type IS NULL OR EXISTS (
		             SELECT 1 FROM buildings b
		             WHERE b.settlement_id = $1 AND b.building_type = pr.building_type))
		     AND (pr.requires_deposit IS NULL
		          OR (pr.requires_deposit = 'copper' AND mt.copper_deposit)
		          OR (pr.requires_deposit = 'tin'    AND mt.tin_deposit)
		          OR (pr.requires_deposit = 'silver' AND COALESCE(mt.silver_deposit, false))
		          OR (pr.requires_deposit = 'cedar'  AND COALESCE(mt.cedar_deposit, false)))
		 JOIN goods g ON g.key = pr.good_key AND g.status = 'active'
		 WHERE mt.terrain NOT IN ('deep_sea','coastal_sea','river','river_ford')
		        OR pr.terrain_type = mt.terrain
		 GROUP BY pr.good_key`,
		settlementID, worldID, catchQ, catchR,
	)
	if err != nil {
		return fmt.Errorf("recompute: query production rules: %w", err)
	}
	type goodPotential struct {
		key           string
		basePotential float64
		hasFieldPath  bool
	}
	var potentials []goodPotential
	for rows.Next() {
		var gp goodPotential
		if err := rows.Scan(&gp.key, &gp.basePotential, &gp.hasFieldPath); err != nil {
			rows.Close()
			return fmt.Errorf("recompute: scan potential: %w", err)
		}
		potentials = append(potentials, gp)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("recompute: rows err: %w", err)
	}

	// ── 3b. Workplace capacity per good ───────────────────────────────────────
	// A building is a workplace with a finite number of stations, and its LEVEL
	// is how many citizens it can put to work (Timothy 2026-07-23, generalising
	// the temple's templeDevotionCapacity to every producing building). Labor
	// allocated beyond what the settlement's buildings + fields can employ is not
	// served and does not produce — so the only way to devote MORE of a city to
	// a good is a bigger workplace. Levels sum: two buildings that both make oil
	// (farm + olive press) each contribute their own stations.
	buildingLevels := make(map[string]int)
	brows, berr := tx.Query(ctx,
		`SELECT good_key, SUM(level)::int FROM (
		     SELECT DISTINCT pr.good_key, b.building_type, b.level
		     FROM production_rules pr
		     JOIN buildings b ON b.settlement_id = $1 AND b.building_type = pr.building_type
		 ) t GROUP BY good_key`,
		settlementID,
	)
	if berr != nil {
		return fmt.Errorf("recompute: query workplace levels: %w", berr)
	}
	for brows.Next() {
		var key string
		var lvl int
		if err := brows.Scan(&key, &lvl); err != nil {
			brows.Close()
			return fmt.Errorf("recompute: scan workplace level: %w", err)
		}
		buildingLevels[key] = lvl
	}
	brows.Close()
	if err := brows.Err(); err != nil {
		return fmt.Errorf("recompute: workplace level rows err: %w", err)
	}

	// NOTE: no early return when potentials is empty. A settlement that just
	// lost its last producible good (deposit exhausted, building razed, terrain
	// changed) must still fall through to step 6 below, which nulls out any
	// good_key whose production_rule no longer matches — otherwise its old
	// rate is orphaned forever (the exact bug this function exists to fix).

	// ── 4. Load weight allocations for this settlement ────────────────────────
	// weight ∈ [0.0,1.0] = fraction of labor_pool dedicated to this good.
	// effective_workers = weight × labor_pool; rate = (base_potential/REF_LABOR) × effective_workers.
	crows, err := tx.Query(ctx,
		`SELECT good_key, weight FROM settlement_labor WHERE settlement_id = $1`,
		settlementID,
	)
	if err != nil {
		return fmt.Errorf("recompute: load weights: %w", err)
	}
	weights := make(map[string]float64)
	for crows.Next() {
		var key string
		var w float64
		if err := crows.Scan(&key, &w); err != nil {
			crows.Close()
			return fmt.Errorf("recompute: scan weight: %w", err)
		}
		weights[key] = w
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return fmt.Errorf("recompute: weight rows err: %w", err)
	}

	// Seed equal weights on first call (e.g. at join before explicit allocation).
	// Existing rows are never auto-adjusted — a new producible good gets weight=0
	// until the Wanax allocates via LaborAlloc.
	if len(weights) == 0 && len(potentials) > 0 {
		n := len(potentials)
		w := 1.0 / float64(n)
		for _, gp := range potentials {
			weights[gp.key] = w
			if _, err := tx.Exec(ctx,
				`INSERT INTO settlement_labor (settlement_id, good_key, weight)
				 VALUES ($1, $2, $3)
				 ON CONFLICT (settlement_id, good_key) DO NOTHING`,
				settlementID, gp.key, w,
			); err != nil {
				return fmt.Errorf("recompute: seed weight %s: %w", gp.key, err)
			}
		}
	}

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
	// 5a. Raw (pre-consumption) production per good, from labor allocation alone
	// — exactly RecomputeProduction's pre-Fas-2 formula, unchanged for every good
	// except grain/fish, which get the food-invariant split applied below.
	rawRates := make(map[string]float64, len(potentials))
	for _, gp := range potentials {
		staffed := weights[gp.key]
		if cap := LaborCapacity(gp.key, gp.hasFieldPath, buildingLevels[gp.key]); staffed > cap {
			staffed = cap
		}
		effectiveWorkers := staffed * float64(laborPool)
		yieldPerWorker := gp.basePotential / REF_LABOR
		rawRates[gp.key] = yieldPerWorker * effectiveWorkers
	}
	// grainSeen reflects the CATCHMENT TERRAIN'S own grain capability (was
	// "grain" among the potentials this settlement's ring actually produced),
	// captured BEFORE NearjordGrainPerTick is folded in below — it gates
	// whether step 5's potentials-loop already writes a grain row (below) vs.
	// the "!grainSeen" fallback branch needs to (a farmless/waterless city
	// still gets nearjord's flat trickle, but has no potentials-loop entry to
	// carry it).
	_, grainSeen := rawRates["grain"]

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
	for _, gp := range potentials {
		newRate := rawRates[gp.key]
		switch gp.key {
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
			settlementID, gp.key, newRate, goodCap(gp.key),
		); err != nil {
			return fmt.Errorf("recompute: upsert good %s: %w", gp.key, err)
		}
	}

	// A settlement with population but no grain-producing catchment still eats:
	// write a grain row with the food-invariant net rate (fish already deducted
	// via the split above, so a coastal-but-farmless city still gets fish
	// relief) so neglected non-farming, non-fishing cities drain and starve as
	// designed.
	if !grainSeen && grainConsumptionPerTick > 0 {
		if _, err := tx.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
			 VALUES ($1, 'grain', 0, $2, $3, current_world_tick())
			 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
			     amount  = LEAST(settlement_goods.cap,
			                 GREATEST(0, settled(settlement_goods.amount, settlement_goods.rate, settlement_goods.calc_tick))),
			     rate    = $2,
			     calc_tick = current_world_tick()`,
			settlementID, grainNet, goodCap("grain"),
		); err != nil {
			return fmt.Errorf("recompute: upsert grain consumption: %w", err)
		}
	}

	// ── 6. Null out stale rates for goods that fell out of the production set ──
	// A good_key can carry a rate from an earlier call whose production_rule no
	// longer matches this catchment/buildings NOW (deposit exhausted, building
	// razed, terrain changed — empirically hit 2026-07-29: a rate=42 cedar row
	// survived on a cedar-less city through a later recompute). Every such row
	// must be zeroed, not left orphaned. touchedKeys is every good_key this call
	// just wrote a real rate for (steps 5 and the grain-consumption branch
	// above); anything else with a non-zero rate is stale.
	//
	// Settle at the OLD rate before zeroing — same settled()/GREATEST(0)/cap
	// pattern as every other write in this function — so production between the
	// last calc_tick and now is neither lost nor fabricated. This covers BOTH
	// known leaks: the potentials-loop only touching goods still in the query
	// result, and the (now-removed) early return when potentials was empty,
	// which used to skip this settlement entirely.
	touchedKeys := make([]string, 0, len(potentials)+1)
	for _, gp := range potentials {
		touchedKeys = append(touchedKeys, gp.key)
	}
	if !grainSeen && grainConsumptionPerTick > 0 {
		touchedKeys = append(touchedKeys, "grain")
	}
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
