package economy

import (
	"context"
	"fmt"
	"sort"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// foodSlot is one placeable (hex, food-good) option, ranked for greedy
// placement — shared by PlaceStartingWorkforce (founding) and
// PlaceNextGubbeOnBestFoodHex (growth), the two P4 auto-placement entries
// (megaron_plan_fysisk_gubbemodell.md P0-UI answer 5).
type foodSlot struct {
	hex     hexgrid.Coord
	good    string
	yield   float64
	cap     int // PlaceCapPerGood — capL1 for fish, the real level-actual cap for grain (see placement_yield.go)
	ordinal int
}

// rankedFoodSlots returns every grain/fish placement option in settlementID's
// catchment, highest marginal yield first (lowest hex-ordinal breaks ties —
// the same determinism P0-UI locked for the [unbuilt] spawn-to-food UI
// preview). Grain and fish now share the SAME real P3 hex cap
// (megaron_plan_grain_cap.md, 2026-08-22 — grain is no longer capacity-exempt);
// grain keeps its own yield SHAPE (marginal yield = rate, not rate/capL1×mult —
// see placementYield's doc comment), but the slot's cap field (how many
// gubbar it can hold before the greedy loop must move to the next-best hex)
// is the real physical ceiling, exactly like fish. Without this, greedy
// placement would stack every food-needing gubbe onto the single best-ranked
// grain hex before ever trying a second one — the founding-time version of
// the "32 gubbar on one hex" bug this whole plan exists to close.
//
// Form B (megaron_plan_byggnadsniva_takt.md, 2026-08-24): the slot's cap is
// now PlaceCapPerGood, not CapPerGood — Place() enforces PlaceCapPerGood at
// write time (settlement_placement.go), so this greedy pass must stop
// filling a hex at the same number or it would try to insert rows Place()
// would reject. Grain's PlaceCapPerGood stays the real level-actual cap
// (unaffected by Form B); fish's becomes capL1 (frozen).
func rankedFoodSlots(ctx context.Context, tx Tx, settlementID uuid.UUID) ([]foodSlot, hexgrid.Coord, error) {
	slots, center, _, err := rankedFoodSlotsWithOptions(ctx, tx, settlementID)
	return slots, center, err
}

// rankedFoodSlotsWithOptions is rankedFoodSlots' fuller sibling: the same
// ranked food-slot list PLUS the raw hexOptions it was built from — grain's
// base potential (the population-remainder term, see FoodGubbarRequired)
// lives there and nowhere else. rankedFoodSlots is now a thin wrapper around
// this; its two existing callers (PlaceStartingWorkforce,
// PlaceNextGubbeOnBestFoodHex) are unchanged.
func rankedFoodSlotsWithOptions(ctx context.Context, tx Tx, settlementID uuid.UUID) ([]foodSlot, hexgrid.Coord, []HexOption, error) {
	var q, r int
	if err := tx.QueryRow(ctx,
		`SELECT prov.map_q, prov.map_r
		 FROM settlements s JOIN provinces prov ON prov.id = s.province_id
		 WHERE s.id = $1`,
		settlementID,
	).Scan(&q, &r); err != nil {
		return nil, hexgrid.Coord{}, nil, fmt.Errorf("ranked food slots: load settlement coords: %w", err)
	}
	center := hexgrid.Coord{Q: q, R: r}

	hexOptions, err := LoadHexProductionOptions(ctx, tx, settlementID, nil) // founding/growth placement previews the full catchment, siege denial does not apply
	if err != nil {
		return nil, center, nil, fmt.Errorf("ranked food slots: %w", err)
	}
	return rankSlotsFromOptions(hexOptions, center), center, hexOptions, nil
}

// rankedFoodSlotsAt is rankedFoodSlots' settlement-free sibling — the same
// ranking over an explicit (worldID, center, buildingLevels) triple instead
// of a settlementID, so FoundingGrainNetPerTick can rank a hypothetical
// catchment before any settlement row exists
// (megaron_plan_grundningsprognosen.md §3). reachable is the FOW gate (see
// LoadHexProductionOptionsAt) — nil for the ordinary unfiltered catchment.
func rankedFoodSlotsAt(ctx context.Context, tx Tx, worldID uuid.UUID, center hexgrid.Coord, buildingLevels map[string]int, reachable map[hexgrid.Coord]bool) ([]foodSlot, error) {
	hexOptions, err := LoadHexProductionOptionsAt(ctx, tx, worldID, center, buildingLevels, reachable)
	if err != nil {
		return nil, fmt.Errorf("ranked food slots at: %w", err)
	}
	return rankSlotsFromOptions(hexOptions, center), nil
}

// rankSlotsFromOptions turns a catchment's per-hex production menu into the
// ranked (highest marginal yield first, lowest ring-ordinal breaks ties)
// grain/fish placement list — shared by rankedFoodSlots (settlement-scoped)
// and rankedFoodSlotsAt (forecast-scoped), so both rank the SAME way off the
// SAME HexOption shape.
func rankSlotsFromOptions(hexOptions []HexOption, center hexgrid.Coord) []foodSlot {
	var slots []foodSlot
	for _, opt := range hexOptions {
		for _, good := range []string{GoodGrain, "fish"} {
			rate := opt.RatePerGood[good]
			if rate <= 0 {
				continue
			}
			capL1 := opt.CapL1PerGood[good]
			cap := opt.PlaceCapPerGood[good]
			if capL1 <= 0 || cap <= 0 {
				continue
			}
			yield := (rate / float64(capL1)) * opt.MultPerGood[good]
			if good == GoodGrain {
				yield = rate // grain's yield shape stays rate × placed, not rate/capL1×mult × placed (placementYield)
			}
			ordinal, _ := hexgrid.RingOrdinal(center, hexgrid.CatchmentRadius, opt.Coord)
			slots = append(slots, foodSlot{opt.Coord, good, yield, cap, ordinal})
		}
	}
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].yield != slots[j].yield {
			return slots[i].yield > slots[j].yield
		}
		return slots[i].ordinal < slots[j].ordinal
	})
	return slots
}

