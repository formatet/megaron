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
	if err != nil || !blessed {
		return err
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
