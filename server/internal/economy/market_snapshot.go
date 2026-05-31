package economy

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RecordMarketSnapshot records a player's observed view of a settlement's goods.
// Called when a caravan delivers to, or a messenger arrives at, a settlement.
// Upserts all goods for the settlement atomically (single observed_at).
func RecordMarketSnapshot(ctx context.Context, pool *pgxpool.Pool, playerID, settlementID uuid.UUID) error {
	rows, err := pool.Query(ctx,
		`SELECT sg.good_key, g.base_value,
		        GREATEST(0, LEAST(sg.cap,
		            sg.amount + (EXTRACT(EPOCH FROM (now()-sg.calc_at))/60 * sg.rate))),
		        sg.rate,
		        sg.cap
		 FROM settlement_goods sg
		 JOIN goods g ON g.key = sg.good_key
		 WHERE sg.settlement_id = $1`,
		settlementID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	type snap struct {
		goodKey string
		stock   float64
		price   float64
	}
	var snaps []snap
	for rows.Next() {
		var goodKey string
		var baseValue, stock, rate, cap float64
		if err := rows.Scan(&goodKey, &baseValue, &stock, &rate, &cap); err != nil {
			continue
		}
		snaps = append(snaps, snap{goodKey: goodKey, stock: stock, price: LocalPrice(baseValue, stock, rate, cap)})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(snaps) == 0 {
		return nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, s := range snaps {
		if _, err := tx.Exec(ctx,
			`INSERT INTO market_snapshots (player_id, settlement_id, good_key, stock, price, observed_at)
			 VALUES ($1, $2, $3, $4, $5, now())
			 ON CONFLICT (player_id, settlement_id, good_key) DO UPDATE SET
			     stock = EXCLUDED.stock, price = EXCLUDED.price, observed_at = EXCLUDED.observed_at`,
			playerID, settlementID, s.goodKey, s.stock, s.price,
		); err != nil {
			slog.Error("market snapshot upsert", "err", err)
		}
	}
	return tx.Commit(ctx)
}
