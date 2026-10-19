package economy

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// VarietyMinShare is the minimum share of an actual meal for a commodity to
// count toward growth variety. Proportional crumbs still feed people.
const VarietyMinShare = 0.10

// FoodMeal is an outcome in physical goods. Emergency slaughter remains a
// whole-animal civilian fallback; other consumers use only FoodGoods (order28a).
type FoodMeal struct {
	Used  map[string]float64
	Unmet float64
}

// PlanFood is the one ration split. Every ordinary food has equal value and is
// consumed in proportion to availability, so no commodity is exhausted first.
func PlanFood(demand float64, available map[string]float64, emergencyHerd bool) FoodMeal {
	out := FoodMeal{Used: map[string]float64{}, Unmet: math.Max(0, demand)}
	total := FoodAmount(available)
	take := math.Min(out.Unmet, total)
	if total > 0 && take > 0 {
		for _, good := range FoodGoods {
			if n := math.Max(0, available[good]) * take / total; n > 0 {
				out.Used[good] = n
			}
		}
		out.Unmet -= take
	}
	if out.Unmet < 1e-9 {
		out.Unmet = 0
	}
	if emergencyHerd && out.Unmet > 0 {
		animals := math.Min(math.Floor(math.Max(0, available[GoodLivestock])), math.Ceil(out.Unmet/livestockFoodValue))
		if animals > 0 {
			out.Used[GoodLivestock] = animals
			out.Unmet = math.Max(0, out.Unmet-animals*livestockFoodValue)
		}
	}
	return out
}

func FoodAmount(goods map[string]float64) float64 {
	total := 0.0
	for _, good := range FoodGoods {
		total += math.Max(0, goods[good])
	}
	return total
}

type foodReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// LoadFood reads settled stocks, which already include production through the
// current tick. The mutation path locks the same rows in a stable commodity order.
func LoadFood(ctx context.Context, db foodReader, sid uuid.UUID, lock bool) (map[string]float64, error) {
	goods := append(append([]string{}, FoodGoods...), GoodLivestock)
	query := `SELECT good_key,GREATEST(0,settled(amount,rate,calc_tick)) FROM settlement_goods WHERE settlement_id=$1 AND good_key=ANY($2) ORDER BY good_key`
	if lock {
		query += ` FOR UPDATE`
	}
	rows, err := db.Query(ctx, query, sid, goods)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stock := map[string]float64{}
	for rows.Next() {
		var good string
		var qty float64
		if err := rows.Scan(&good, &qty); err != nil {
			return nil, err
		}
		stock[good] = qty
	}
	return stock, rows.Err()
}

// TakeFood plans and applies a meal inside the caller's transaction. A required
// full ration never partially debits on refusal. It does not invent missing rows.
func TakeFood(ctx context.Context, tx pgx.Tx, sid uuid.UUID, demand float64, emergencyHerd, requireFull bool) (FoodMeal, error) {
	stock, err := LoadFood(ctx, tx, sid, true)
	if err != nil {
		return FoodMeal{}, err
	}
	meal := PlanFood(demand, stock, emergencyHerd)
	if requireFull && meal.Unmet > 0 {
		return meal, nil
	}
	goods := append(append([]string{}, FoodGoods...), GoodLivestock)
	for _, good := range goods {
		if qty := meal.Used[good]; qty > 0 {
			tag, err := tx.Exec(ctx, `UPDATE settlement_goods SET amount=GREATEST(0,settled(amount,rate,calc_tick)-$3),calc_tick=current_world_tick() WHERE settlement_id=$1 AND good_key=$2`, sid, good, qty)
			if err != nil {
				return FoodMeal{}, err
			}
			if tag.RowsAffected() != 1 {
				return FoodMeal{}, fmt.Errorf("missing consumed good %s", good)
			}
		}
	}
	return meal, nil
}
