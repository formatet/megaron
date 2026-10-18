package economy

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
)

// DebitArmyGrain feeds a unit from its already-resolved paying city's grain,
// then that SAME city's Sitos grain. Fish never pays army upkeep. The debit
// consumes food directly, so no temporary credit can overflow the city cap.
// Failed affordability changes neither source. Caller owns per-unit tick claims
// and records the returned reserve source in the upkeep outcome.
func DebitArmyGrain(ctx context.Context, pool *pgxpool.Pool, sid uuid.UUID, need float64) (paid bool, fromSitos float64, err error) {
	if need <= 0 {
		return true, 0, nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, 0, err
	}
	defer tx.Rollback(ctx)
	var locked uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT id FROM settlements WHERE id=$1 FOR UPDATE`, sid).Scan(&locked); err != nil {
		return false, 0, fmt.Errorf("army grain: lock city: %w", err)
	}
	var city, reserve float64
	err = tx.QueryRow(ctx, `SELECT GREATEST(0,settled(amount,rate,calc_tick)) FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain' FOR UPDATE`, sid).Scan(&city)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	err = tx.QueryRow(ctx, `SELECT amount FROM settlement_granary WHERE settlement_id=$1 AND good_key='grain' FOR UPDATE`, sid).Scan(&reserve)
	if errors.Is(err, pgx.ErrNoRows) {
		reserve = 0
	} else if err != nil {
		return false, 0, err
	}
	if city+reserve < need {
		return false, 0, nil
	}
	fromSitos = math.Max(0, need-city)
	if _, err = tx.Exec(ctx, `UPDATE settlement_goods SET amount=$2,calc_tick=current_world_tick() WHERE settlement_id=$1 AND good_key='grain'`, sid, math.Max(0, city-need)); err != nil {
		return false, 0, err
	}
	if fromSitos > 0 {
		if _, err = tx.Exec(ctx, `UPDATE settlement_granary SET amount=amount-$2 WHERE settlement_id=$1 AND good_key='grain'`, sid, fromSitos); err != nil {
			return false, 0, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return false, 0, err
	}
	return true, fromSitos, nil
}
