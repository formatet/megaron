// scrub-coarse-names runs the coarse-word filter once over names already in
// the database (Timothy 2026-10-29: existing names are washed once when the
// filter goes live, without kharis or notice). Dry run is the default; an
// explicit -apply commits the single transaction.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"

	"formatet/megaron/server/internal/textfilter"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Every column a Wanax names something in.
var columns = []struct{ table, column string }{
	{"settlements", "name"},
	{"units", "name"},
	{"sea_storms", "name"},
	{"place_names", "name"},
	{"kingdoms", "name"},
}

func main() {
	apply := flag.Bool("apply", false, "commit the renames (default rolls back)")
	flag.Parse()
	if err := run(context.Background(), *apply); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, apply bool) error {
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	changed := 0
	for _, c := range columns {
		rows, err := tx.Query(ctx, `SELECT ctid::text, `+c.column+` FROM `+c.table+` WHERE `+c.column+` IS NOT NULL FOR UPDATE`)
		if err != nil {
			return fmt.Errorf("%s: %w", c.table, err)
		}
		type rename struct{ ctid, from, to string }
		var todo []rename
		for rows.Next() {
			var r rename
			if err := rows.Scan(&r.ctid, &r.from); err != nil {
				rows.Close()
				return err
			}
			if to, hits := textfilter.Name(r.from, rand.IntN); hits > 0 {
				r.to = to
				todo = append(todo, r)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, r := range todo {
			if _, err := tx.Exec(ctx, `UPDATE `+c.table+` SET `+c.column+` = $2 WHERE ctid = $1::tid`, r.ctid, r.to); err != nil {
				return fmt.Errorf("%s %q: %w", c.table, r.from, err)
			}
			fmt.Printf("%s.%s: %q → %q\n", c.table, c.column, r.from, r.to)
			changed++
		}
	}
	fmt.Printf("%d names changed\n", changed)
	if !apply {
		fmt.Println("dry run — rolled back (use -apply to commit)")
		return nil
	}
	return tx.Commit(ctx)
}
