package messenger

import (
	"context"
	"encoding/json"
	"time"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/movement"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SpeedUpMessenger shortens what is left of a Runner's free land leg (bönesystemet
// v3 steg 2b). A Runner aboard a carrier, awaiting passage, or without a saved
// route is left alone: it follows its ship or caravan. Idempotent through the
// route's own mark (the mark is born anew with every new leg).
func SpeedUpMessenger(ctx context.Context, tx pgx.Tx, id, blessing uuid.UUID, now time.Time) (*religion.SpedUp, error) {
	var raw []byte
	var status string
	var passage *string
	var carrierT, carrierU *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT land_route,status,passage_status,carrier_transport_id,carrier_unit_id FROM messengers WHERE id=$1 FOR UPDATE`, id).
		Scan(&raw, &status, &passage, &carrierT, &carrierU); err != nil {
		return nil, err
	}
	if len(raw) == 0 || passage != nil || carrierT != nil || carrierU != nil || (status != "outbound" && status != "returning") {
		return nil, nil
	}
	var r movement.StoredRoute
	if json.Unmarshal(raw, &r) != nil || r.DepartureAt.IsZero() || r.TickNanoseconds <= 0 {
		return nil, nil
	}
	sped, ok := movement.SpeedUpRemaining(r, r.MilliAt(now), religion.SpeedNum, religion.SpeedDen)
	if !ok {
		return nil, nil
	}
	sped.Sped = blessing.String()
	out, err := json.Marshal(sped)
	if err != nil {
		return nil, err
	}
	arrives := sped.WallAt(int64(sped.EndTick)*1000 + sped.OffsetMilli)
	if _, err := tx.Exec(ctx, `UPDATE messengers SET land_route=$2,arrives_at=$3 WHERE id=$1`, id, out, arrives); err != nil {
		return nil, err
	}
	if _, err := events.RequeueByPayload(ctx, tx, "messenger_id", id.String(), events.MessengerJobTypes, sped.EndTick, arrives); err != nil {
		return nil, err
	}
	return &religion.SpedUp{Kind: "messenger", ID: id, OldArriveTick: r.EndTick, NewArriveTick: sped.EndTick}, nil
}

// speedAtDeparture is called by SaveDepartureTx right after the leg's route is
// frozen: a live speed prayer over the Runner's city (or the realm) shortens the
// whole leg. The arrival job is queued afterwards, from the new route.
func speedAtDeparture(ctx context.Context, tx pgx.Tx, id uuid.UUID, now time.Time) error {
	var owner uuid.UUID
	var city *uuid.UUID
	var tick int
	if err := tx.QueryRow(ctx, `SELECT m.sender_id,m.origin_id,w.current_tick FROM messengers m JOIN worlds w ON w.id=m.world_id WHERE m.id=$1`, id).Scan(&owner, &city, &tick); err != nil {
		return err
	}
	b, ok, err := religion.SpeedBlessingFor(ctx, tx, owner, religion.EffectBlessedSpeedLand, id, city, tick)
	if err != nil || !ok {
		return err
	}
	_, err = SpeedUpMessenger(ctx, tx, id, b, now)
	return err
}
