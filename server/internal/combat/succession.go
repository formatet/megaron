package combat

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// handleOwnerCityLoss applies metropolis-succession or game-over bookkeeping after
// a settlement leaves a Wanax's control — whether by collapse (pop ≤ 100) or by
// conquest. It MUST be called AFTER the lost settlement's ownership has been
// cleared or transferred (its owner_id no longer points at the losing player) and
// its state updated, within the same transaction.
//
// The rule (Timothy 2026-07-10):
//   - 0 active settlements remain → the Wanax has lost the game. Mark them
//     'dispossessed' and anchor last_settlement_id to the fallen city so the
//     epitaph crawl can render their reign. Returns gameOver = true.
//   - ≥1 remain but none is a capital → the lost city was their metropolis;
//     automatically promote the highest-loyalty survivor to capital (no player
//     choice — loyalty decides, population then id break ties).
//   - ≥1 remain and a capital still stands → a mere colony fell; nothing to do.
//
// This replaces the old unconditional "mark dispossessed" writes, which fired even
// when the player still held other cities (harmless only because respawn used to
// replant a capital — respawn is now removed).
func handleOwnerCityLoss(ctx context.Context, tx pgx.Tx, ownerID, worldID, lostSettlementID uuid.UUID) (gameOver bool, err error) {
	if ownerID == uuid.Nil {
		return false, nil
	}

	// "Held" = any settlement still theirs, even if besieged or revolting; only
	// 'collapsed' (torn down) and 'sunk' (island lost) no longer count.
	var remaining int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM settlements
		 WHERE owner_id = $1 AND world_id = $2
		   AND state NOT IN ('collapsed','sunk') AND id <> $3`,
		ownerID, worldID, lostSettlementID,
	).Scan(&remaining); err != nil {
		return false, fmt.Errorf("count remaining settlements: %w", err)
	}

	if remaining == 0 {
		// Game over: the fallen city was their last. Record the anchor for the epitaph.
		if _, err := tx.Exec(ctx,
			`UPDATE player_world_records
			   SET status = 'dispossessed', settlement_id = NULL, last_settlement_id = $3
			 WHERE player_id = $1 AND world_id = $2`,
			ownerID, worldID, lostSettlementID,
		); err != nil {
			return true, fmt.Errorf("mark dispossessed: %w", err)
		}
		slog.Info("wanax defeated — last settlement lost",
			"player", ownerID, "world", worldID, "last_settlement", lostSettlementID)
		return true, nil
	}

	// The Wanax survives. Ensure a capital still stands; if not, the lost city was
	// the metropolis → promote the highest-loyalty survivor.
	var capitals int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM settlements
		 WHERE owner_id = $1 AND world_id = $2
		   AND state NOT IN ('collapsed','sunk') AND is_capital = true AND id <> $3`,
		ownerID, worldID, lostSettlementID,
	).Scan(&capitals); err != nil {
		return false, fmt.Errorf("count surviving capitals: %w", err)
	}
	if capitals > 0 {
		return false, nil // a colony fell; the metropolis still stands
	}

	// Promote by loyalty (Timothy 2026-07-10). Prefer a healthy seat over a
	// besieged/revolting one; population then id break ties deterministically.
	var newCapital uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT id FROM settlements
		 WHERE owner_id = $1 AND world_id = $2
		   AND state NOT IN ('collapsed','sunk') AND id <> $3
		 ORDER BY (state = 'active') DESC, loyalty DESC, population DESC, id ASC
		 LIMIT 1`,
		ownerID, worldID, lostSettlementID,
	).Scan(&newCapital); err != nil {
		// remaining > 0 so a survivor must exist; never fail the collapse/conquest
		// over succession bookkeeping — log and leave the player capital-less
		// (recoverable) rather than rolling back the teardown.
		slog.Warn("metropolis succession: no promotable settlement found",
			"player", ownerID, "world", worldID, "err", err)
		return false, nil
	}

	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET is_capital = true, updated_at = now() WHERE id = $1`,
		newCapital,
	); err != nil {
		return false, fmt.Errorf("promote new capital: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE player_world_records SET settlement_id = $3
		 WHERE player_id = $1 AND world_id = $2`,
		ownerID, worldID, newCapital,
	); err != nil {
		return false, fmt.Errorf("repoint records to new capital: %w", err)
	}
	slog.Info("metropolis succession — new capital promoted",
		"player", ownerID, "world", worldID, "new_capital", newCapital, "fallen", lostSettlementID)
	return false, nil
}
