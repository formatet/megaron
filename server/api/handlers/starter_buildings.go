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
// cult and farm/lumbermill rates are picked up. Idempotent via ON CONFLICT.
//
// Starting liquid silver is NOT seeded here — it used to be a flat 300/cap-1000
// stopgap for the zero-liquidity deadlock, but that flat seed ran after (and
// clobbered) the caller's pop-scaled genesis liquid-silver seed
// (economy.GenesisSilverLiquid, temenos_sitos.md). Callers (join.go, respawn.go)
// seed liquid silver themselves, earlier in the same tx, before calling this.
func seedStarterBuildings(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID) error {
	if _, err := tx.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level)
		 SELECT $1, bt, 1 FROM unnest(ARRAY['farm','lumbermill','temple','market']) AS bt
		 ON CONFLICT (settlement_id, building_type) DO NOTHING`,
		settlementID,
	); err != nil {
		return fmt.Errorf("seed starter buildings: %w", err)
	}
	return nil
}
