package combat

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Unit-loyalty (L2) is DERIVED, never stored: a unit inherits the loyalty of the
// settlement that supplies it (units.settlement_id when garrisoned, otherwise its
// owner's capital — the same paying-settlement fallback the upkeep loop uses).
// A high settlement ⇒ a high unit, always in sync, with no separate event stream.
// Low unit-loyalty makes an army desert faster (upkeep) and rout sooner (resolver).

// defaultLoyalty is the loyalty assumed when the supplying settlement can't be
// read (no capital, deleted row). It matches the starting loyalty so an unknown
// settlement behaves as an ordinary loyal one, never as near-revolt.
const defaultLoyalty = 2

// loyaltyReader is the read subset of pgx satisfied by both pgx.Tx and
// *pgxpool.Pool, so combat resolution (tx) and the upkeep loop (pool) share the
// same loyalty lookups.
type loyaltyReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// settlementLoyalty returns a settlement's current loyalty (1–4), or
// defaultLoyalty if it can't be read.
func settlementLoyalty(ctx context.Context, db loyaltyReader, settlementID uuid.UUID) int {
	l := defaultLoyalty
	if err := db.QueryRow(ctx,
		`SELECT loyalty FROM settlements WHERE id = $1`, settlementID,
	).Scan(&l); err != nil {
		return defaultLoyalty
	}
	return l
}

// supplyingSettlement resolves the settlement that supplies a unit for
// loyalty purposes: its own settlement_id if garrisoned there, else its owner's
// capital. Returns (id, loyalty, ok). ok is false when neither can be found.
func supplyingSettlement(ctx context.Context, db loyaltyReader, ownerID uuid.UUID, settlementID *uuid.UUID, worldID uuid.UUID) (uuid.UUID, int, bool) {
	if settlementID != nil {
		return *settlementID, settlementLoyalty(ctx, db, *settlementID), true
	}
	var id uuid.UUID
	var loyalty int
	if err := db.QueryRow(ctx,
		`SELECT id, loyalty FROM settlements
		 WHERE owner_id = $1 AND world_id = $2 AND is_capital = true`,
		ownerID, worldID,
	).Scan(&id, &loyalty); err != nil {
		return uuid.UUID{}, defaultLoyalty, false
	}
	return id, loyalty, true
}

// desertionStepForLoyalty (L2) scales silver-shortage desertion by the supplying
// settlement's loyalty: a disloyal army sheds more men per unpaid period, a
// fanatical one fewer. Baseline (loyalty 2) keeps the existing upkeepDesertionStep.
// Calibration, not an invariant.
func desertionStepForLoyalty(loyalty int) int {
	switch loyalty {
	case 1:
		return upkeepDesertionStep * 2 // near-revolt: men melt away
	case 4:
		return upkeepDesertionStep / 2 // fanatical: cling on
	default:
		return upkeepDesertionStep // loyalty 2/3 (and unknown) = baseline
	}
}
