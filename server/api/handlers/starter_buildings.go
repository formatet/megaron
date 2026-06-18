package handlers

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/poleia/server/internal/province"
)

// seedStarterBuildings gives a new capital the minimal building set so the core
// loop is ALIVE from t=0: farm + lumbermill (production), temple (cult → kharis),
// market (silver income). Without these a fresh settlement has no silver income
// and no cult production, so kharis decays to zero (eternal KharisMissedMaintenance
// + DivinePunishment) and the silver economy never starts — both observed dead in
// the W8 world. Giving every settlement a working base means the religion and trade
// subsystems exercise regardless of how the (8B) governor plays.
//
// Must be called inside an open pgx.Tx, BEFORE RecomputeProduction, so the temple →
// cult and farm/lumbermill rates are picked up. Applies the market's silver_rate
// bonus directly (mirrors combat.BuildCompleteHandler) and seeds a little starting
// silver for immediate trade liquidity. Idempotent via ON CONFLICT DO NOTHING.
func seedStarterBuildings(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID) error {
	if _, err := tx.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level)
		 SELECT $1, bt, 1 FROM unnest(ARRAY['farm','lumbermill','temple','market']) AS bt
		 ON CONFLICT (settlement_id, building_type) DO NOTHING`,
		settlementID,
	); err != nil {
		return fmt.Errorf("seed starter buildings: %w", err)
	}

	// Market provides silver income; also seed starting silver so buy-offers can
	// clear from t=0. Without this every Wanax sits at silver=0 and no buy-offer
	// is ever generated (the trade contract only supports paying silver, not
	// barter), so silver never enters circulation — a zero-liquidity deadlock.
	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET
		   silver_amount  = 300,
		   silver_rate    = silver_rate + $1,
		   silver_calc_at = now()
		 WHERE id = $2`,
		province.BuildingSpecs[province.BuildingMarket].SilverRate, settlementID,
	); err != nil {
		return fmt.Errorf("apply market silver rate: %w", err)
	}
	return nil
}
