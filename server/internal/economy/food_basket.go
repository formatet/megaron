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
	// Lost is whole animals still above the herd cap after the meal: the pen
	// cannot hold them (order 28b). Zero unless the caller passed a cap.
	Lost float64
}

// HerdBaseCap is the herd a settlement holds without a pen (the founding herd
// fills it); every pen level adds HerdCapPerPenLevel. Alpha: the generous side.
const (
	HerdBaseCap        = 50
	HerdCapPerPenLevel = 100
)

// HerdCap is the most animals a settlement keeps. keryx and the web read it
// from the server's herd.cap field and never recompute it.
func HerdCap(penLevel int) float64 {
	if penLevel < 0 {
		penLevel = 0
	}
	return float64(HerdBaseCap + HerdCapPerPenLevel*penLevel)
}

// PenLevel is the settlement's pen building level, 0 when it has none.
func PenLevel(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, sid uuid.UUID) int {
	var lvl int
	_ = db.QueryRow(ctx, `SELECT COALESCE(MAX(level),0) FROM buildings WHERE settlement_id=$1 AND building_type='pen'`, sid).Scan(&lvl)
	return lvl
}

// PlanFood is the one ration split. Every ordinary food has equal value and is
// consumed in proportion to availability, so no commodity is exhausted first.
func PlanFood(demand float64, available map[string]float64, emergencyHerd bool) FoodMeal {
	return PlanFoodHerd(demand, available, emergencyHerd, math.Inf(1))
}

// PlanFoodHerd is PlanFood for a settlement whose herd has a cap (order 28b).
// Animals above herdCap are ordinary food: a fifth good in the equal pool.
// Animals under the cap are eaten only as emergency slaughter. Whatever is
// still above the cap after the meal is Lost.
func PlanFoodHerd(demand float64, available map[string]float64, emergencyHerd bool, herdCap float64) FoodMeal {
	out := FoodMeal{Used: map[string]float64{}, Unmet: math.Max(0, demand)}
	herd := math.Max(0, available[GoodLivestock])
	surplus := 0.0
	if !math.IsInf(herdCap, 1) {
		surplus = math.Max(0, herd-math.Max(0, herdCap))
	}
	total := FoodAmount(available) + surplus
	take := math.Min(out.Unmet, total)
	if total > 0 && take > 0 {
		for _, good := range FoodGoods {
			if n := math.Max(0, available[good]) * take / total; n > 0 {
				out.Used[good] = n
			}
		}
		if surplus > 0 {
			out.Used[GoodLivestock] += surplus * take / total
		}
		out.Unmet -= take
	}
	if out.Unmet < 1e-9 {
		out.Unmet = 0
	}
	if emergencyHerd && out.Unmet > 0 {
		animals := math.Min(math.Floor(herd-out.Used[GoodLivestock]), math.Ceil(out.Unmet/livestockFoodValue))
		if animals > 0 {
			out.Used[GoodLivestock] += animals
			out.Unmet = math.Max(0, out.Unmet-animals*livestockFoodValue)
		}
	}
	if !math.IsInf(herdCap, 1) {
		out.Lost = math.Floor(math.Max(0, herd-out.Used[GoodLivestock]-math.Max(0, herdCap)))
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
	return takeFood(ctx, tx, sid, demand, emergencyHerd, requireFull, false)
}

// TakeCivilianFood is the people's daily meal: emergency slaughter allowed, the
// herd capped by the settlement's pen, animals the pen cannot hold written off.
func TakeCivilianFood(ctx context.Context, tx pgx.Tx, sid uuid.UUID, demand float64) (FoodMeal, error) {
	return takeFood(ctx, tx, sid, demand, true, false, true)
}

func takeFood(ctx context.Context, tx pgx.Tx, sid uuid.UUID, demand float64, emergencyHerd, requireFull, capped bool) (FoodMeal, error) {
	stock, err := LoadFood(ctx, tx, sid, true)
	if err != nil {
		return FoodMeal{}, err
	}
	herdCap := math.Inf(1)
	if capped {
		herdCap = HerdCap(PenLevel(ctx, tx, sid))
	}
	meal := PlanFoodHerd(demand, stock, emergencyHerd, herdCap)
	if requireFull && meal.Unmet > 0 {
		return meal, nil
	}
	goods := append(append([]string{}, FoodGoods...), GoodLivestock)
	for _, good := range goods {
		qty := meal.Used[good]
		if good == GoodLivestock {
			qty += meal.Lost
		}
		if qty > 0 {
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