// PlaceStartingWorkforce auto-places a newly founded settlement's starting
// gubbar on food (grain/fish) hexes, greedily by marginal yield, stopping the
// moment the city is self-sufficient — NOT placing every gubbe on food by
// default. Timothy 2026-08-08, resolving the collision between P4's "en
// oplacerad gubbe producerar ingenting" (Temenos_varutaxonomi_sol.md §1.1)
// and the pre-existing "a neglected new city must never starve" invariant
// (TestApplyDecay_GrainFundedGrowth_MinimalCitySelfSufficient):
//
//	"wanaxen måste logga in och grunda staden med sin host, och då tänker jag
//	att det automatiskt placeras ut gubbar att arbeta på de mest produktiva
//	mathexarna till den grad att staden försörjs matmässigt, om det kräver
//	5/10 placeras de - om det kräver 9/10 placeras de. Om staden behöver 10/10
//	och ändå inte försörjer sig ska de placeras men spelare måste varnas."
//
// A city needing only 5 of its 10 starting gubbar to feed itself keeps 5 in
// the pool for the Wanax to assign elsewhere — activation is preserved for
// the majority of the workforce, survival is guaranteed for all of it. If
// even every gubbe on food isn't enough, all are placed anyway (best effort)
// and sufficient=false tells the caller to warn — both founding call sites
// (found_metropolis.go, unit_arrival.go foundColony) already read the grain
// rate PlaceStartingWorkforce leaves behind, net it against population via
// economy.GrainBalance (D6, Utfodringsordningen — the raw rate itself can no
// longer go negative since D1), and fire a "grain running out" notification
// when THAT nets negative, so no new warning channel is needed here.
//
// Must run BEFORE RecomputeProduction (which derives rates from whatever
// this function placed) and inside the same transaction as founding, so a
// failed founding never leaves orphaned placement rows.
func PlaceStartingWorkforce(ctx context.Context, tx Tx, settlementID uuid.UUID) (placed int, sufficient bool, err error) {
	var population int
	if err := tx.QueryRow(ctx,
		`SELECT population FROM settlements WHERE id = $1`, settlementID,
	).Scan(&population); err != nil {
		return 0, false, fmt.Errorf("place starting workforce: load population: %w", err)
	}
	totalGubbar := population / 100

	slots, _, err := rankedFoodSlots(ctx, tx, settlementID)
	if err != nil {
		return 0, false, err
	}

	// Guaranteed food regardless of placement (RecomputeProduction step 4b):
	// nearjord's flat trickle + the population remainder auto-farming grain.
	hexOptions, err := LoadHexProductionOptions(ctx, tx, settlementID, nil) // founding/growth placement previews the full catchment, siege denial does not apply
	if err != nil {
		return 0, false, fmt.Errorf("place starting workforce: %w", err)
	}
	grainBasePotential := 0.0
	for _, opt := range hexOptions {
		grainBasePotential += opt.RatePerGood[GoodGrain]
	}
	remainderCitizens := population % 100
	guaranteedFood := NearjordGrainPerTick + (grainBasePotential/REF_LABOR)*float64(remainderCitizens)
	demand := GrainConsumptionPerTick(population)

	placements, sufficient := placeGreedyOnFoodSlots(slots, guaranteedFood, demand, totalGubbar)

	gubbeOrdinal := 1
	for _, p := range placements {
		if _, err := tx.Exec(ctx,
			`INSERT INTO settlement_placement (settlement_id, gubbe_ordinal, target_kind, hex_q, hex_r, good_key)
			 VALUES ($1, $2, 'hex', $3, $4, $5)`,
			settlementID, gubbeOrdinal, p.hex.Q, p.hex.R, p.good,
		); err != nil {
			return gubbeOrdinal - 1, false, fmt.Errorf("place starting workforce: insert gubbe %d: %w", gubbeOrdinal, err)
		}
		gubbeOrdinal++
	}

	return len(placements), sufficient, nil
}

