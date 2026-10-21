package messenger

import (
	"context"
	"fmt"
	"time"

	"formatet/megaron/server/internal/carrier"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func movementVerb(verb string) bool { return verb == "march" || verb == "redirect" || verb == "recall" }

// SupersedeUnboardedMovementTx physically calls back only own movement-order
// Runners waiting in an own port, with no carrier witness. They are already
// away from their sending city, so deleting the envelope would teleport them.
// The caller sends the replacement in this same transaction.
func SupersedeUnboardedMovementTx(ctx context.Context, tx pgx.Tx, sched *events.Scheduler, world, owner, unitID uuid.UUID, newVerb string, now time.Time, currentTick int) ([]uuid.UUID, error) {
	if !movementVerb(newVerb) || unitID == uuid.Nil {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `SELECT m.id FROM messengers m JOIN settlements port ON port.id=m.passage_port_id
 WHERE m.world_id=$1 AND m.sender_id=$2 AND m.kind='order' AND m.status='outbound'
 AND m.passage_status='awaiting_passage' AND port.owner_id=$2 AND port.state='active'
 AND m.carrier_transport_id IS NULL AND m.carrier_unit_id IS NULL AND m.carrier_witness_id=0
 AND NOT EXISTS(SELECT 1 FROM events e WHERE e.stream_id=m.id AND e.event_type IN `+carrier.TypesSQL+`)
 AND m.order_payload->>'unit_id'=$3 AND m.order_payload->>'verb' IN ('march','redirect','recall')
 ORDER BY m.id FOR UPDATE OF m`, world, owner, unitID.String())
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var superseded []uuid.UUID
	for _, id := range ids {
		result, err := CallBackTx(ctx, tx, sched, world, id, owner, now, currentTick)
		if err != nil {
			return nil, fmt.Errorf("supersede order %s: %w", id, err)
		}
		if result.Started {
			superseded = append(superseded, id)
		}
	}
	return superseded, nil
}
