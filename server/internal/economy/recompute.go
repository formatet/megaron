package economy

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// REF_LABOR is the reference population for the production formula.
// yield_per_worker(g) = base_potential(g) / REF_LABOR.
// A good produced by a settlement at REF_LABOR workers with all citizens
// assigned gets base_potential as its rate — same as the pre-Fas-2 baseline.
const REF_LABOR = 100.0

// PopCosts mirrors province/training.go:UnitSpecs.PopCost.
// Defined here so economy stays Go-import-free upward (G1).
// ship = galley (DB-kolumn). war_galley + merchantman = nya skepp-typer (mig 039).
// war_chariot ersatte cavalry/chariot (mig 042); catapult borttagen.
var PopCosts = map[string]int{
	"spearman":       5,
	"war_chariot":    8,
	"priest":         3,
	"ship":           10, // galley
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
	// W3: production comes from the 6 adjacent catchment tiles, not the own hex.
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
	// Each of the 6 adjacent map_tiles contributes based on its own terrain,
	// deposits, and coastal flag. The settlement's buildings gate building-gated
	// rules. Sea tiles (deep_sea, coastal_sea) are excluded — no land production.
	rows, err := tx.Query(ctx,
		`SELECT pr.good_key, SUM(pr.rate_per_min) AS base_potential
		 FROM map_tiles mt
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
		 WHERE mt.world_id = $2
		   AND mt.terrain NOT IN ('deep_sea', 'coastal_sea')
		   AND (
		       (mt.q = $3+1 AND mt.r = $4  ) OR (mt.q = $3-1 AND mt.r = $4  ) OR
		       (mt.q = $3   AND mt.r = $4+1) OR (mt.q = $3   AND mt.r = $4-1) OR
		       (mt.q = $3+1 AND mt.r = $4-1) OR (mt.q = $3-1 AND mt.r = $4+1)
		   )
		 GROUP BY pr.good_key`,
		settlementID, worldID, q, r,
	)
	if err != nil {
		return fmt.Errorf("recompute: query production rules: %w", err)
	}
	type goodPotential struct {
		key           string
		basePotential float64
	}
	var potentials []goodPotential
	for rows.Next() {
		var gp goodPotential
		if err := rows.Scan(&gp.key, &gp.basePotential); err != nil {
			rows.Close()
			return fmt.Errorf("recompute: scan potential: %w", err)
		}
		potentials = append(potentials, gp)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("recompute: rows err: %w", err)
	}

	if len(potentials) == 0 {
		// No producible goods — nothing to recompute.
		return nil
	}

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
	if len(weights) == 0 {
		n := len(potentials)
		if n == 0 {
			return nil
		}
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

	// ── 5. Settle and write new rates ─────────────────────────────────────────
	for _, gp := range potentials {
		effectiveWorkers := weights[gp.key] * float64(laborPool)
		yieldPerWorker := gp.basePotential / REF_LABOR
		newRate := yieldPerWorker * effectiveWorkers

		// Settle existing amount at old rate, then overwrite rate + calc_at.
		if _, err := tx.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
			 VALUES ($1, $2, 0, $3, $4, now())
			 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
			     amount  = LEAST(settlement_goods.cap,
			                 settled(settlement_goods.amount, settlement_goods.rate, settlement_goods.calc_at)),
			     rate    = $3,
			     calc_at = now()`,
			settlementID, gp.key, newRate, goodCap(gp.key),
		); err != nil {
			return fmt.Errorf("recompute: upsert good %s: %w", gp.key, err)
		}
	}

	return nil
}

// goodCap returns the storage cap for a good key (mirrors join.go caps).
func goodCap(key string) float64 {
	switch key {
	case "grain":
		return 1000
	case "timber", "cedar":
		return 500
	case "stone":
		return 1000
	case "copper", "tin":
		return 300
	case "pottery":
		return 500
	case "cult":
		return 2000
	default:
		return 200
	}
}