// gubbePlacement is one greedy-placed gubbe's target and yield — the return
// shape of placeGreedyOnFoodSlots.
type gubbePlacement struct {
	hex   hexgrid.Coord
	good  string
	yield float64
}

// placeGreedyOnFoodSlots is PlaceStartingWorkforce's placement DECISION as a
// pure function, with no DB access — the same greedy-until-sufficient
// algorithm, extracted so FoundingGrainNetPerTick can run it against a
// hypothetical catchment (no settlement, nothing to write) and get the exact
// list a real founding would produce (megaron_plan_grundningsprognosen.md
// §3: "en formel, två anrop" — PlaceStartingWorkforce INSERTs this list,
// FoundingGrainNetPerTick sums it). Stops the moment either the city is
// self-sufficient (cumulative >= demand) or the workforce runs out (placed
// >= totalGubbar) — see PlaceStartingWorkforce's doc comment for the full
// rationale (Timothy 2026-08-08).
func placeGreedyOnFoodSlots(slots []foodSlot, guaranteedFood, demand float64, totalGubbar int) (placements []gubbePlacement, sufficient bool) {
	cumulative := guaranteedFood
	for _, slot := range slots {
		if len(placements) >= totalGubbar || cumulative >= demand {
			break
		}
		for i := 0; i < slot.cap; i++ {
			if len(placements) >= totalGubbar || cumulative >= demand {
				break
			}
			placements = append(placements, gubbePlacement{slot.hex, slot.good, slot.yield})
			cumulative += slot.yield
		}
	}
	return placements, cumulative >= demand
}

// FoodGubbarRequired answers P4's version of break-even: how many gubbar
// must stand on the catchment's food (grain/fish) slots for the settlement's
// OWN production to cover the population's daily ration — the same question
// the old pre-P4 weight-based figure tried to answer in a weight
// settlement_labor no longer reads (megaron_plan_p4_arvet_i_province.md §2). Runs EXACTLY the
// same two calls PlaceStartingWorkforce and FoundingGrainNetPerTick already
// share (rankedFoodSlots + placeGreedyOnFoodSlots) — a formula, three
// callers, never a second one.
//
// achievable=false means the catchment cannot feed the population even with
// EVERY gubbe placed on food (Gournia/Zakros in drift: no grain or fish
// terrain at all). placeGreedyOnFoodSlots alone does not guarantee
// len(placements)==totalGubbar in that exact case (an empty slot list simply
// never enters its loop, leaving placements at 0) — so required is forced to
// totalGubbar (pop/100) whenever achievable is false, per §2's contract and
// the "matdöd stad" acceptance test (§4.4): the caller must be told the WHOLE
// workforce is needed (and still not enough), never left with a silent 0.
func FoodGubbarRequired(ctx context.Context, tx Tx, settlementID uuid.UUID) (required int, achievable bool, err error) {
	slots, _, hexOptions, err := rankedFoodSlotsWithOptions(ctx, tx, settlementID)
	if err != nil {
		return 0, false, err
	}

	var population int
	if err := tx.QueryRow(ctx,
		`SELECT population FROM settlements WHERE id = $1`, settlementID,
	).Scan(&population); err != nil {
		return 0, false, fmt.Errorf("food gubbar required: load population: %w", err)
	}
	totalGubbar := population / 100
	remainderCitizens := population % 100

	grainBasePotential := 0.0
	for _, opt := range hexOptions {
		grainBasePotential += opt.RatePerGood[GoodGrain]
	}
	// ⚠️ The ONE place outside recompute.go step 4b (and FoundingGrainNetPerTick,
	// its forecast-time twin) where REF_LABOR still governs output after P4 —
	// the population remainder's old aggregate term, the SAME expression, not
	// a second one.
	guaranteedFood := NearjordGrainPerTick + (grainBasePotential/REF_LABOR)*float64(remainderCitizens)
	demand := GrainConsumptionPerTick(population)

	placements, sufficient := placeGreedyOnFoodSlots(slots, guaranteedFood, demand, totalGubbar)

	required = len(placements)
	if !sufficient {
		required = totalGubbar
	}
	return required, sufficient, nil
}

