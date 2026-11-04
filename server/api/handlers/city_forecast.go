package handlers

import (
	"context"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/kharis"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// NewCityForecaster is the one wiring of the forward owner of a city's goods:
// combat composes upkeep and food, kharis supplies the temple plan. The tick's
// FoodRunningLow notice, the Warehouse and the status read the same instance.
func NewCityForecaster() combat.StorageForecaster {
	return combat.StorageForecaster{Tithe: func(ctx context.Context, tx pgx.Tx, sid uuid.UUID) (map[string]map[string]float64, error) {
		return kharis.ForecastTithe(ctx, tx, sid)
	}}
}
