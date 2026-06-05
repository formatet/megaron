package economy

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// REF_LABOR is the reference population for the production formula.
// A settlement with exactly 100 free laborers and weight 1.0 on a good
// gets rate = base_potential (same as today's terrain-only rate).
const REF_LABOR = 100.0

// PopCosts mirrors province/training.go:UnitSpecs.PopCost.
// Defined here so economy stays Go-import-free upward (G1).
var PopCosts = map[string]int{
	"infantry":       5,
	"cavalry":        8,
	"catapult":       2,
	"priest":         3,
	"ship":           10,
	"elite_infantry": 10,
}

// Tx is the minimal interface accepted by RecomputeProduction so it can work
// with both pgx.Tx and pgxpool.Pool (the latter satisfies this interface too).
type Tx interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// RecomputeProduction settles and rewrites settlement_goods.rate for every
// producible good of the given settlement, using the labor-allocation formula:
//
//	labor_pool        = max(0, population − Σ(army_col × PopCost) − Σ(in_transit × PopCost))
//	base_potential(g) = SUM(production_rules matching terrain/deposits/buildings)
//	rate(g)           = base_potential(g) × weight(g) × labor_pool / REF_LABOR
//
// The unconditional timber trickle (NULL terrain, NULL building rule) is
// included in base_potential and therefore IS labor-scaled. However it is so
// small relative to the other contributions that the anti-deadlock property
// is preserved for any non-zero population.
//
// Must be called inside an existing transaction. Passes ctx to every DB call.
func RecomputeProduction(ctx context.Context, tx Tx, settlementID uuid.UUID) error {
	// ── 1. Compute labor_pool ────────────────────────────────────────────────
	var population int
	var infantry, cavalry, catapult, priest, ship, eliteInfantry int
	err := tx.QueryRow(ctx,
		`SELECT population, infantry, cavalry, catapult, priest, ship, elite_infantry
		 FROM settlements WHERE id = $1`,
		settlementID,
	).Scan(&population, &infantry, &cavalry, &catapult, &priest, &ship, &eliteInfantry)
	if err != nil {
		return fmt.Errorf("recompute: load settlement: %w", err)
	}

	homePop := infantry*PopCosts["infantry"] +
		cavalry*PopCosts["cavalry"] +
		catapult*PopCosts["catapult"] +
		priest*PopCosts["priest"] +
		ship*PopCosts["ship"] +
		eliteInfantry*PopCosts["elite_infantry"]

	// Count pop-cost of units currently in transit from this settlement.
	// marching_armies.origin_id is a province FK; resolved=false means in flight.
	var transitPop int
	_ = tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(
		     m.infantry       * $2 +
		     m.cavalry        * $3 +
		     m.catapult       * $4 +
		     m.priest         * $5 +
		     m.ship           * $6 +
		     m.elite_infantry * $7
		 ), 0)
		 FROM marching_armies m
		 JOIN settlements s ON s.province_id = m.origin_id
		 WHERE s.id = $1
		   AND m.resolved = false`,
		settlementID,
		PopCosts["infantry"], PopCosts["cavalry"], PopCosts["catapult"],
		PopCosts["priest"], PopCosts["ship"], PopCosts["elite_infantry"],
	).Scan(&transitPop)

	laborPool := population - homePop - transitPop
	if laborPool < 0 {
		laborPool = 0
	}

	// ── 2. Gather terrain/deposit info for this settlement ────────────────────
	var terrainType string
	var copperDeposit, tinDeposit, silverDeposit, cedarDeposit bool
	err = tx.QueryRow(ctx,
		`SELECT prov.terrain_type,
		        prov.copper_deposit, prov.tin_deposit,
		        COALESCE(prov.silver_deposit, false),
		        COALESCE(prov.cedar_deposit,  false)
		 FROM settlements s
		 JOIN provinces prov ON prov.id = s.province_id
		 WHERE s.id = $1`,
		settlementID,
	).Scan(&terrainType, &copperDeposit, &tinDeposit, &silverDeposit, &cedarDeposit)
	if err != nil {
		return fmt.Errorf("recompute: load terrain: %w", err)
	}

	// ── 3. Compute base_potential per producible good ─────────────────────────
	// A good is producible if at least one production_rule fires (terrain +
	// buildings + deposits). Aggregate via SUM so multiple rules sum up.
	rows, err := tx.Query(ctx,
		`SELECT pr.good_key, SUM(pr.rate_per_min) AS base_potential
		 FROM production_rules pr
		 WHERE
		     (pr.terrain_type IS NULL OR pr.terrain_type = $1)
		     AND (pr.building_type IS NULL OR EXISTS (
		             SELECT 1 FROM buildings b
		             WHERE b.settlement_id = $2 AND b.building_type = pr.building_type))
		     AND (pr.requires_deposit IS NULL
		          OR (pr.requires_deposit = 'copper' AND $3)
		          OR (pr.requires_deposit = 'tin'    AND $4)
		          OR (pr.requires_deposit = 'silver' AND $5)
		          OR (pr.requires_deposit = 'cedar'  AND $6))
		 GROUP BY pr.good_key`,
		terrainType, settlementID,
		copperDeposit, tinDeposit, silverDeposit, cedarDeposit,
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

	// ── 4. Load allocation weights for this settlement ────────────────────────
	wrows, err := tx.Query(ctx,
		`SELECT good_key, weight FROM settlement_labor WHERE settlement_id = $1`,
		settlementID,
	)
	if err != nil {
		return fmt.Errorf("recompute: load weights: %w", err)
	}
	weights := make(map[string]float64)
	for wrows.Next() {
		var key string
		var w float64
		if err := wrows.Scan(&key, &w); err != nil {
			wrows.Close()
			return fmt.Errorf("recompute: scan weight: %w", err)
		}
		weights[key] = w
	}
	wrows.Close()
	if err := wrows.Err(); err != nil {
		return fmt.Errorf("recompute: weight rows err: %w", err)
	}

	// If no weights exist yet, fall back to uniform across producible goods.
	if len(weights) == 0 {
		n := float64(len(potentials))
		for _, gp := range potentials {
			weights[gp.key] = 1.0 / n
		}
	}

	// ── 5. Settle and write new rates ─────────────────────────────────────────
	for _, gp := range potentials {
		w := weights[gp.key] // 0 if good not in labor table
		newRate := gp.basePotential * w * float64(laborPool) / REF_LABOR

		// Settle existing amount at old rate, then overwrite rate + calc_at.
		if _, err := tx.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
			 VALUES ($1, $2, 0, $3, $4, now())
			 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
			     amount  = LEAST(settlement_goods.cap,
			                 settlement_goods.amount +
			                 EXTRACT(EPOCH FROM (now()-settlement_goods.calc_at))/60 * settlement_goods.rate),
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
	default:
		return 200
	}
}
