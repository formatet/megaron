package economy

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"math"
)

// HospitalityFood loads at most the return reserve requested by a caller that
// has proved a physical port visit. The town retains one full civilian meal;
// guests share the same four-food basket, never its herd or silver. Repeating
// the same visit only fills a remaining deficit, not another ration.
func HospitalityFood(ctx context.Context, tx pgx.Tx, ship, port uuid.UUID, needed float64) error {
	goods, err := StoredProvisions(ctx, tx, ship)
	if err != nil {
		return err
	}
	missing := math.Max(0, needed-FoodAmount(goods))
	if missing <= 1e-9 {
		return nil
	}
	var pop int
	if err = tx.QueryRow(ctx, `SELECT population FROM settlements WHERE id=$1 AND state='active' FOR UPDATE`, port).Scan(&pop); err == pgx.ErrNoRows {
		return nil
	} else if err != nil {
		return err
	}
	stock, err := LoadFood(ctx, tx, port, true)
	if err != nil {
		return err
	}
	available := math.Max(0, FoodAmount(stock)-GrainConsumptionPerTick(pop))
	take := math.Min(missing, available)
	if take <= 1e-9 {
		return nil
	}
	meal, err := TakeFood(ctx, tx, port, take, false, false)
	if err != nil {
		return err
	}
	if meal.Unmet > 1e-7 {
		return fmt.Errorf("hospitality food changed while locked")
	}
	return AddProvisions(ctx, tx, ship, meal.Used)
}
