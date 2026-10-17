package province

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// GoodsCostEpsilon tolerates floating-point residue when paying goods costs.
// It is absolute (not proportional to the stock or cost).
const GoodsCostEpsilon = 1e-6

// GoodsAffordable is shared by cost validation and capability projections.
// Subtraction preserves a real shortfall even at very large stock values.
func GoodsAffordable(have, need float64) bool {
	return have >= need || need-have <= GoodsCostEpsilon
}

// DeductGood checks and pays one exact cost atomically. Only a floating-point
// residue may be forgiven; accepted payments settle to at least zero.
func DeductGood(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID, good string, qty float64) (pgconn.CommandTag, error) {
	return tx.Exec(ctx, `UPDATE settlement_goods SET
 amount = GREATEST(0, settled(amount, rate, calc_tick) - $1),
 calc_tick = current_world_tick()
 WHERE settlement_id = $2 AND good_key = $3
 AND (settled(amount, rate, calc_tick) >= $1
      OR $1 - settled(amount, rate, calc_tick) <= $4)`,
		qty, settlementID, good, GoodsCostEpsilon)
}
