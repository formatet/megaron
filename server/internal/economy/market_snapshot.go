package economy

import (
	"context"
	"log/slog"
	"time"

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
		            settled(sg.amount, sg.rate, sg.calc_tick))),
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

// propagatedMarketCap is how many of the teacher's most-recently-observed
// settlements a learner inherits knowledge of on a single contact.
const propagatedMarketCap = 5

// PropagateMarketKnowledge lets learnerID inherit sourceSettlementID's owner's
// (the "teacher") freshest market knowledge of OTHER settlements, when a
// messenger from learnerID reaches sourceSettlementID. This is the transitive
// discovery mechanism: a Wanax hears about a distant tin town through a
// contact, long before ever seeing it directly — the settlement's province is
// seeded into the learner's map memory (player_scouted_provinces) so it
// becomes visible/contactable, and its prices are copied in as secondhand
// market_snapshots rows.
//
// Firsthand knowledge (secondhand=false, i.e. observed directly) always wins:
// a secondhand row only overwrites an existing secondhand row, and only if
// the incoming observed_at is newer.
func PropagateMarketKnowledge(ctx context.Context, pool *pgxpool.Pool, learnerID, sourceSettlementID uuid.UUID) error {
	var teacherID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT owner_id FROM settlements WHERE id = $1`, sourceSettlementID,
	).Scan(&teacherID); err != nil {
		return err
	}
	if teacherID == nil || *teacherID == learnerID {
		return nil
	}

	rows, err := pool.Query(ctx,
		`WITH capped AS (
		     SELECT settlement_id, MAX(observed_at) AS latest
		     FROM market_snapshots
		     WHERE player_id = $1
		       AND settlement_id <> $2
		       AND settlement_id NOT IN (SELECT id FROM settlements WHERE owner_id = $3)
		     GROUP BY settlement_id
		     ORDER BY latest DESC
		     LIMIT $4
		 )
		 SELECT ms.settlement_id, s.province_id, ms.good_key, ms.stock, ms.price, ms.observed_at
		 FROM market_snapshots ms
		 JOIN settlements s ON s.id = ms.settlement_id
		 JOIN capped c ON c.settlement_id = ms.settlement_id
		 WHERE ms.player_id = $1
		   AND ms.observed_at > now() - interval '3 days'`,
		*teacherID, sourceSettlementID, learnerID, propagatedMarketCap,
	)
	if err != nil {
		return err
	}

	type learned struct {
		settlementID uuid.UUID
		provinceID   uuid.UUID
		goodKey      string
		stock        float64
		price        float64
		observedAt   time.Time
	}
	var items []learned
	for rows.Next() {
		var it learned
		if scanErr := rows.Scan(&it.settlementID, &it.provinceID, &it.goodKey, &it.stock, &it.price, &it.observedAt); scanErr == nil {
			items = append(items, it)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	seenProvinces := map[uuid.UUID]struct{}{}
	for _, it := range items {
		if _, err := tx.Exec(ctx,
			`INSERT INTO market_snapshots (player_id, settlement_id, good_key, stock, price, observed_at, secondhand)
			 VALUES ($1, $2, $3, $4, $5, $6, true)
			 ON CONFLICT (player_id, settlement_id, good_key) DO UPDATE SET
			     stock = EXCLUDED.stock, price = EXCLUDED.price, observed_at = EXCLUDED.observed_at, secondhand = true
			 WHERE market_snapshots.secondhand = true AND EXCLUDED.observed_at > market_snapshots.observed_at`,
			learnerID, it.settlementID, it.goodKey, it.stock, it.price, it.observedAt,
		); err != nil {
			slog.Error("secondhand market snapshot upsert", "err", err)
			continue
		}
		seenProvinces[it.provinceID] = struct{}{}
	}

	for provinceID := range seenProvinces {
		if _, err := tx.Exec(ctx,
			`INSERT INTO player_scouted_provinces (world_id, player_id, province_id)
			 SELECT p.world_id, $1, p.id FROM provinces p WHERE p.id = $2
			 ON CONFLICT DO NOTHING`,
			learnerID, provinceID,
		); err != nil {
			slog.Error("gossip: seed scouted province", "err", err)
		}
	}

	return tx.Commit(ctx)
}
