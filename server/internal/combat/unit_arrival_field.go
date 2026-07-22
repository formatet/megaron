package combat

// P2 fix (2026-07-18 soak, "Dole mot Eastern Outpost"): a march arriving on a
// hex with no settlement row used to always garrison peacefully, even when a
// hostile unit already sat there (status='positioned' — the only way a unit
// occupies a settlement-less hex; there is no "outpost province" establishment
// path in the current codebase despite the provinces.owner_id/outpost_feeds
// columns existing — see migration 030's comment. The soak report's "Eastern
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
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
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

// resolveFieldCombat resolves an arriving unit against hostile field
// defenders already loaded by the caller (resolve() in unit_arrival.go).
func (h *UnitArrivalHandler) resolveFieldCombat(
	ctx context.Context, tx pgx.Tx,
	u unitRow, defenders []fieldDefender, destQ, destR int, worldID uuid.UUID,
) error {
	attStr := unitStrength(u.utype, u.size)

	// ── Defence strength: sum of every hostile unit on the hex. Mixed-owner
	// stacks on one open hex are not otherwise modelled elsewhere in the
	// codebase; the first defender's owner is used as the representative side
	// below for the kharis/loyalty bias. ──
	const fortifyBonus = 1.5
	var defStr float64
	for _, d := range defenders {
		str := unitStrength(d.utype, d.size)
		if d.stance != nil && *d.stance == "fortify" {
			str *= fortifyBonus
		}
		defStr += str
	}
	defenderOwnerID := defenders[0].ownerID

	// ── Fortune (W5): roll once, bias by kharis delta — same as resolveCombat. ──
	var attackerKharis, defenderKharis float64
	_ = tx.QueryRow(ctx,
		`SELECT GREATEST(0, settled(kharis_amount, kharis_rate, kharis_calc_tick))
		 FROM player_world_records WHERE player_id = $1 AND world_id = $2`,
		u.ownerID, worldID,
	).Scan(&attackerKharis)
	_ = tx.QueryRow(ctx,
		`SELECT GREATEST(0, settled(kharis_amount, kharis_rate, kharis_calc_tick))
		 FROM player_world_records WHERE player_id = $1 AND world_id = $2`,
		defenderOwnerID, worldID,
	).Scan(&defenderKharis)
	fortune := rollFortune(attackerKharis, defenderKharis)
	attStrWithFortune := attStr * (1 + fortune)

	// ── L2 unit-loyalty rout bias. ──
	attSettleID, attLoyalty, attHasSettle := supplyingSettlement(ctx, tx, u.ownerID, nil, worldID)
	_, defLoyalty, _ := supplyingSettlement(ctx, tx, defenderOwnerID, nil, worldID)

	result := ResolveStrengthsWithRout(attStrWithFortune, defStr, fortune,
		routFractionForLoyalty(attLoyalty), routFractionForLoyalty(defLoyalty))

	slog.Info("field combat resolved",
		"unit", u.id, "q", destQ, "r", destR,
		"att", attStr, "fortune", fortune, "def", defStr, "outcome", result.Outcome,
		"rounds", result.Rounds, "defenders", len(defenders))

	attSizeBefore := u.size
	attSizeAfter := int(float64(u.size) * (1 - result.AttackerLosses))
	attPopLost := attSizeBefore - attSizeAfter

	if result.Outcome == OutcomeAttackerWins {
		if err := h.applyFieldDefenderLosses(ctx, tx, defenders, result.DefenderLosses, worldID); err != nil {
			return err
		}
		if attSizeAfter <= 0 {
			if _, err := tx.Exec(ctx,
				`UPDATE units SET status = 'disbanded', updated_at = now() WHERE id = $1`, u.id,
			); err != nil {
				return fmt.Errorf("field combat: disband zeroed attacker: %w", err)
			}
			h.disbandCargoIfPresent(ctx, tx, u, worldID)
		} else {
			if _, err := tx.Exec(ctx,
				`UPDATE units SET
				   size          = $2,
				   status        = 'positioned',
				   q             = $3,
				   r             = $4,
				   settlement_id = NULL,
				   target_q      = NULL,
				   target_r      = NULL,
				   departs_at    = NULL,
				   arrives_at    = NULL,
				   depart_tick   = NULL,
				   arrive_tick   = NULL,
				   updated_at    = now()
				 WHERE id = $1`,
				u.id, attSizeAfter, destQ, destR,
			); err != nil {
				return fmt.Errorf("field combat: position victorious attacker: %w", err)
			}
		}
		if attPopLost > 0 {
			if _, err := tx.Exec(ctx,
				`UPDATE settlements SET population = GREATEST(50, population - $2)
				 WHERE owner_id = $1 AND world_id = $3 AND is_capital = true`,
				u.ownerID, attPopLost, worldID,
			); err != nil {
				slog.Warn("field combat: could not apply attacker pop loss", "unit", u.id, "err", err)
			}
		}
		if h.hub != nil {
			_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, "FieldBattleWon", 3, map[string]any{
				"unit_id": u.id, "q": destQ, "r": destR,
			})
			_ = h.hub.NotifyPlayer(ctx, worldID, defenderOwnerID, "FieldBattleLost", 2, map[string]any{
				"q": destQ, "r": destR,
			})
		}
	} else {
		// No settlement to reference — reuses applyDefenderWins' rout/disband
		// logic (it only touches dest.settlementID at the very end, guarded by
		// a nil check, to apply settlement-garrison losses; field defender
		// losses below cover that instead).
		if err := h.applyDefenderWins(ctx, tx, u, destSettlement{}, attSizeAfter, attPopLost, result, destQ, destR, worldID); err != nil {
			return err
		}
		if err := h.applyFieldDefenderLosses(ctx, tx, defenders, result.DefenderLosses, worldID); err != nil {
			return err
		}
		if h.hub != nil {
			_ = h.hub.NotifyPlayer(ctx, worldID, defenderOwnerID, "FieldBattleWon", 3, map[string]any{
				"q": destQ, "r": destR,
			})
		}
	}

	_, _ = h.eventStore.Append(ctx, u.id, events.StreamType(unit.StreamUnit), unit.EventUnitCombatResolved,
		unit.UnitCombatResolvedPayload{
			UnitID:     u.id,
			Role:       "attacker",
			SizeBefore: attSizeBefore,
			SizeAfter:  attSizeAfter,
			Outcome:    string(result.Outcome),
			PopLost:    attPopLost,
		}, worldID, nil)

	h.applyBattleLoyalty(ctx, tx, result.Outcome, attSettleID, attHasSettle, nil, worldID)

	return nil
}

