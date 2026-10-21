package combat

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The unit lock must be held. An identical arrival resolves the unit's current
// course at that tick, so a replacement course can reuse the pending job.
func enqueueUnitArrivalTx(ctx context.Context, tx pgx.Tx, scheduler *events.Scheduler, worldID uuid.UUID, payload unit.ScheduledUnitArrivalPayload, due int) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var queued bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM scheduled_events WHERE world_id=$1 AND event_type=$2 AND due_tick=$3 AND payload=$4::jsonb AND processed_at IS NULL AND failed_at IS NULL)`, worldID, string(events.ScheduledUnitArrival), due, raw).Scan(&queued); err != nil {
		return err
	}
	if queued {
		return nil
	}
	return scheduler.EnqueueTickTx(ctx, tx, worldID, events.ScheduledUnitArrival, payload, due)
}
