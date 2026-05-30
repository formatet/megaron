package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/province"
	"github.com/poleia/server/internal/settlement"
)

// loadTerrainProvince fetches terrain data for a province hex tile.
func loadTerrainProvince(ctx context.Context, pool *pgxpool.Pool, id, worldID uuid.UUID) (*province.Province, error) {
	var p province.Province
	err := pool.QueryRow(ctx,
		`SELECT id, world_id, map_q, map_r, terrain_type, territory_state, controller_id,
		        copper_deposit, tin_deposit
		 FROM provinces WHERE id = $1 AND world_id = $2`,
		id, worldID,
	).Scan(&p.ID, &p.WorldID, &p.MapTile.Q, &p.MapTile.R,
		&p.TerrainType, &p.TerritoryState, &p.ControllerID,
		&p.CopperDeposit, &p.TinDeposit)
	if err != nil {
		return nil, fmt.Errorf("scan province: %w", err)
	}
	return &p, nil
}

// loadSettlement fetches a settlement by its own UUID.
func loadSettlement(ctx context.Context, pool *pgxpool.Pool, id, worldID uuid.UUID) (*settlement.Settlement, error) {
	row := pool.QueryRow(ctx,
		`SELECT id, world_id, province_id, name, culture_id, owner_id, kingdom_id,
		        control_type, founded_from, governor_id, governor_is_ai,
		        loyalty, loyalty_trend, wall_level, is_capital, state, population,
		        gold_amount, gold_rate, gold_cap, gold_calc_at,
		        food_amount, food_rate, food_cap, food_calc_at,
		        lumber_amount, lumber_rate, lumber_cap, lumber_calc_at,
		        stone_amount, stone_rate, stone_cap, stone_calc_at,
		        kharis_amount, kharis_rate, kharis_cap, kharis_calc_at,
		        infantry, cavalry, catapult, priest, ship, elite_infantry,
		        updated_at
		 FROM settlements WHERE id = $1 AND world_id = $2`,
		id, worldID,
	)
	return scanSettlement(row)
}

// loadSettlementByProvince fetches the settlement that occupies a given province tile.
func loadSettlementByProvince(ctx context.Context, pool *pgxpool.Pool, provinceID, worldID uuid.UUID) (*settlement.Settlement, error) {
	var id uuid.UUID
	err := pool.QueryRow(ctx,
		`SELECT id FROM settlements WHERE province_id = $1 AND world_id = $2`,
		provinceID, worldID,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("no settlement")
	}
	if err != nil {
		return nil, err
	}
	return loadSettlement(ctx, pool, id, worldID)
}

// loadPlayerCapital finds the player's capital settlement in a world.
func loadPlayerCapital(ctx context.Context, pool *pgxpool.Pool, playerID, worldID uuid.UUID) (*settlement.Settlement, error) {
	var id uuid.UUID
	err := pool.QueryRow(ctx,
		`SELECT id FROM settlements WHERE world_id = $1 AND owner_id = $2 AND is_capital = true`,
		worldID, playerID,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("no capital")
	}
	if err != nil {
		return nil, err
	}
	return loadSettlement(ctx, pool, id, worldID)
}

// resolveSettlementID returns the settlement UUID for a given province tile.
func resolveSettlementID(ctx context.Context, pool *pgxpool.Pool, provinceID, worldID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := pool.QueryRow(ctx,
		`SELECT id FROM settlements WHERE province_id = $1 AND world_id = $2`,
		provinceID, worldID,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("no settlement for province %s", provinceID)
	}
	return id, err
}

// scanSettlement reads a settlement from a pgx.Row.
func scanSettlement(row pgx.Row) (*settlement.Settlement, error) {
	var s settlement.Settlement
	var goldCalcAt, foodCalcAt, lumberCalcAt, stoneCalcAt, kharisCalcAt time.Time

	err := row.Scan(
		&s.ID, &s.WorldID, &s.ProvinceID, &s.Name, &s.CultureID,
		&s.OwnerID, &s.KingdomID, &s.ControlType, &s.FoundedFrom,
		&s.GovernorID, &s.GovernorIsAI,
		&s.Loyalty, &s.LoyaltyTrend, &s.WallLevel, &s.IsCapital, &s.State, &s.Population,
		&s.Resources.Gold.Amount, &s.Resources.Gold.RatePerMinute, &s.Resources.Gold.Cap, &goldCalcAt,
		&s.Resources.Food.Amount, &s.Resources.Food.RatePerMinute, &s.Resources.Food.Cap, &foodCalcAt,
		&s.Resources.Lumber.Amount, &s.Resources.Lumber.RatePerMinute, &s.Resources.Lumber.Cap, &lumberCalcAt,
		&s.Resources.Stone.Amount, &s.Resources.Stone.RatePerMinute, &s.Resources.Stone.Cap, &stoneCalcAt,
		&s.Resources.Kharis.Amount, &s.Resources.Kharis.RatePerMinute, &s.Resources.Kharis.Cap, &kharisCalcAt,
		&s.Army.Infantry, &s.Army.Cavalry, &s.Army.Catapult, &s.Army.Priest, &s.Army.Ship, &s.Army.EliteInfantry,
		&s.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan settlement: %w", err)
	}

	s.Resources.Gold.LastCalcAt = goldCalcAt
	s.Resources.Food.LastCalcAt = foodCalcAt
	s.Resources.Lumber.LastCalcAt = lumberCalcAt
	s.Resources.Stone.LastCalcAt = stoneCalcAt
	s.Resources.Kharis.LastCalcAt = kharisCalcAt

	return &s, nil
}
