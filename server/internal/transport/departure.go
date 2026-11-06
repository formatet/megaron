package transport

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"math"
)

// BlessDeparture is called before ETA/scheduling. prepareJourney also calls it
// for Dispatch writers; DepartureChecked prevents a second consumption.
func BlessDeparture(ctx context.Context, tx pgx.Tx, world, owner, city uuid.UUID, j *province.TradeJourney, outward bool) error {
	if j.DepartureChecked {
		return nil
	}
	if err := j.Validate(); err != nil {
		return err
	}
	j.DepartureChecked = true
	if !outward {
		return nil
	}
	effect := religion.EffectNextLandDeparture
	if j.Category == "naval" {
		effect = religion.EffectNextShipDeparture
	}
	blessed, err := religion.ConsumeDeparture(ctx, tx, world, owner, city, effect)
	if err != nil {
		return err
	}
	if !blessed {
		return blessSpeed(ctx, tx, world, owner, city, j)
	}
	j.TimeFactorMilli = int(math.Round(religion.DepartureTimeFactor * 1000))
	j.Thread = j.Category == "land"
	j.Stillness = j.Category == "naval"
	var total int64
	for _, cost := range j.StepCosts {
		total += cost
	}
	j.TravelTicks = max(1, int((total*3*int64(j.TimeFactorMilli)+1000000)/2000000))
	return j.Validate()
}

func nullableBlessingTick(blessed bool, t int) *int {
	if !blessed {
		return nil
	}
	return &t
}

func departureInterceptRadius(t inFlightTransport) int {
	var j province.TradeJourney
	if t.category == "land" && json.Unmarshal(t.journey, &j) == nil && j.Thread {
		return 1
	}
	return interceptRadius
}

// blessSpeed lets a live speed prayer shorten a departing journey (steg 2b): the
// whole journey takes DepartureTimeFactor of its time. A journey already sped by a
// departure prayer is not sped again.
// The realm and the departing city are covered; a ship named alone is not (no ship is known here).
func blessSpeed(ctx context.Context, tx pgx.Tx, world, owner, city uuid.UUID, j *province.TradeJourney) error {
	if j.TimeFactorMilli == 750 || j.SpeedBlessing != "" {
		return nil
	}
	effect := religion.EffectBlessedSpeedLand
	if j.Category == "naval" {
		effect = religion.EffectBlessedSpeedSea
	}
	var tick int
	if err := tx.QueryRow(ctx, `SELECT current_tick FROM worlds WHERE id=$1`, world).Scan(&tick); err != nil {
		return err
	}
	id, ok, err := religion.SpeedBlessingFor(ctx, tx, owner, effect, uuid.Nil, &city, tick)
	if err != nil || !ok {
		return err
	}
	var total int64
	for _, cost := range j.StepCosts {
		total += cost
	}
	j.TravelTicks = max(1, int((total*3*750+1000000)/2000000))
	j.SpeedBlessing = id.String()
	return j.Validate()
}
