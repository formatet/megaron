package combat

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The unit lock must be held. An identical arrival resolves the unit's current
// course at that tick, so a replacement course can reuse the pending job.
func enqueueUnitArrivalTx(ctx context.Context, tx pgx.Tx, scheduler *events.Scheduler, worldID uuid.UUID, payload unit.ScheduledUnitArrivalPayload, due int) error {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT march_route FROM units WHERE id=$1`, payload.UnitID).Scan(&raw); err != nil {
		return err
	}
	var r StoredRoute
	if json.Unmarshal(raw, &r) == nil && r.Version == 2 {
		a, err := tick.LoadAnchor(ctx, tx, worldID)
		if err != nil {
			return err
		}
		end := int64(r.EndTick)*1000 + r.OffsetMilli
		arrival := a.WallAt(end)
		departure := a.WallAt(int64(r.StartTick)*1000 + r.OffsetMilli)
		if _, err = tx.Exec(ctx, `UPDATE units SET departs_at=$2,arrives_at=$3 WHERE id=$1`, payload.UnitID, departure, arrival); err != nil {
			return err
		}
		body, err := json.Marshal(arrivalV2Payload{ScheduledUnitArrivalPayload: payload, RouteKey: fmt.Sprintf("%x", sha256.Sum256(raw))})
		if err != nil {
			return err
		}
		oldBody, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		// The new course atomically replaces an identical pending legacy envelope.
		// Different envelopes (including other due ticks) retain their own jobs.
		tag, err := tx.Exec(ctx, `UPDATE scheduled_events SET event_type=$5,payload=$6,process_after=$7 WHERE id=(SELECT id FROM scheduled_events WHERE world_id=$1 AND event_type=$2 AND due_tick=$3 AND payload=$4::jsonb AND processed_at IS NULL AND failed_at IS NULL ORDER BY id LIMIT 1 FOR UPDATE)`, worldID, string(events.ScheduledUnitArrival), due, oldBody, string(events.ScheduledUnitArrivalV2), body, arrival)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			return nil
		}

		_, err = tx.Exec(ctx, `INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,priority) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, worldID, string(events.ScheduledUnitArrivalV2), body, arrival, int(end/1000), events.TickPriority(events.ScheduledUnitArrivalV2))
		return err
	}
	return scheduler.EnsurePendingTickTx(ctx, tx, worldID, events.ScheduledUnitArrival, payload, due)
}
