package economy

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// placementWriteTx serializes the read/check/write cycle on the settlement.
// An existing transaction keeps the lock until its caller commits. Pool callers
// (growth/pruning) get an owned transaction: a standalone FOR UPDATE on a pool
// would release the lock before the placement query and provide no protection.
func placementWriteTx(ctx context.Context, db Tx, id uuid.UUID) (pgx.Tx, func(error) error, int, error) {
	tx, external := db.(pgx.Tx)
	if !external {
		beginner, ok := db.(interface {
			Begin(context.Context) (pgx.Tx, error)
		})
		if !ok {
			return nil, nil, 0, fmt.Errorf("placement write requires a transaction or transaction-capable connection")
		}
		var err error
		tx, err = beginner.Begin(ctx)
		if err != nil {
			return nil, nil, 0, err
		}
	}
	var population int
	if err := tx.QueryRow(ctx, `SELECT population FROM settlements WHERE id=$1 FOR UPDATE`, id).Scan(&population); err != nil {
		if !external {
			_ = tx.Rollback(ctx)
		}
		return nil, nil, 0, fmt.Errorf("lock placement settlement: %w", err)
	}
	finish := func(err error) error {
		if external {
			return err
		}
		defer tx.Rollback(ctx)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	return tx, finish, population, nil
}
