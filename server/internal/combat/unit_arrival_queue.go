package combat

import (
	"context"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The unit lock must be held. An identical arrival resolves the unit's current
// course at that tick, so a replacement course can reuse the pending job.
func enqueueUnitArrivalTx(ctx context.Context, tx pgx.Tx, scheduler *events.Scheduler, worldID uuid.UUID, payload unit.ScheduledUnitArrivalPayload, due int) error {
	return scheduler.EnsurePendingTickTx(ctx, tx, worldID, events.ScheduledUnitArrival, payload, due)
}