// applyFieldDefenderLosses reduces the size of hostile field units per the
// resolved loss rate; wiped-out ones are disbanded. Mirrors
// applyDefenderUnitLosses (unit_arrival.go) but keys on the hex-positioned
// unit rows already loaded by loadFieldDefenders rather than a settlement_id.
func (h *UnitArrivalHandler) applyFieldDefenderLosses(
	ctx context.Context, tx pgx.Tx, defenders []fieldDefender, lossRate float64, worldID uuid.UUID,
) error {
	totalsByOwner := map[uuid.UUID]int{}
	for _, d := range defenders {
		newSize := int(float64(d.size) * (1 - lossRate))
		lost := d.size - newSize
		totalsByOwner[d.ownerID] += lost

		if newSize <= 0 {
			if _, err := tx.Exec(ctx,
				`UPDATE units SET status = 'disbanded', size = 0, updated_at = now() WHERE id = $1`, d.id,
			); err != nil {
				slog.Warn("could not disband field defender", "unit", d.id, "err", err)
			}
		} else {
			if _, err := tx.Exec(ctx,
				`UPDATE units SET size = $2, updated_at = now() WHERE id = $1`, d.id, newSize,
			); err != nil {
				slog.Warn("could not reduce field defender size", "unit", d.id, "err", err)
			}
		}
	}

	for ownerID, lost := range totalsByOwner {
		if lost <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET population = GREATEST(50, population - $2)
			 WHERE owner_id = $1 AND world_id = $3 AND is_capital = true`,
			ownerID, lost, worldID,
		); err != nil {
			slog.Warn("could not apply field defender pop loss", "owner", ownerID, "err", err)
		}
	}
	return nil
}
