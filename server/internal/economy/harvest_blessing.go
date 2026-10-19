package economy

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// The harvest blessing (Timothy 2026-10-10): the gods raise the yield of every
// food good except fish by HarvestBlessingYieldBonus for HarvestBlessingTicks.
// It replaced a one-shot ×1.25 on the grain STORE, which gave most to the city
// with the fullest food stock and nothing to the one that needed it.
const (
	HarvestBlessingYieldBonus = 0.25
	HarvestBlessingTicks      = 25
)

// HarvestBlessedGoods are the goods the blessing raises: FoodGoods minus fish, plus the established herd blessing
// (the sea is Poseidon's, not the fields').
func HarvestBlessedGoods() []string {
	out := make([]string, 0, len(FoodGoods))
	for _, g := range FoodGoods {
		if g != GoodFish {
			out = append(out, g)
		}
	}
	return append(out, GoodLivestock)
}

// ApplyHarvestBlessing blesses the settlement's harvest from now for
// HarvestBlessingTicks — a second blessing while one runs restarts the span,
// it does not stack — and recomputes production so the bonus shows at once.
// Returns the tick the blessing ends.
func ApplyHarvestBlessing(ctx context.Context, tx Tx, settlementID uuid.UUID) (untilTick int, err error) {
	if err := tx.QueryRow(ctx,
		`UPDATE settlements
		 SET harvest_blessing_until_tick = GREATEST(COALESCE(harvest_blessing_until_tick, 0), current_world_tick() + $2)
		 WHERE id = $1
		 RETURNING harvest_blessing_until_tick`,
		settlementID, HarvestBlessingTicks,
	).Scan(&untilTick); err != nil {
		return 0, fmt.Errorf("harvest blessing: %w", err)
	}
	if err := RecomputeProduction(ctx, tx, settlementID); err != nil {
		return 0, fmt.Errorf("harvest blessing: %w", err)
	}
	return untilTick, nil
}
