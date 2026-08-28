// Command recompute-all runs economy.RecomputeProduction for every owned
// settlement in a world (or in every world).
//
// WHY THIS EXISTS: production_rules is a static catalogue, but a settlement's
// actual rates live in settlement_goods and are only rewritten when
// RecomputeProduction runs. Nothing in the tick loop calls it — it fires on
// events only (build, placement, founding, arrival, occupation, collapse,
// ship repair, training; see the callers of economy.RecomputeProduction).
//
// A settlement where nothing happens therefore keeps its old rates forever.
// That makes any migration that edits production_rules only HALF applied: the
// catalogue is new, every quiet city still produces at the old figures.
//
// Found the hard way with migration 136 (dagsverkesskalan, 2026-08-27): the
// migration rescaled existing rates correctly, but the NEW per-tick figures —
// farm bonus ×1.70, the city hex's grain ration dropping to exactly one
// gubbe's daily need — only reached a settlement once something happened to
// it. Mochlos sat at 114.82 grain/tick where the new rules say 82.40, and the
// two food-dead cities produced 1.16 where the rules say 0.50.
//
// Run this after any migration that touches production_rules.
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

	// Grain is the good worth reporting: it is the one every settlement has,
	// and the one whose drift is immediately legible as "does this city feed
	// itself". Read before and after so the operator sees the actual effect
	// rather than trusting a success count.
	grainRate := func(id uuid.UUID) float64 {
		var r float64
		_ = pool.QueryRow(ctx,
			`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'`,
			id).Scan(&r)
		return r
	}

	var changed, failed int
	for _, t := range targets {
		before := grainRate(t.id)
		if *dryRun {
			fmt.Printf("%-16s grain rate %8.2f  (dry run, not recomputed)\n", t.name, before)
			continue
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: begin: %v\n", t.name, err)
			failed++
			continue
		}
		if err := economy.RecomputeProduction(ctx, tx, t.id); err != nil {
			fmt.Fprintf(os.Stderr, "%s: recompute: %v\n", t.name, err)
			_ = tx.Rollback(ctx)
			failed++
			continue
		}
		if err := tx.Commit(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "%s: commit: %v\n", t.name, err)
			failed++
			continue
		}
		after := grainRate(t.id)
		mark := " "
		if diff := after - before; diff > 0.005 || diff < -0.005 {
			mark = "*"
			changed++
		}
		fmt.Printf("%s %-16s grain rate %8.2f → %8.2f\n", mark, t.name, before, after)
	}

	if *dryRun {
		fmt.Printf("\n%d settlements would be recomputed\n", len(targets))
		return
	}
	fmt.Printf("\n%d settlements recomputed, %d changed, %d failed\n", len(targets), changed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}