// PlaceNextGubbeOnBestFoodHex auto-places ONE newly born gubbe (population
// crossing a new full hundred, §1.1) on the catchment's best-yielding food
// slot with room left — P0-UI answer 5 (LÅST 2026-08-07): "Den nyfödda
// gubben placeras på den avslöjade catchment-hex med högst marginellt
// fisk-/grainutbyte som har ledig kapacitet; finns ingen sådan → ledig-pool."
// Unconditional (unlike PlaceStartingWorkforce): places even when the city is
// already self-sufficient — this exists to fix the "1-gubbe city crossing
// 199→200 loses its 99 invisible auto-farmers" cliff, not to gate on need.
//
// KNOWN SIMPLIFICATION: does not apply the FOW "avslöjad" (revealed) gate —
// kharis may not import province (G1: kharis→ai,clock,economy,events,
// hexgrid,religion,unit), so a rigorous fog check isn't reachable from the
// tick handler that calls this. A Wanax's own catchment is very often already
// scouted near the city centre; a growth-gubbe landing on an unrevealed outer
// ring hex is the residual risk, not yet closed. The HTTP placement path
// (task 4/5, player-initiated) DOES check FOW, at the handler layer.
//
// gubbeOrdinal is caller-supplied (callers pass floor(population/100) at
// call time) and is NOT guaranteed free: PrunePlacementsToPopulation
// (megaron_plan_placeringsbeskarning.md) can strand a surviving placement's
// ordinal below a settlement's current gubbar count when population later
// regrows through it, since the raw oldGubbar+1..newGubbar loop
// (kharis/tick.go, settlement_placement.go SlaughterLivestock) doesn't check
// which ordinals a prune left occupied. The INSERT is therefore ON CONFLICT
// DO NOTHING on settlement_placement's UNIQUE(settlement_id, gubbe_ordinal) —
// a collision is treated exactly like "every food slot full": placed=false,
// not an error, the gubbe falls to the pool for the Wanax to place by hand.
// Falls through to the pool the same way if every food slot is already at
// capacity.
func PlaceNextGubbeOnBestFoodHex(ctx context.Context, tx Tx, settlementID uuid.UUID, gubbeOrdinal int) (placed bool, err error) {
	slots, _, err := rankedFoodSlots(ctx, tx, settlementID)
	if err != nil {
		return false, err
	}

	placedCounts, err := LoadPlacementCounts(ctx, tx, settlementID)
	if err != nil {
		return false, fmt.Errorf("place next gubbe: %w", err)
	}

	for _, slot := range slots {
		occupied := placedCounts.Hex[slot.hex][slot.good]
		if occupied >= slot.cap {
			continue // full — grain's real P3 cap now binds here too, same as fish (megaron_plan_grain_cap.md)
		}
		tag, err := tx.Exec(ctx,
			`INSERT INTO settlement_placement (settlement_id, gubbe_ordinal, target_kind, hex_q, hex_r, good_key)
			 VALUES ($1, $2, 'hex', $3, $4, $5)
			 ON CONFLICT (settlement_id, gubbe_ordinal) DO NOTHING`,
			settlementID, gubbeOrdinal, slot.hex.Q, slot.hex.R, slot.good,
		)
		if err != nil {
			return false, fmt.Errorf("place next gubbe %d: %w", gubbeOrdinal, err)
		}
		return tag.RowsAffected() > 0, nil
	}
	return false, nil // no room anywhere — falls to the pool
}

