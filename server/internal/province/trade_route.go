package province

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
)

// ResolveTradeRoute prefers an actual naval route when both settlements are
// coastal or have harbours. Otherwise it requires a real land route. Ship
// availability belongs to the caller; a shipless caller must plan land again.
func ResolveTradeRoute(ctx context.Context, db Queryer, worldID uuid.UUID, originCoastal, destCoastal bool, origin, dest MapPosition) (category string, dist int, err error) {
	g, roads, err := LoadWorldGraph(ctx, db, worldID)
	if err != nil {
		return "", 0, err
	}
	if originCoastal && destCoastal {
		j, err := g.PlanTradeJourney(origin, dest, "naval", roads)
		if err == nil {
			return j.Category, j.Distance, nil
		}
		if !errors.Is(err, ErrNoTradePath) {
			return "", 0, err
		}
	}
	j, err := g.PlanTradeJourney(origin, dest, "land", roads)
	if err != nil {
		return "", 0, err
	}
	return j.Category, j.Distance, nil
}

// ValidateTransportBy accepts only explicit player choices or the legacy default.
func ValidateTransportBy(by string) error {
	if by != "" && by != "land" && by != "sea" {
		return fmt.Errorf("by must be land or sea")
	}
	return nil
}

// ResolveTradeRouteBy never substitutes another category for an explicit choice.
// Empty preserves the old naval preference and caller-controlled ship fallback.
func ResolveTradeRouteBy(ctx context.Context, db Queryer, world uuid.UUID, originCoastal, destCoastal bool, origin, dest MapPosition, by string) (string, int, error) {
	if err := ValidateTransportBy(by); err != nil {
		return "", 0, err
	}
	if by == "" {
		return ResolveTradeRoute(ctx, db, world, originCoastal, destCoastal, origin, dest)
	}
	category := "land"
	if by == "sea" {
		if !originCoastal || !destCoastal {
			return "", 0, ErrNoTradePath
		}
		category = "naval"
	}
	j, err := PlanTradeJourney(ctx, db, world, origin, dest, category)
	if err != nil {
		return "", 0, err
	}
	return category, j.Distance, nil
}
