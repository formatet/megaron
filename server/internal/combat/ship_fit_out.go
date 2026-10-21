package combat

import (
	"context"
	"encoding/json"
	"fmt"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FitOutTicks is the only duration setting for preparing a laid-up hull.
const FitOutTicks = 3

type ShipFitOutCompletePayload struct {
	UnitID  uuid.UUID `json:"unit_id"`
	WorldID uuid.UUID `json:"world_id"`
}

type ShipFitOutCompleteHandler struct{ pool *pgxpool.Pool }

func NewShipFitOutCompleteHandler(pool *pgxpool.Pool) *ShipFitOutCompleteHandler {
	return &ShipFitOutCompleteHandler{pool: pool}
}

// The status guard makes a retried scheduled event harmless. A captured ship
// retains its preparation timer and becomes ready for its current owner.
func (h *ShipFitOutCompleteHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p ShipFitOutCompletePayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("decode ship fit-out: %w", err)
	}
	_, err := h.pool.Exec(ctx, `UPDATE units SET status='garrison', build_complete_at=NULL, updated_at=now()
		WHERE id=$1 AND world_id=$2 AND status='fitting_out'`, p.UnitID, p.WorldID)
	return err
}
