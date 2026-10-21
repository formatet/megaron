package economy

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"math"
)

// LoadProvisions tops up to need using food physically available at the caller-verified
// port. Caller owns the transaction, physical location and departure decision.
func LoadProvisions(ctx context.Context, tx pgx.Tx, ship, port uuid.UUID, need float64) error {
	goods, err := StoredProvisions(ctx, tx, ship)
	if err != nil {
		return err
	}
	missing := math.Max(0, need-FoodAmount(goods))
	if missing <= 1e-9 {
		return nil
	}
	meal, err := TakeFood(ctx, tx, port, missing, false, true)
	if err != nil {
		return err
	}
	if meal.Unmet > 1e-9 {
		return fmt.Errorf("not enough food in the departure port: voyage needs %.2f, available %.2f", need, need-meal.Unmet)
	}
	return AddProvisions(ctx, tx, ship, meal.Used)
}
