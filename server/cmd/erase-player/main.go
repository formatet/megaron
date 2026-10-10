// Command erase-player erases a player's account on the admin's request
// (megaron_plan_kontoradering.md, Timothy 2026-10-10). Players cannot erase
// themselves; they write to the admin in the community chat.
//
// Erasure anonymises, it does not delete rows: 32 foreign keys point at
// players, and letters, battles and epitaphs belong to the other players too.
// The login becomes unusable, the player's private rows go, their cities are
// released as by abandon, and the Wanax name stays (it is the game's name, not
// the person's). The server's Agora loop deactivates the chat account.
//
//	erase-player <username>         show what would be erased
//	erase-player --yes <username>   erase it
//
// Run on the server: cd /opt/poleia/server && set -a && . ../.env && go run ./cmd/erase-player …
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	yes := flag.Bool("yes", false, "erase (without it, only show what would be erased)")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: erase-player [--yes] <username>")
		os.Exit(2)
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	rep, err := erasePlayer(ctx, pool, flag.Arg(0), *yes)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Player %s (Wanax %s)\n", flag.Arg(0), rep.wanax)
	fmt.Printf("  cities released: %d · host ended: %v · notifications: %d · bug reports: %d · chat account: %v\n",
		rep.cities, rep.host, rep.notifications, rep.reports, rep.agora)
	if !*yes {
		fmt.Println("Nothing changed. Run again with --yes to erase.")
		return
	}
	fmt.Printf("Erased. The login is now %s and cannot be used.\n", rep.newUsername)
	if rep.agora {
		fmt.Println("The chat account is deactivated by the server within a minute.")
	}
	if rep.reports > 0 {
		fmt.Printf("Copies of the bug reports remain in REPORTS_DIR/reports.jsonl and the vault's\n"+
			"megaron_buggrapporter_*.md; remove the lines naming %s by hand.\n", flag.Arg(0))
	}
}

type eraseReport struct {
	wanax, newUsername              string
	cities, notifications, reports int
	host, agora                     bool
}

var errAlreadyErased = errors.New("this account is already erased")

func erasePlayer(ctx context.Context, pool *pgxpool.Pool, username string, apply bool) (eraseReport, error) {
	var rep eraseReport
	tx, err := pool.Begin(ctx)
	if err != nil {
		return rep, err
	}
	defer tx.Rollback(ctx)

	var playerID uuid.UUID
	var erased bool
	var localpart *string
	if err := tx.QueryRow(ctx,
		`SELECT id, COALESCE(wanax_name, ''), erased_at IS NOT NULL, agora_localpart
		 FROM players WHERE username = $1 FOR UPDATE`,
		username,
	).Scan(&playerID, &rep.wanax, &erased, &localpart); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return rep, fmt.Errorf("no player with username %q", username)
		}
		return rep, err
	}
	if erased {
		return rep, errAlreadyErased
	}
	rep.agora = localpart != nil

	// Live cities: everything still held. Collapsed, sunk and abandoned are already gone.
	rows, err := tx.Query(ctx,
		`SELECT id, province_id, world_id, name FROM settlements
		 WHERE owner_id = $1 AND state NOT IN ('collapsed', 'sunk', 'abandoned') FOR UPDATE`,
		playerID)
	if err != nil {
		return rep, err
	}
	type city struct {
		id, province, world uuid.UUID
		name                string
	}
	var cities []city
	for rows.Next() {
		var c city
		if err := rows.Scan(&c.id, &c.province, &c.world, &c.name); err != nil {
			rows.Close()
			return rep, err
		}
		cities = append(cities, c)
	}
	rows.Close()
	rep.cities = len(cities)

	var hostID *uuid.UUID
	_ = tx.QueryRow(ctx,
		`SELECT host_unit_id FROM founder_phase WHERE owner_id = $1 AND active LIMIT 1`, playerID,
	).Scan(&hostID)
	rep.host = hostID != nil
	_ = tx.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE player_id = $1`, playerID).Scan(&rep.notifications)
	_ = tx.QueryRow(ctx, `SELECT count(*) FROM player_reports WHERE player_id = $1`, playerID).Scan(&rep.reports)

	if !apply {
		return rep, nil
	}

	// Release the cities as abandon does (api/handlers/settlement.go Abandon):
	// garrison and its embarked cargo disbanded, the hex free, the city ownerless.
	// Units in the field are left to the game: with no city to pay them they desert.
	for _, c := range cities {
		if _, err := tx.Exec(ctx,
			`UPDATE units SET status = 'disbanded', updated_at = now()
			 WHERE id IN (SELECT cargo_unit_id FROM units WHERE settlement_id = $1 AND status = 'garrison' AND cargo_unit_id IS NOT NULL)
			   AND status = 'embarked'`, c.id); err != nil {
			return rep, fmt.Errorf("disband cargo of %s: %w", c.name, err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE units SET status = 'disbanded', updated_at = now()
			 WHERE settlement_id = $1 AND status = 'garrison'`, c.id); err != nil {
			return rep, fmt.Errorf("disband garrison of %s: %w", c.name, err)
		}
		if _, err := tx.Exec(ctx, `UPDATE provinces SET territory_state = 'free' WHERE id = $1`, c.province); err != nil {
			return rep, fmt.Errorf("free province of %s: %w", c.name, err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET owner_id = NULL, kingdom_id = NULL, state = 'abandoned', updated_at = now()
			 WHERE id = $1`, c.id); err != nil {
			return rep, fmt.Errorf("release %s: %w", c.name, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO events (stream_id, stream_type, event_type, payload, world_id, world_tick)
			 VALUES ($1, 'province', 'SettlementAbandoned', jsonb_build_object('player_id', $2::text, 'name', $3::text), $4,
			         (SELECT current_tick FROM worlds WHERE id = $4))`,
			c.id, playerID.String(), c.name, c.world); err != nil {
			return rep, fmt.Errorf("record abandonment of %s: %w", c.name, err)
		}
	}
	if hostID != nil {
		if _, err := tx.Exec(ctx, `UPDATE units SET status = 'disbanded', updated_at = now() WHERE id = $1`, *hostID); err != nil {
			return rep, fmt.Errorf("disband host: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE founder_phase SET active = false WHERE owner_id = $1 AND active`, playerID); err != nil {
			return rep, fmt.Errorf("end founder phase: %w", err)
		}
	}
	if _, err := tx.Exec(ctx,
		`UPDATE player_world_records SET status = 'departed' WHERE player_id = $1 AND status IN ('active', 'dispossessed')`,
		playerID); err != nil {
		return rep, fmt.Errorf("mark departed: %w", err)
	}
	for _, table := range []string{"refresh_tokens", "notifications", "dispatch_mutes", "player_reports", "refusals"} {
		if _, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE player_id = $1`, playerID); err != nil {
			return rep, fmt.Errorf("clear %s: %w", table, err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM standing_orders WHERE owner_id = $1`, playerID); err != nil {
		return rep, fmt.Errorf("clear standing orders: %w", err)
	}
	// '!' is never a bcrypt hash, so no password can match it.
	if err := tx.QueryRow(ctx,
		`UPDATE players SET username = 'erased-' || substr(md5(id::text), 1, 8), password_hash = '!erased',
		     erased_at = now(), agora_deactivate_pending = (agora_localpart IS NOT NULL)
		 WHERE id = $1 RETURNING username`, playerID,
	).Scan(&rep.newUsername); err != nil {
		return rep, fmt.Errorf("anonymise login: %w", err)
	}
	return rep, tx.Commit(ctx)
}
