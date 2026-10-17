package religion

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Execer is the DB surface the index needs — satisfied by *pgxpool.Pool and
// pgx.Tx alike. Same shape as economy.DBTX (recompute.go:59); consumer-side
// interface per the G1 rule.
type Execer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// RecomputeDivineValuations rebuilds valuation version 2 in worker ticks.
//
// Runs ONCE PER DAY on the tick, never per rite: a prayer must not pay for a
// world-wide scan. The result lands in divine_labor_valuations and is read by both
// consumers — a prayer's odds and the standing cult's flow.
//
// The measure deliberately counts only ACTIVE settlements and distinct OWNERS.
// Razed ruins hold no wealth the world can trade for, and counting settlements
// instead of owners would let one Wanax with five cities look like five holders
// — which at 14 players would swing the gods' taste every time a colony is founded.
// The tick stamp comes from the DB's own current_world_tick() rather than a
// caller-supplied number, so the price list can never be stamped with a clock
// the world does not share.
func RecomputeDivineValuations(ctx context.Context, db Execer, worldID uuid.UUID) error {
	// Holder threshold is per-good (goodDivisors, see valuation.go) since mig 136
	// rescaled each good's stored amount by a different factor — a flat
	// holderMinStock would silently miscount small-but-real stocks as "no
	// holding" for every good mig 136 touched. Sent down as two parallel arrays
	// and unnested into a per-good threshold table so the SQL never hand-copies
	// the divisor numbers; goodDivisors stays the one source. Goods absent from
	// the arrays (silver, bronze) fall back to holderMinStock via COALESCE.
	goodKeys := make([]string, 0, len(goodDivisors))
	goodThresholds := make([]float64, 0, len(goodDivisors))
	for key, divisor := range goodDivisors {
		goodKeys = append(goodKeys, key)
		goodThresholds = append(goodThresholds, holderMinStock/divisor)
	}

	// One pass: per good, how many distinct owners hold a real stock of it, and
	// how much of it exists in the world. settled() projects the lazy tuple so a
	// good that has been accruing since its last write is counted honestly.
	rows, err := db.Query(ctx,
		`WITH stock AS (
		     SELECT sg.good_key,
		            s.owner_id,
		            GREATEST(0, settled(sg.amount, sg.rate, sg.calc_tick)) AS amount
		     FROM settlement_goods sg
		     JOIN settlements s ON s.id = sg.settlement_id
		     WHERE s.world_id = $1 AND s.state = 'active' AND s.owner_id IS NOT NULL
		 ),
		 threshold AS (
		     SELECT * FROM unnest($3::text[], $4::float8[]) AS t(good_key, min_stock)
		 )
		 SELECT g.key,
		        g.labor_value,
		        COALESCE(COUNT(DISTINCT stock.owner_id) FILTER (
		            WHERE stock.amount >= COALESCE(threshold.min_stock, $2)
		        ), 0) AS holders,
		        COALESCE(SUM(stock.amount), 0) AS world_stock
		 FROM goods g
		 LEFT JOIN stock ON stock.good_key = g.key
		 LEFT JOIN threshold ON threshold.good_key = g.key
		 WHERE g.labor_value IS NOT NULL AND g.status = 'active'
		 GROUP BY g.key, g.labor_value`,
		worldID, holderMinStock, goodKeys, goodThresholds)
	if err != nil {
		return fmt.Errorf("load world stock: %w", err)
	}

	type goodStat struct {
		key        string
		baseValue  float64
		holders    int
		worldStock float64
	}
	var stats []goodStat
	var maxStock float64
	for rows.Next() {
		var g goodStat
		if err := rows.Scan(&g.key, &g.baseValue, &g.holders, &g.worldStock); err != nil {
			rows.Close()
			return fmt.Errorf("scan world stock: %w", err)
		}
		if g.worldStock > maxStock {
			maxStock = g.worldStock
		}
		stats = append(stats, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("world stock rows: %w", err)
	}

	// Total owners in the world — the denominator for spread. Counting owners
	// with at least one active settlement, so a wiped-out Wanax does not make
	// every good look monopolised.
	var totalOwners int
	ownerRows, err := db.Query(ctx,
		`SELECT COUNT(DISTINCT owner_id) FROM settlements
		 WHERE world_id = $1 AND state = 'active' AND owner_id IS NOT NULL`, worldID)
	if err != nil {
		return fmt.Errorf("count owners: %w", err)
	}
	if ownerRows.Next() {
		_ = ownerRows.Scan(&totalOwners)
	}
	ownerRows.Close()

	for _, g := range stats {
		rarity := Rarity(g.holders, totalOwners, g.worldStock, maxStock)
		computed := DivineValue(g.baseValue, rarity)

		// Smoothing reads the previous value inside the UPSERT so the whole
		// recompute stays one statement per good and cannot half-apply.
		if _, err := db.Exec(ctx,
			`INSERT INTO divine_labor_valuations
			   (world_id, good_key, rarity_spread, rarity_volume, divine_value, calc_tick)
			 VALUES ($1, $2, $3, $4, $5, current_world_tick())
			 ON CONFLICT (world_id, good_key) DO UPDATE SET
			   rarity_spread = EXCLUDED.rarity_spread,
			   rarity_volume = EXCLUDED.rarity_volume,
			   divine_value  = CASE
			       WHEN divine_labor_valuations.divine_value <= 0 THEN EXCLUDED.divine_value
			       ELSE divine_labor_valuations.divine_value * (1 - $6::float8) + EXCLUDED.divine_value * $6::float8
			   END,
			   calc_tick = EXCLUDED.calc_tick`,
			worldID, g.key, rarity.Spread, rarity.Volume, computed, smoothing,
		); err != nil {
			return fmt.Errorf("upsert divine valuation %s: %w", g.key, err)
		}
	}
	return nil
}

// LoadDivineValues reads a world's divine price list as good_key→value.
func LoadDivineValues(ctx context.Context, db Execer, worldID uuid.UUID) (map[string]float64, error) {
	rows, err := db.Query(ctx,
		`SELECT good_key, divine_value FROM divine_labor_valuations WHERE world_id = $1`, worldID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[string]float64{}
	for rows.Next() {
		var key string
		var v float64
		if err := rows.Scan(&key, &v); err != nil {
			return nil, err
		}
		values[key] = v
	}
	return values, rows.Err()
}

// LaborValueUnit labels the internal measure in persistent event payloads; the temple presents offering values.
const LaborValueUnit = "worker_ticks"
const LaborValuationVersion = 2

// LoadLaborValues is the shared reference used by the index and both cult consumers.
func LoadLaborValues(ctx context.Context, db Execer) (map[string]float64, error) {
	rows, err := db.Query(ctx, `SELECT key,labor_value FROM goods WHERE labor_value IS NOT NULL AND status='active'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[string]float64{}
	for rows.Next() {
		var key string
		var value float64
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, rows.Err()
}

// InitializeLaborValuations runs before serving requests. Existing v2 history
// is retained on restarts; the historical v1 table never supplies a seed value.
func InitializeLaborValuations(ctx context.Context, db interface {
	Execer
	Begin(context.Context) (pgx.Tx, error)
}, worldID uuid.UUID) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Serialize the first initialization for this world. The whole index appears
	// atomically; a failed boot cannot leave a partial history to smooth later.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, worldID.String()); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT EXISTS(SELECT 1 FROM goods g WHERE g.labor_value IS NOT NULL
 AND g.status='active' AND NOT EXISTS(SELECT 1 FROM divine_labor_valuations dv
 WHERE dv.world_id=$1 AND dv.good_key=g.key))`, worldID)
	if err != nil {
		return err
	}
	var missing bool
	if rows.Next() {
		err = rows.Scan(&missing)
	}
	rows.Close()
	if err != nil {
		return err
	}
	if missing {
		if err := RecomputeDivineValuations(ctx, tx, worldID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
