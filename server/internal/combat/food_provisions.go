package combat

import (
	"context"
	"formatet/megaron/server/internal/economy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func storedProvisions(ctx context.Context, tx pgx.Tx, id uuid.UUID) (map[string]float64, error) {
	return economy.StoredProvisions(ctx, tx, id)
}
func addProvisions(ctx context.Context, tx pgx.Tx, id uuid.UUID, goods map[string]float64) error {
	return economy.AddProvisions(ctx, tx, id, goods)
}
func returnProvisions(ctx context.Context, tx pgx.Tx, id, sid uuid.UUID) error {
	return economy.ReturnProvisions(ctx, tx, id, sid)
}
func (h *UpkeepHandler) eatProvisions(ctx context.Context, id uuid.UUID, need float64) (bool, error) {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	fed, err := economy.EatProvisions(ctx, tx, id, need)
	if err != nil || !fed {
		return fed, err
	}
	return true, tx.Commit(ctx)
}
