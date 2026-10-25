package economy

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"time"
)

// ClockTradeJourney stamps each actual dispatch, including chained returns.
func ClockTradeJourney(ctx context.Context, tx pgx.Tx, world uuid.UUID, j *province.TradeJourney, now time.Time) error {
	a, err := tick.LoadAnchor(ctx, tx, world)
	if err != nil {
		return err
	}
	dispatch := a.ExactMilliAt(now)
	if dispatch < int64(a.Tick)*1000 {
		dispatch = int64(a.Tick) * 1000
	}
	j.StartAt(dispatch, now, time.Duration(tick.TickSeconds)*time.Second)
	return nil
}

// EnqueueFreightTx reads the just-written physical mover. Its saved route owns
// the deadline; old envelopes and NULL-journey return chains keep old semantics.
func EnqueueFreightTx(ctx context.Context, tx pgx.Tx, s *events.Scheduler, world uuid.UUID, typ events.ScheduledEventType, payload any, due int) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var p struct {
		TransportID uuid.UUID `json:"transport_id"`
	}
	if err = json.Unmarshal(raw, &p); err != nil {
		return err
	}
	var journey []byte
	if err = tx.QueryRow(ctx, `SELECT journey FROM transports WHERE id=$1`, p.TransportID).Scan(&journey); err != nil {
		return err
	}
	var j province.TradeJourney
	if len(journey) > 0 {
		if err = json.Unmarshal(journey, &j); err != nil {
			return err
		}
	}
	if j.Clock == nil {
		return s.EnqueueTickTx(ctx, tx, world, typ, payload, due)
	}
	switch typ {
	case events.ScheduledTransportArrival:
		typ = events.ScheduledTransportArrivalV2
	case events.ScheduledTradeDelivery:
		typ = events.ScheduledTradeDeliveryV2
	case events.ScheduledGiftDelivery:
		typ = events.ScheduledGiftDeliveryV2
	case events.ScheduledShipmentDeliveryV2:
		typ = events.ScheduledShipmentDeliveryV3
	case events.ScheduledTradeReturn:
		typ = events.ScheduledTradeReturnV2
	}
	r := j.Route(0, 0)
	return s.EnqueueDeadlineTx(ctx, tx, world, typ, payload, r.EndTick, r.WallAt(int64(r.EndTick)*1000+r.OffsetMilli))
}
