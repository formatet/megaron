package handlers

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// seedStarterBuildings gives a new capital the minimal building set so the core
// loop is ALIVE from t=0: farm + lumbermill (production), temple (cult → kharis),
// market. Without these a fresh settlement has no production and no cult, so
// kharis decays to zero and the silver economy never starts — both observed dead
// in the W8 world. Giving every settlement a working base means the religion and
// trade subsystems exercise regardless of how the (8B) governor plays.
//
// Must be called inside an open pgx.Tx, BEFORE RecomputeProduction, so the temple →
// cult and farm/lumbermill rates are picked up. Seeds starting silver (300) directly
// into the settlement_goods silver row for immediate trade liquidity. Idempotent
// via ON CONFLICT.
func seedStarterBuildings(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID) error {
	if _, err := tx.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level)
		 SELECT $1, bt, 1 FROM unnest(ARRAY['farm','lumbermill','temple','market']) AS bt
		 ON CONFLICT (settlement_id, building_type) DO NOTHING`,
		settlementID,
	); err != nil {
		return fmt.Errorf("seed starter buildings: %w", err)
	}

	// Seed starting silver so buy-offers can clear from t=0. Without this every
	// Wanax sits at silver=0 and no buy-offer is ever generated (the trade
	// contract only supports paying silver, not barter), so silver never enters
	// circulation — a zero-liquidity deadlock.
	if _, err := tx.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'silver', 300, 0, 1000, current_world_tick())
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET amount = 300, calc_tick = current_world_tick()`,
		settlementID,
	); err != nil {
		return fmt.Errorf("seed starting silver: %w", err)
	}
	return nil
}
