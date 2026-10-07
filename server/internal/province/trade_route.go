package province

import (
	"context"
	"errors"
	"github.com/google/uuid"
)

// ResolveTradeRoute prefers an actual naval route when both settlements are
// coastal or have harbours. Otherwise it requires a real land route. Ship
// availability belongs to the caller; a shipless caller must plan land again.
func ResolveTradeRoute(ctx context.Context, db Queryer, worldID uuid.UUID, originCoastal, destCoastal bool, origin, dest MapPosition) (category string, dist int, err error) {
	g, err := LoadTileGraph(ctx, db, worldID)
	if err != nil {
		return "", 0, err
	}
	if originCoastal && destCoastal {
		j, err := g.PlanTradeJourney(origin, dest, "naval")
		if err == nil {
			return j.Category, j.Distance, nil
		}
		if !errors.Is(err, ErrNoTradePath) {
			return "", 0, err
		}
	}
	j, err := g.PlanTradeJourney(origin, dest, "land")
	if err != nil {
		return "", 0, err
	}
	return j.Category, j.Distance, nil
}
