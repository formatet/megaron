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
// chariot ersatte cavalry (mig 042); catapult borttagen.
var PopCosts = map[string]int{
	"infantry":       5,
	"chariot":        8,
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
//	labor_pool        = max(0, population − Σ(army_col × PopCost) − Σ(in_transit × PopCost))
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
	var population int
	var infantry, chariot, priest, ship, eliteInfantry, warGalley, merchantman int
	err := tx.QueryRow(ctx,
		`SELECT population, infantry, chariot, priest, ship, elite_infantry, war_galley, merchantman
		 FROM settlements WHERE id = $1`,
		settlementID,
	).Scan(&population, &infantry, &chariot, &priest, &ship, &eliteInfantry, &warGalley, &merchantman)
	if err != nil {
		return fmt.Errorf("recompute: load settlement: %w", err)
	}

	homePop := infantry*PopCosts["infantry"] +
		chariot*PopCosts["chariot"] +
		priest*PopCosts["priest"] +
		ship*PopCosts["ship"] +
		eliteInfantry*PopCosts["elite_infantry"] +
		warGalley*PopCosts["war_galley"] +
		merchantman*PopCosts["merchantman"]

	// Count pop-cost of units currently in transit from this settlement.
	// marching_armies.origin_id is a province FK; resolved=false means in flight.
	var transitPop int
	_ = tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(
		     m.infantry       * $2 +
		     m.chariot        * $3 +
		     m.priest         * $4 +
		     m.ship           * $5 +
		     m.elite_infantry * $6 +
		     m.war_galley     * $7 +
		     m.merchantman    * $8
		 ), 0)
		 FROM marching_armies m
		 JOIN settlements s ON s.province_id = m.origin_id
		 WHERE s.id = $1
		   AND m.resolved = false`,
		settlementID,
		PopCosts["infantry"], PopCosts["chariot"],
		PopCosts["priest"], PopCosts["ship"], PopCosts["elite_infantry"],
		PopCosts["war_galley"], PopCosts["merchantman"],
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

	// ── 4. Load citizen allocations for this settlement ───────────────────────
	crows, err := tx.Query(ctx,
		`SELECT good_key, citizens FROM settlement_labor WHERE settlement_id = $1`,
		settlementID,
	)
	if err != nil {
		return fmt.Errorf("recompute: load citizens: %w", err)
	}
	citizens := make(map[string]int)
	for crows.Next() {
		var key string
		var c int
		if err := crows.Scan(&key, &c); err != nil {
			crows.Close()
			return fmt.Errorf("recompute: scan citizens: %w", err)
		}
		citizens[key] = c
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return fmt.Errorf("recompute: citizens rows err: %w", err)
	}

	// Fyll citizens om tabellen är tom (första gången — t.ex. vid join).
	// Om rader redan finns, rör dem INTE: en ny producerbar vara (t.ex. fisk efter hamn)
	// visas i Goods-endpointen med citizens=0 och producible=true — Wanax tilldelar
	// själv via LaborAlloc. Ingen automatisk tilldelning av 1 citizen (fisk-buggen).
	if len(citizens) == 0 {
		// Första gången: fördela labor_pool jämnt.
		n := len(potentials)
		if n == 0 {
			return nil
		}
		perGood := laborPool / n
		if perGood < 1 {
			perGood = 1
		}
		for _, gp := range potentials {
			citizens[gp.key] = perGood
			if _, err := tx.Exec(ctx,
				`INSERT INTO settlement_labor (settlement_id, good_key, citizens)
				 VALUES ($1, $2, $3)
				 ON CONFLICT (settlement_id, good_key) DO NOTHING`,
				settlementID, gp.key, perGood,
			); err != nil {
				return fmt.Errorf("recompute: seed citizens %s: %w", gp.key, err)
			}
		}
	}
	// Nya producerbara varor (gp.key saknas i citizens) behandlas med 0 citizens —
	// rate blir 0 tills Wanax allokerar via LaborAlloc. settlement_goods-raden
	// skapas ändå (upsert nedan) med rate=0 så varan syns i Goods-endpointen.

	// ── 5. Settle and write new rates ─────────────────────────────────────────
	for _, gp := range potentials {
		c := citizens[gp.key] // 0 if good not allocated
		yieldPerWorker := gp.basePotential / REF_LABOR
		newRate := yieldPerWorker * float64(c)

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
