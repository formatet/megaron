package economy

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func StoredProvisions(ctx context.Context, tx pgx.Tx, id uuid.UUID) (map[string]float64, error) {
	var total float64
	var goods map[string]float64
	if err := tx.QueryRow(ctx, `SELECT provisions,provision_goods FROM units WHERE id=$1 FOR UPDATE`, id).Scan(&total, &goods); err != nil {
		return nil, err
	}
	if math.Abs(FoodAmount(goods)-total) > 1e-7 {
		return nil, fmt.Errorf("provision ledger mismatch for %s", id)
	}
	if goods == nil {
		goods = map[string]float64{}
	}
	return goods, nil
}
func WriteProvisions(ctx context.Context, tx pgx.Tx, id uuid.UUID, goods map[string]float64) error {
	_, err := tx.Exec(ctx, `UPDATE units SET provisions=$2,provision_goods=$3,updated_at=now() WHERE id=$1`, id, FoodAmount(goods), goods)
	return err
}
func AddProvisions(ctx context.Context, tx pgx.Tx, id uuid.UUID, loaded map[string]float64) error {
	goods, err := StoredProvisions(ctx, tx, id)
	if err != nil {
		return err
	}
	for _, good := range FoodGoods {
		goods[good] += loaded[good]
	}
	return WriteProvisions(ctx, tx, id, goods)
}
func ReturnProvisions(ctx context.Context, tx pgx.Tx, id, sid uuid.UUID) error {
	goods, err := StoredProvisions(ctx, tx, id)
	if err != nil {
		return err
	}
	for _, good := range FoodGoods {
		if n := goods[good]; n > 0 {
			tag, err := tx.Exec(ctx, `SELECT credit_settlement_good($1,$2,$3,'returned_provisions')`, sid, good, n)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return fmt.Errorf("missing returned provision good %s", good)
			}
		}
	}
	return WriteProvisions(ctx, tx, id, map[string]float64{})
}

func EatProvisions(ctx context.Context, tx pgx.Tx, id uuid.UUID, need float64) (bool, error) {
	goods, err := StoredProvisions(ctx, tx, id)
	if err != nil {
		return false, err
	}
	meal := PlanFood(need, goods, false)
	if meal.Unmet > 0 {
		return false, nil
	}
	for good, n := range meal.Used {
		goods[good] = math.Max(0, goods[good]-n)
	}
	if err := WriteProvisions(ctx, tx, id, goods); err != nil {
		return false, err
	}
	return true, nil
}
