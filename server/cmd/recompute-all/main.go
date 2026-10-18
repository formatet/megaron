// Command recompute-all runs economy.RecomputeProduction for every owned
// settlement in a world (or in every world).
//
// WHY THIS EXISTS: production_rules is a static catalogue, but a settlement's
// actual rates live in settlement_goods and are rewritten by
// RecomputeProduction. Kharis ticks and gameplay events recompute them too,
// but an operator can apply catalogue or formula changes immediately here.
// Dry-run executes that same path in a transaction and rolls it back.
//
// Found the hard way with migration 136 (dagsverkesskalan, 2026-08-27): the
// migration rescaled existing rates correctly, but the NEW per-tick figures —
// farm bonus ×1.70, the city hex's grain ration dropping to exactly one
// gubbe's daily need — only reached a settlement once something happened to
// it. Mochlos sat at 114.82 grain/tick where the new rules say 82.40, and the
// two food-dead cities produced 1.16 where the rules say 0.50.
//
// Run this after a production catalogue or formula change when stored rates
// must be updated immediately.
//
//	go run ./cmd/recompute-all                 # every world
//	go run ./cmd/recompute-all -world <uuid>   # one world
//	go run ./cmd/recompute-all -dry-run        # report, change nothing
//
// Safe to run repeatedly: RecomputeProduction is deterministic in the
// settlement's current state, so a second run over unchanged data is a no-op.
package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/jackc/pgx/v5"
	"math"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"formatet/megaron/server/internal/economy"
)

func main() {
	worldFlag := flag.String("world", "", "only this world id (default: every world)")
	dryRun := flag.Bool("dry-run", false, "report what would change, write nothing")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL not set")
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	query := `SELECT s.id, s.name FROM settlements s
	          WHERE s.owner_id IS NOT NULL AND s.state NOT IN ('sunk', 'collapsed')`
	args := []any{}
	if *worldFlag != "" {
		id, perr := uuid.Parse(*worldFlag)
		if perr != nil {
			fmt.Fprintf(os.Stderr, "bad -world uuid: %v\n", perr)
			os.Exit(1)
		}
		query += ` AND s.world_id = $1`
		args = append(args, id)
	}
	query += ` ORDER BY s.population DESC`

	type target struct {
		id   uuid.UUID
		name string
	}
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list settlements: %v\n", err)
		os.Exit(1)
	}
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.name); err != nil {
			rows.Close()
			fmt.Fprintf(os.Stderr, "scan: %v\n", err)
			os.Exit(1)
		}
		targets = append(targets, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "rows: %v\n", err)
		os.Exit(1)
	}

	var changed, failed int
	for _, t := range targets {
		result, err := recomputeSettlement(ctx, pool, t.id, *dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", t.name, err)
			failed++
			continue
		}
		mark := " "
		if math.Abs(result.After.Grain-result.Before.Grain) > 1e-9 || math.Abs(result.After.Fish-result.Before.Fish) > 1e-9 || result.Removed > 0 || result.Replaced > 0 {
			mark = "*"
			changed++
		}
		fmt.Printf("%s %-16s grain %8.4f → %8.4f; fish %8.4f → %8.4f; placements removed %d, replaced %d\n", mark, t.name, result.Before.Grain, result.After.Grain, result.Before.Fish, result.After.Fish, result.Removed, result.Replaced)
	}
	action := "recomputed"
	if *dryRun {
		action = "previewed (all transactions rolled back)"
	}
	fmt.Printf("\n%d settlements %s, %d changed, %d failed\n", len(targets)-failed, action, changed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

type foodRates struct{ Grain, Fish float64 }
type recomputeResult struct {
	Before, After     foodRates
	Removed, Replaced int
}

func readFoodRates(ctx context.Context, tx pgx.Tx, id uuid.UUID) (r foodRates, err error) {
	err = tx.QueryRow(ctx, `SELECT COALESCE(sum(rate) FILTER (WHERE good_key='grain'),0), COALESCE(sum(rate) FILTER (WHERE good_key='fish'),0) FROM settlement_goods WHERE settlement_id=$1`, id).Scan(&r.Grain, &r.Fish)
	return
}

// Dry-run executes exactly the same reconciliation/recompute path as apply,
// including lazy amounts, then rolls the entire city transaction back.
func recomputeSettlement(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, dryRun bool) (r recomputeResult, err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return r, err
	}
	defer tx.Rollback(ctx)
	var lockedID uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT id FROM settlements WHERE id=$1 FOR UPDATE`, id).Scan(&lockedID); err != nil {
		return r, err
	}
	if r.Before, err = readFoodRates(ctx, tx, id); err != nil {
		return r, err
	}
	if r.Removed, r.Replaced, err = economy.ReconcilePlacements(ctx, tx, id); err != nil {
		return r, err
	}
	if err = economy.RecomputeProduction(ctx, tx, id); err != nil {
		return r, err
	}
	if r.After, err = readFoodRates(ctx, tx, id); err != nil {
		return r, err
	}
	if dryRun {
		err = tx.Rollback(ctx)
	} else {
		err = tx.Commit(ctx)
	}
	return r, err
}
