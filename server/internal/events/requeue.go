package events

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// RequeueByPayload moves the still-pending jobs of one mover (payload key = id,
// e.g. transport_id) to a new deadline, in place. Used when a journey is
// shortened under way (bönesystemet v3 steg 2b): the job is the same job, so
// there is never an old one left to run at the wrong time. Only unprocessed,
// unfailed jobs of the listed types move. Returns how many moved.
func RequeueByPayload(ctx context.Context, tx pgx.Tx, key, id string, types []ScheduledEventType, due int, at time.Time) (int64, error) {
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = string(t)
	}
	tag, err := tx.Exec(ctx, `UPDATE scheduled_events SET due_tick=$4, process_after=$5
 WHERE payload->>$1=$2 AND event_type=ANY($3) AND processed_at IS NULL AND failed_at IS NULL`, key, id, names, due, at)
	return tag.RowsAffected(), err
}

// TransportJobTypes are every pending job that ends a transport's journey.
var TransportJobTypes = []ScheduledEventType{
	ScheduledTransportArrival, ScheduledTransportArrivalV2, ScheduledTradeDelivery, ScheduledTradeDeliveryV2,
	ScheduledGiftDelivery, ScheduledGiftDeliveryV2, ScheduledShipmentDeliveryV2, ScheduledShipmentDeliveryV3,
	ScheduledTradeReturn, ScheduledTradeReturnV2,
}

// MessengerJobTypes are the jobs that end a Runner's leg.
var MessengerJobTypes = []ScheduledEventType{
	ScheduledMessengerArrival, ScheduledMessengerArrivalV2, ScheduledMessengerReturn, ScheduledMessengerReturnV2,
	ScheduledOrderDelivery, ScheduledOrderDeliveryV2,
}