// PrunePlacementsToPopulation enforces Föda S1b's invariant
// count(settlement_placement) ≤ floor(population/100). Placement tracks UP
// on growth (PlaceNextGubbeOnBestFoodHex) but nothing tracked it DOWN, so a
// settlement that grew large and later shrank (starvation, battle loss,
// occupation) kept phantom placements — measured 2026-08-25 on CT126: up to
// 300 placements at 3999 population (target 39). Removes rows in S1b's
// LOCKED order — förädling first, then övrig icke-mat, then tempel, food
// LAST. row_number() ranks every row ascending by (tier, gubbe_ordinal), so
// food sits at the LOWEST rn (survives even when the target is tiny) and
// förädling sits at the HIGHEST rn (the first to fall once rn exceeds the
// target); within a tier the lowest gubbe_ordinal survives longest, so the
// most recently placed gubbe of that tier goes first. `rn > target` then
// deletes exactly that tail in ONE DELETE — converges in a single pass, not
// one row per tick, so an already-broken city self-heals the next time this
// runs instead of needing hundreds of ticks. Idempotent when already within
// the cap (returns 0, no-op). Never touches settlement_labor (cult
// devotion, KH1) — this DELETE only ever targets settlement_placement.
// Caller must run RecomputeProduction afterwards in the same tick.
func PrunePlacementsToPopulation(ctx context.Context, tx Tx, settlementID uuid.UUID) (pruned int, err error) {
	tag, err := tx.Exec(ctx,
		`DELETE FROM settlement_placement
		 WHERE id IN (
		     SELECT id FROM (
		         SELECT id, row_number() OVER (
		             ORDER BY CASE good_key
		                 WHEN $2 THEN 1 WHEN $3 THEN 1 WHEN $4 THEN 1  -- grain/fish/livestock: mat SIST → survives longest
		                 WHEN $5 THEN 2                                -- cult: tempel (never a real row today)
		                 WHEN $6 THEN 4 WHEN $7 THEN 4 WHEN $8 THEN 4  -- bronze/oil/wine: hantverk/förädling → removed first
		                 ELSE 3                                        -- övrig icke-mat (stone/timber/cedar/silver/copper/tin/…)
		             END,
		             gubbe_ordinal  -- within a tier, lowest ordinal survives longest
		         ) AS rn
		         FROM settlement_placement WHERE settlement_id = $1
		     ) ranked
		     WHERE ranked.rn > (SELECT population / 100 FROM settlements WHERE id = $1)
		 )`,
		settlementID, GoodGrain, GoodFish, GoodLivestock, GoodCult, GoodBronze, GoodOil, GoodWine,
	)
	if err != nil {
		return 0, fmt.Errorf("prune placements to population: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// BackfillPlacements auto-places workforce for every settlement in a world
// that has population but zero settlement_placement rows — settlements that
// existed before P4 landed (settlement_labor's weights are simply never read
// for non-cult goods anymore, so those settlements would otherwise sit at 0
// production for anything but nearjord + the population remainder until a
// Wanax manually visits the [not yet built] P5 stadsvy). Idempotent: a
// settlement that already has any placement rows is skipped untouched, so
// this is safe to re-run (e.g. after a later reseed adds new settlements).
// Not automatic on boot — explicit, operator-triggered, matches how reseeds
// and restarts already work (megaron_drift.md §Deploy).
//
// Runs PlaceStartingWorkforce (same greedy-until-sufficient algorithm as a
// real founding) followed by RecomputeProduction per settlement, each in its
// own transaction — one settlement's failure doesn't block the rest.
func BackfillPlacements(ctx context.Context, pool *pgxpool.Pool, worldID uuid.UUID) (backfilled int, err error) {
	rows, err := pool.Query(ctx,
		`SELECT s.id FROM settlements s
		 WHERE s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state NOT IN ('sunk','collapsed')
		   AND s.population > 0
		   AND NOT EXISTS (SELECT 1 FROM settlement_placement sp WHERE sp.settlement_id = s.id)`,
		worldID,
	)
	if err != nil {
		return 0, fmt.Errorf("backfill placements: query settlements: %w", err)
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("backfill placements: scan: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("backfill placements: rows: %w", err)
	}

	for _, id := range ids {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return backfilled, fmt.Errorf("backfill placements: begin tx for %s: %w", id, err)
		}
		if _, _, err := PlaceStartingWorkforce(ctx, tx, id); err != nil {
			tx.Rollback(ctx)
			return backfilled, fmt.Errorf("backfill placements: place %s: %w", id, err)
		}
		if err := RecomputeProduction(ctx, tx, id); err != nil {
			tx.Rollback(ctx)
			return backfilled, fmt.Errorf("backfill placements: recompute %s: %w", id, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return backfilled, fmt.Errorf("backfill placements: commit %s: %w", id, err)
		}
		backfilled++
	}
	return backfilled, nil
}
