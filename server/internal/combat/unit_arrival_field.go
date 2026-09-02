package combat

// P2 fix (2026-07-18 soak, "Dole mot Eastern Outpost"): a march arriving on a
// hex with no settlement row used to always garrison peacefully, even when a
// hostile unit already sat there (status='positioned' — the only way a unit
// occupies a settlement-less hex; there was no "outpost province" establishment
// path in the codebase despite provinces.owner_id/outpost_feeds existing — see
// migration 030. Those columns were dropped outright in migration 138
// (2026-09-02) since nothing ever wrote them. The soak report's "Eastern
// Outpost" was in fact this: an enemy unit parked on open ground). This file
// adds the missing combat resolution for that case, mirroring resolveCombat's
// strength/fortune/loyalty math (in unit_arrival.go) but without a wall bonus
// (nothing to besiege) or a settlement to capture — a win destroys the
// defending field units outright, a loss routs/destroys the attacker exactly
// like resolveCombat's applyDefenderWins path.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// fieldDefender is one hostile unit sitting 'positioned' at a settlement-less
// hex — a candidate defender in resolveFieldCombat.
type fieldDefender struct {
	id      uuid.UUID
	ownerID uuid.UUID
	utype   string
	size    int
	stance  *string
}

// loadFieldDefenders returns every unit hostile to attackerID sitting
// 'positioned' at (q,r) — i.e. holding open ground with no settlement there.
// Empty (nil, nil) means the hex is uncontested.
func loadFieldDefenders(ctx context.Context, tx pgx.Tx, worldID uuid.UUID, q, r int, attackerID uuid.UUID) ([]fieldDefender, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, owner_id, type, size, stance FROM units
		 WHERE world_id = $1 AND q = $2 AND r = $3 AND status = 'positioned' AND owner_id != $4`,
		worldID, q, r, attackerID,
	)
	if err != nil {
		return nil, fmt.Errorf("load field defenders: %w", err)
	}
	defer rows.Close()
	var out []fieldDefender
	for rows.Next() {
		var d fieldDefender
		if scanErr := rows.Scan(&d.id, &d.ownerID, &d.utype, &d.size, &d.stance); scanErr == nil {
			out = append(out, d)
		}
	}
	return out, rows.Err()
}

// resolveFieldCombat used to resolve an arriving unit against hostile field
// defenders (already loaded by the caller, resolve() in unit_arrival.go) as a
// single immediate strength/fortune roll. KR3 (megaron_plan_kr3_stridssystem.md
// §1) replaces that one-shot model: this now only INITIATES (or joins) a
// persistent battles row and hands the arriving unit's outcome to
// ScheduledBattleTick, resolved over one or more subsequent battle-ticks.
// The old fortune/strength/wall math (rollFortune, ResolveStrengthsWithRout)
// stays in this package for old data and its own direct-call unit tests, but
// is no longer reachable from any production entry point — all four (settlement
// resolveCombat, resolveFieldCombat, amphibious resolveAmphibiousAssault, and
// avsiktslagret's unit_intercept_scan.go) now go through initiateOrJoinBattle.
// The live field-battle balance is battle.go's seeded T12 dice model, which is
// already an economy.Dice consumer and reproducible per (seed, tick, round).
func (h *UnitArrivalHandler) resolveFieldCombat(
	ctx context.Context, tx pgx.Tx,
	u unitRow, defenders []fieldDefender, destQ, destR int, worldID uuid.UUID,
) error {
	arriving := battleParticipant{unitID: u.id, ownerID: u.ownerID, utype: u.utype, side: "attacker", currentSize: u.size}
	var defParticipants []battleParticipant
	for _, d := range defenders {
		defParticipants = append(defParticipants, battleParticipant{unitID: d.id, ownerID: d.ownerID, utype: d.utype, side: "defender", currentSize: d.size})
	}

	if err := h.initiateOrJoinBattle(ctx, tx, worldID, destQ, destR, arriving, defParticipants); err != nil {
		return fmt.Errorf("field combat: initiate/join battle: %w", err)
	}

	slog.Info("field combat: battle initiated/joined", "unit", u.id, "q", destQ, "r", destR, "defenders", len(defenders))

	// The arriving unit now holds the contested hex while the battle resolves
	// over subsequent battle-ticks — no immediate outcome, no win/lose branch
	// here anymore. It keeps its current size; ScheduledBattleTick applies
	// losses as rounds are resolved.
	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   status        = 'positioned',
		   q             = $2,
		   r             = $3,
		   settlement_id = NULL,
		   target_q      = NULL,
		   target_r      = NULL,
		   departs_at    = NULL,
		   arrives_at    = NULL,
		   depart_tick   = NULL,
		   arrive_tick   = NULL,
		   updated_at    = now()
		 WHERE id = $1`,
		u.id, destQ, destR,
	); err != nil {
		return fmt.Errorf("field combat: position arriving unit: %w", err)
	}

	return nil
}
