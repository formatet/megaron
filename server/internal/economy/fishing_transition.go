package economy

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CompleteFishingRuleTransition applies final fishing capacity before workers
// or HTTP can see the new rules. All affected active-world cities are changed
// atomically, highest surplus ordinals go idle, and replay is a no-op.
func CompleteFishingRuleTransition(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := transitionFishingRules(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func transitionFishingRules(ctx context.Context, tx pgx.Tx) error {
	var done bool
	if err := tx.QueryRow(ctx, `SELECT completed FROM fishing_rule_transition WHERE id=true FOR UPDATE`).Scan(&done); err != nil {
		return err
	}
	if done {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT s.id FROM settlements s JOIN worlds w ON w.id=s.world_id
 WHERE w.status='active' AND s.owner_id IS NOT NULL AND s.state NOT IN ('sunk','collapsed','razed') ORDER BY s.id`)
	if err != nil {
		return err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		var locked uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM settlements WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
			return err
		}
		if _, err := liftHexPlacementsToCapacity(ctx, tx, id, []string{GoodFish}, nil, nil); err != nil {
			return fmt.Errorf("fishing trim %s: %w", id, err)
		}
		// Recompute settles each lazy stock at its saved old rate before writing the
		// new rate; trimming never changes the people's population or old balance.
		if err := RecomputeProduction(ctx, tx, id); err != nil {
			return fmt.Errorf("fishing recompute %s: %w", id, err)
		}
	}
	_, err = tx.Exec(ctx, `UPDATE fishing_rule_transition SET completed=true WHERE id=true`)
	return err
}
