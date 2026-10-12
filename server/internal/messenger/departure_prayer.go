package messenger

import (
	"context"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"math"
	"time"
)

// BlessRunner freezes only the outward land leg. Aboard travel is always read
// from the actual ship; replies and disembarkation never consume this gift.
func BlessRunner(ctx context.Context, tx pgx.Tx, world, owner, city uuid.UUID, now time.Time, current, due int) (time.Time, int, error) {
	if due <= current {
		return now, due, nil
	}
	blessed, err := religion.ConsumeDeparture(ctx, tx, world, owner, city, religion.EffectNextLandDeparture)
	if err != nil {
		return time.Time{}, 0, err
	}
	if blessed {
		due = current + max(1, int(math.Round(float64(due-current)*religion.DepartureTimeFactor)))
	}
	return now.Add(tick.RealUntil(due, current)), due, nil
}
