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
	cap     int // how many gubbar this slot can hold in this placement pass
	ordinal int
}

// rankedFoodSlots returns every grain/fish placement option in settlementID's
// catchment, highest marginal yield first (lowest hex-ordinal breaks ties —
// the same determinism P0-UI locked for the [unbuilt] spawn-to-food UI
// preview). Grain and fish now share the SAME real P3 hex cap
// (megaron_plan_grain_cap.md, 2026-08-22 — grain is no longer capacity-exempt);
// grain keeps its own yield SHAPE (marginal yield = rate, not rate/cap — see
// placementYield's doc comment), but the slot's cap field (how many gubbar it
// can hold before the greedy loop must move to the next-best hex) is the real
// physical ceiling, exactly like fish. Without this, greedy placement would
// stack every food-needing gubbe onto the single best-ranked grain hex before
// ever trying a second one — the founding-time version of the "32 gubbar on
// one hex" bug this whole plan exists to close.
func rankedFoodSlots(ctx context.Context, tx Tx, settlementID uuid.UUID) ([]foodSlot, hexgrid.Coord, error) {
	var q, r int
	if err := tx.QueryRow(ctx,
		`SELECT prov.map_q, prov.map_r
		 FROM settlements s JOIN provinces prov ON prov.id = s.province_id
		 WHERE s.id = $1`,
		settlementID,
	).Scan(&q, &r); err != nil {
		return nil, hexgrid.Coord{}, fmt.Errorf("ranked food slots: load settlement coords: %w", err)
	}
	center := hexgrid.Coord{Q: q, R: r}

	hexOptions, err := LoadHexProductionOptions(ctx, tx, settlementID, nil) // founding/growth placement previews the full catchment, siege denial does not apply
	if err != nil {
		return nil, center, fmt.Errorf("ranked food slots: %w", err)
	}

	var slots []foodSlot
	for _, opt := range hexOptions {
		for _, good := range []string{GoodGrain, "fish"} {
			rate := opt.RatePerGood[good]
			if rate <= 0 {
				continue
			}
			cap := opt.CapPerGood[good]
			if cap <= 0 {
				continue
			}
			yield := rate / float64(cap)
			if good == GoodGrain {
				yield = rate // grain's yield shape stays rate × placed, not rate/cap × placed (placementYield)
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
	return slots, center, nil
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
// rate PlaceStartingWorkforce leaves behind and fire a "grain running out"
// notification when it's negative, so no new warning channel is needed here.
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

	cumulative := guaranteedFood
	gubbeOrdinal := 1
	for _, slot := range slots {
		if placed >= totalGubbar || cumulative >= demand {
			break
		}
		for i := 0; i < slot.cap; i++ {
			if placed >= totalGubbar || cumulative >= demand {
				break
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO settlement_placement (settlement_id, gubbe_ordinal, target_kind, hex_q, hex_r, good_key)
				 VALUES ($1, $2, 'hex', $3, $4, $5)`,
				settlementID, gubbeOrdinal, slot.hex.Q, slot.hex.R, slot.good,
			); err != nil {
				return placed, false, fmt.Errorf("place starting workforce: insert gubbe %d: %w", gubbeOrdinal, err)
			}
			gubbeOrdinal++
			placed++
			cumulative += slot.yield
		}
	}

	return placed, cumulative >= demand, nil
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
// gubbeOrdinal must not collide with any existing placement (settlement_
// placement's UNIQUE(settlement_id, gubbe_ordinal)) — callers pass
// floor(population/100) at call time (the newly created gubbe's own ordinal).
// Falls through to the pool (no row inserted, placed=false) if every food
// slot is already at capacity — not an error.
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
		if _, err := tx.Exec(ctx,
			`INSERT INTO settlement_placement (settlement_id, gubbe_ordinal, target_kind, hex_q, hex_r, good_key)
			 VALUES ($1, $2, 'hex', $3, $4, $5)`,
			settlementID, gubbeOrdinal, slot.hex.Q, slot.hex.R, slot.good,
		); err != nil {
			return false, fmt.Errorf("place next gubbe %d: %w", gubbeOrdinal, err)
		}
		return true, nil
	}
	return false, nil // no room anywhere — falls to the pool
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
