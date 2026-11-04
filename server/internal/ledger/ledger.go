// Package ledger owns the booked daily count of settlement_goods (ekonomins
// sanning, Timothy 2026-11-04). The stock is the truth; the SQL trigger
// settlement_goods_ledger (mig 238) explains every change of amount in the same
// transaction as the write, under a category. A writer names its category with
// Reason; a write without one is booked as "unlabelled" and shows up as Other.
package ledger

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Retention is how many game days of ledger rows are kept.
const Retention = 14

// OtherEpsilon: smaller than this, Other is not shown.
const OtherEpsilon = 0.05

// Categories written by the trigger itself.
const (
	Production = "production"
	Overflow   = "overflow"
	Unlabelled = "unlabelled"
)

// TempleTithe is the category for one god's temple in the city.
func TempleTithe(god string) string { return "temple_tithe:" + god }

// Execer is a pgx.Tx or pool.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Reason names the category of the settlement_goods writes that follow in tx.
// It is transaction-local, so the caller must pass the tx doing the write.
func Reason(ctx context.Context, tx Execer, reason string) error {
	_, err := tx.Exec(ctx, `SELECT set_config('megaron.ledger_reason', $1, true)`, reason)
	return err
}

// ExecAs runs one statement on the pool inside its own transaction under a category,
// for writers that have no transaction of their own.
func ExecAs(ctx context.Context, pool *pgxpool.Pool, reason, sql string, args ...any) (pgconn.CommandTag, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	defer tx.Rollback(ctx)
	if err := Reason(ctx, tx, reason); err != nil {
		return pgconn.CommandTag{}, err
	}
	tag, err := tx.Exec(ctx, sql, args...)
	if err != nil {
		return tag, err
	}
	return tag, tx.Commit(ctx)
}

// CloseDay settles every producing row of the world, stores the closing stock and
// trims old rows. Exactly once per event (processed_tick_claims).
func CloseDay(ctx context.Context, pool *pgxpool.Pool, worldID uuid.UUID, eventID int64) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	scope := uuid.NewSHA1(worldID, []byte("ledger_close"))
	claim, err := tx.Exec(ctx, `INSERT INTO processed_tick_claims (event_id, scope_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, eventID, scope)
	if err != nil {
		return err
	}
	if claim.RowsAffected() == 0 {
		return nil
	}
	if err := Reason(ctx, tx, "settle_floor"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE settlement_goods g
		    SET amount = storage_capped(g.settlement_id,g.good_key,settled(g.amount,g.rate,g.calc_tick),g.cap,'production'),
		        calc_tick = current_world_tick()
		   FROM settlements s
		  WHERE g.settlement_id = s.id AND s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state <> 'sunk'
		    AND g.rate <> 0 AND g.calc_tick < current_world_tick()`, worldID); err != nil {
		return fmt.Errorf("ledger close settle: %w", err)
	}
	if err := Reason(ctx, tx, ""); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO settlement_ledger_close (settlement_id, tick, good_key, amount)
		 SELECT g.settlement_id, current_world_tick(), g.good_key, settled(g.amount,g.rate,g.calc_tick)
		   FROM settlement_goods g JOIN settlements s ON s.id = g.settlement_id
		  WHERE s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state <> 'sunk'
		 ON CONFLICT (settlement_id, tick, good_key) DO UPDATE SET amount = EXCLUDED.amount`, worldID); err != nil {
		return fmt.Errorf("ledger close snapshot: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM settlement_ledger WHERE tick < current_world_tick() - $1`, Retention); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM settlement_ledger_close WHERE tick < current_world_tick() - $1`, Retention); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Post is one line of a good's day.
type Post struct {
	Category string  `json:"category"`
	Amount   float64 `json:"amount"`
}

// GoodDay is one good's last closed game day.
type GoodDay struct {
	Good  string  `json:"good"`
	Delta float64 `json:"delta"` // closing − opening: the stock's own change
	Posts []Post  `json:"posts"`
	Other float64 `json:"other"` // Delta − Σ posts; shown only when |Other| ≥ OtherEpsilon
}

// Day is the last closed game day of one settlement. Known is false until the city
// has two closings (the first one has no opening to compare against).
type Day struct {
	Known bool      `json:"known"`
	Tick  int       `json:"tick"`
	Goods []GoodDay `json:"goods"`
}

type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// LastDay reads the last closed day. The ledger is the owner; nothing re-derives it.
func LastDay(ctx context.Context, q Querier, sid uuid.UUID) (Day, error) {
	var out Day
	rows, err := q.Query(ctx,
		`WITH last AS (SELECT max(tick) AS t FROM settlement_ledger_close WHERE settlement_id = $1),
		      prev AS (SELECT max(tick) AS t FROM settlement_ledger_close WHERE settlement_id = $1 AND tick < (SELECT t FROM last))
		 SELECT c.good_key, c.amount, COALESCE(o.amount, 0), (SELECT t FROM last), (SELECT t FROM prev)
		   FROM settlement_ledger_close c
		   LEFT JOIN settlement_ledger_close o ON o.settlement_id = c.settlement_id AND o.good_key = c.good_key AND o.tick = (SELECT t FROM prev)
		  WHERE c.settlement_id = $1 AND c.tick = (SELECT t FROM last)
		  ORDER BY c.good_key`, sid)
	if err != nil {
		return out, err
	}
	type gd struct {
		good       string
		closing, o float64
	}
	var gds []gd
	var last, prev *int
	for rows.Next() {
		var g gd
		var l, p *int
		if err := rows.Scan(&g.good, &g.closing, &g.o, &l, &p); err != nil {
			rows.Close()
			return out, err
		}
		last, prev = l, p
		gds = append(gds, g)
	}
	rows.Close()
	if rows.Err() != nil || last == nil || prev == nil {
		return out, rows.Err()
	}
	out.Known, out.Tick = true, *last
	prows, err := q.Query(ctx,
		`SELECT good_key, category, amount FROM settlement_ledger
		  WHERE settlement_id = $1 AND tick > $2 AND tick <= $3 ORDER BY good_key, category`, sid, *prev, *last)
	if err != nil {
		return out, err
	}
	posts := map[string][]Post{}
	for prows.Next() {
		var g string
		var p Post
		if err := prows.Scan(&g, &p.Category, &p.Amount); err != nil {
			prows.Close()
			return out, err
		}
		posts[g] = append(posts[g], p)
	}
	prows.Close()
	for _, g := range gds {
		d := GoodDay{Good: g.good, Delta: g.closing - g.o, Posts: posts[g.good]}
		sum := 0.0
		for _, p := range d.Posts {
			sum += p.Amount
		}
		d.Other = d.Delta - sum
		if math.Abs(d.Other) < OtherEpsilon {
			d.Other = 0
		}
		if len(d.Posts) == 0 && d.Other == 0 {
			continue
		}
		out.Goods = append(out.Goods, d)
	}
	sort.Slice(out.Goods, func(i, j int) bool { return out.Goods[i].Good < out.Goods[j].Good })
	return out, nil
}
