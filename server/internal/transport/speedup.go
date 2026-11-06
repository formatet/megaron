package transport

import (
	"context"
	"encoding/json"
	"time"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SpeedUpTransport shortens what is left of one caravan or sea freight under way
// (bönesystemet v3 steg 2b). Idempotent: a journey already sped up (a speed prayer
// or a departure prayer) is left alone and (nil, nil) is returned. The position
// now is the same before and after; the pending delivery job moves in place.
func SpeedUpTransport(ctx context.Context, tx pgx.Tx, id, blessing uuid.UUID, now time.Time) (*religion.SpedUp, error) {
	var raw []byte
	var status string
	var due int
	if err := tx.QueryRow(ctx, `SELECT journey,status,due_tick FROM transports WHERE id=$1 FOR UPDATE`, id).Scan(&raw, &status, &due); err != nil {
		return nil, err
	}
	var j province.TradeJourney
	if status != "in_transit" || len(raw) == 0 || json.Unmarshal(raw, &j) != nil || j.Clock == nil || j.Validate() != nil {
		return nil, nil
	}
	if j.TimeFactorMilli == 750 || j.SpeedBlessing != "" {
		return nil, nil
	}
	r := j.Route(0, 0)
	sped, ok := movementSpeedUp(r, r.MilliAt(now))
	if !ok {
		return nil, nil
	}
	j.Clock.EndTick = sped.EndTick
	j.StepCosts = sped.Costs
	j.TravelTicks = sped.EndTick - sped.StartTick
	j.SpeedBlessing = blessing.String()
	if j.Validate() != nil {
		return nil, nil
	}
	out, err := json.Marshal(j)
	if err != nil {
		return nil, err
	}
	arrives := sped.WallAt(int64(sped.EndTick)*1000 + sped.OffsetMilli)
	if _, err := tx.Exec(ctx, `UPDATE transports SET journey=$2,due_tick=$3,arrives_at=$4,updated_at=now() WHERE id=$1`, id, out, sped.EndTick, arrives); err != nil {
		return nil, err
	}
	if _, err := events.RequeueByPayload(ctx, tx, "transport_id", id.String(), events.TransportJobTypes, sped.EndTick, arrives); err != nil {
		return nil, err
	}
	return &religion.SpedUp{Kind: "transport", ID: id, OldArriveTick: due, NewArriveTick: sped.EndTick}, nil
}
