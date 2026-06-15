package combat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/economy"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/unit"
)

// UnitArrivalHandler processes ScheduledUnitArrival events (C4).
//
// When a marching unit arrives at its destination it either:
//   - Joins garrison (empty/own/allied hex)
//   - Triggers deterministic combat (enemy settlement present)
//
// Dual-read: defence strength is computed from BOTH units-table garrison units
// AND the legacy integer army columns on settlements. This ensures that units
// trained via C2 and settlements using old integer columns both contribute to
// defence during the dual-write transition period (until C8).
//
// Idempotency: the arriving unit is fetched with FOR UPDATE and the handler
// exits early if status != 'marching'. ON CONFLICT DO NOTHING is used for
// projection inserts. Re-running the handler is therefore safe.
type UnitArrivalHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
	hub        Broadcaster
}

// NewUnitArrivalHandler creates a UnitArrivalHandler.
func NewUnitArrivalHandler(pool *pgxpool.Pool, store *events.Store, hub Broadcaster) *UnitArrivalHandler {
	return &UnitArrivalHandler{pool: pool, eventStore: store, hub: hub}
}

// Handle processes one ScheduledUnitArrival scheduled event.
func (h *UnitArrivalHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var payload unit.ScheduledUnitArrivalPayload
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal unit arrival payload: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := h.resolve(ctx, tx, payload.UnitID, payload.WorldID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (h *UnitArrivalHandler) resolve(ctx context.Context, tx pgx.Tx, unitID, worldID uuid.UUID) error {
	// Load arriving unit with FOR UPDATE — idempotency guard.
	var u unitRow
	if err := tx.QueryRow(ctx,
		`SELECT id, owner_id, type, category, size, crew, cargo_unit_id,
		        status, target_q, target_r
		 FROM units WHERE id = $1 FOR UPDATE`,
		unitID,
	).Scan(&u.id, &u.ownerID, &u.utype, &u.category, &u.size, &u.crew, &u.cargoUnitID,
		&u.status, &u.targetQ, &u.targetR); err != nil {
		return fmt.Errorf("load arriving unit: %w", err)
	}

	// Idempotent: already resolved.
	if u.status != "marching" {
		return nil
	}
	if u.targetQ == nil || u.targetR == nil {
		return fmt.Errorf("unit %s has no target coordinates", unitID)
	}

	destQ, destR := *u.targetQ, *u.targetR

	// Find settlement at destination (if any).
	var dest destSettlement
	err := tx.QueryRow(ctx,
		`SELECT s.id, s.owner_id, s.wall_level,
		        s.infantry, s.chariot, s.elite_infantry,
		        s.ship, s.war_galley, s.merchantman,
		        p.id
		 FROM provinces p
		 LEFT JOIN settlements s ON s.province_id = p.id
		 WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3`,
		worldID, destQ, destR,
	).Scan(&dest.settlementID, &dest.ownerID,
		&dest.wallLevel,
		&dest.legacyInfantry, &dest.legacyChariot, &dest.legacyElite,
		&dest.legacyShip, &dest.legacyWarGalley, &dest.legacyMerchantman,
		&dest.provinceID)

	hasSettlement := err == nil && dest.settlementID != nil

	// No settlement or uncontested → become garrison.
	if !hasSettlement || dest.ownerID == nil || *dest.ownerID == u.ownerID {
		return h.arriveGarrison(ctx, tx, u, destQ, destR, dest.settlementID, worldID)
	}

	// Enemy settlement present — fight!
	return h.resolveCombat(ctx, tx, u, dest, destQ, destR, worldID)
}

// arriveGarrison places the unit at the destination as a garrison unit.
// If the unit is a naval vessel with cargo, the cargo land unit's position is
// updated to match the ship's destination (C6: cargo follows the ship).
func (h *UnitArrivalHandler) arriveGarrison(
	ctx context.Context, tx pgx.Tx,
	u unitRow, destQ, destR int, settlementID *uuid.UUID, worldID uuid.UUID,
) error {
	newStatus := "garrison"
	if settlementID == nil {
		newStatus = "positioned" // unit on the map without a settlement
	}

	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   status        = $2,
		   q             = $3,
		   r             = $4,
		   settlement_id = $5,
		   target_q      = NULL,
		   target_r      = NULL,
		   departs_at    = NULL,
		   arrives_at    = NULL,
		   updated_at    = now()
		 WHERE id = $1`,
		u.id, newStatus, destQ, destR, settlementID,
	); err != nil {
		return fmt.Errorf("unit arrive garrison: %w", err)
	}

	// C6: if this ship carried a land unit, move the cargo to the ship's new position.
	// The cargo stays 'embarked' — the Wanax must explicitly /unload to deploy it.
	if u.cargoUnitID != nil {
		if _, err := tx.Exec(ctx,
			`UPDATE units SET
			   q             = $2,
			   r             = $3,
			   settlement_id = $4,
			   updated_at    = now()
			 WHERE id = $1 AND status = 'embarked'`,
			*u.cargoUnitID, destQ, destR, settlementID,
		); err != nil {
			// Non-fatal: log and continue (cargo ghost is bad but arrival must not fail).
			slog.Warn("C6: could not update cargo unit position on ship arrival",
				"ship", u.id, "cargo", *u.cargoUnitID, "err", err)
		} else {
			slog.Info("C6: cargo unit position updated with ship", "ship", u.id, "cargo", *u.cargoUnitID, "q", destQ, "r", destR)
		}
	}

	_, _ = h.eventStore.Append(ctx, u.id, events.StreamType(unit.StreamUnit), unit.EventUnitArrived,
		unit.UnitArrivedPayload{
			UnitID:    u.id,
			Q:         destQ,
			R:         destR,
			NewStatus: newStatus,
		}, worldID, nil)

	slog.Info("unit arrived (garrison)", "unit", u.id, "q", destQ, "r", destR, "status", newStatus)
	return nil
}

// resolveCombat handles the arriving unit attacking an enemy settlement.
//
// Dual-read defence:
//   units at destination (unit.SummaryAtHex / ListBySettlement) +
//   legacy integer columns on settlement.
//
// Idempotency: u.status == 'marching' was checked at top; all writes are
// conditional on status or use ON CONFLICT DO NOTHING.
func (h *UnitArrivalHandler) resolveCombat(
	ctx context.Context, tx pgx.Tx,
	u unitRow, dest destSettlement, destQ, destR int, worldID uuid.UUID,
) error {
	// ── Attack strength ────────────────────────────────────────────────────────
	attStr := unitStrength(u.utype, u.size)

	// ── Defence strength: dual-read ────────────────────────────────────────────
	// 1. Units-table garrison at destination.
	var defUnitStr float64
	garrisonRows, err := tx.Query(ctx,
		`SELECT type, size FROM units
		 WHERE settlement_id = $1 AND status = 'garrison' AND status != 'disbanded'`,
		*dest.settlementID,
	)
	if err == nil {
		for garrisonRows.Next() {
			var utype string
			var usize int
			if scanErr := garrisonRows.Scan(&utype, &usize); scanErr == nil {
				defUnitStr += unitStrength(utype, usize)
			}
		}
		garrisonRows.Close()
	}

	// 2. Legacy integer columns (dual-read for backward compat during C2→C4 transition).
	legacyStr := float64(dest.legacyInfantry*1 + dest.legacyElite*2 + dest.legacyChariot*3 +
		dest.legacyWarGalley*3 + dest.legacyShip*1)

	// Take the maximum to avoid double-counting when both sources represent the
	// same garrison (C2 dual-writes both). Once C4 is the sole writer, units-table
	// will always dominate and legacy will be 0.
	defBase := defUnitStr
	if legacyStr > defUnitStr {
		defBase = legacyStr
	}

	wallMod := WallModifier(dest.wallLevel)
	defStr := defBase * wallMod

	// ── Resolve ───────────────────────────────────────────────────────────────
	var result CombatResult
	if attStr > defStr {
		result.Outcome = OutcomeAttackerWins
		result.AttackerLosses = 0.1 + 0.3*(defStr/maxF(attStr, 1))
		result.DefenderLosses = 0.5 + 0.4*(attStr/maxF(defStr, 1))
		if result.DefenderLosses > 1.0 {
			result.DefenderLosses = 1.0
		}
	} else {
		result.Outcome = OutcomeDefenderWins
		result.DefenderLosses = 0.05 + 0.15*(attStr/maxF(defStr, 1))
		result.AttackerLosses = 0.4 + 0.5*(defStr/maxF(attStr, 1))
		if result.AttackerLosses > 1.0 {
			result.AttackerLosses = 1.0
		}
	}
	result.AttackStrength = attStr
	result.DefenceStrength = defStr

	slog.Info("unit combat resolved",
		"unit", u.id, "q", destQ, "r", destR,
		"att", attStr, "def", defStr, "outcome", result.Outcome)

	// ── Apply losses ──────────────────────────────────────────────────────────
	attSizeBefore := u.size
	attSizeAfter := int(float64(u.size) * (1 - result.AttackerLosses))
	attPopLost := attSizeBefore - attSizeAfter

	if result.Outcome == OutcomeAttackerWins {
		if err := h.applyAttackerWins(ctx, tx, u, dest, attSizeAfter, attPopLost, result, destQ, destR, worldID); err != nil {
			return err
		}
	} else {
		if err := h.applyDefenderWins(ctx, tx, u, dest, attSizeAfter, attPopLost, result, worldID); err != nil {
			return err
		}
	}

	// Append combat events for both sides.
	_, _ = h.eventStore.Append(ctx, u.id, events.StreamType(unit.StreamUnit), unit.EventUnitCombatResolved,
		unit.UnitCombatResolvedPayload{
			UnitID:     u.id,
			Role:       "attacker",
			SizeBefore: attSizeBefore,
			SizeAfter:  attSizeAfter,
			Outcome:    string(result.Outcome),
			PopLost:    attPopLost,
		}, worldID, nil)

	_, _ = h.eventStore.Append(ctx, dest.provinceID, events.StreamCombat, "UnitCombatResolved",
		map[string]any{
			"unit_id":     u.id,
			"outcome":     string(result.Outcome),
			"att":         attStr,
			"def":         defStr,
			"att_losses":  result.AttackerLosses,
			"def_losses":  result.DefenderLosses,
		}, worldID, nil)

	return nil
}

// disbandCargoIfPresent disbands a naval unit's cargo land unit when the ship is
// destroyed in combat (C6). The cargo unit is marked 'disbanded'; men are
// permanently lost (demographic cost applied to their owner's capital).
// Non-fatal: errors are logged but do not abort the calling operation.
func (h *UnitArrivalHandler) disbandCargoIfPresent(ctx context.Context, tx pgx.Tx, ship unitRow, worldID uuid.UUID) {
	if ship.cargoUnitID == nil {
		return
	}
	cargoID := *ship.cargoUnitID
	// Mark cargo disbanded.
	if _, err := tx.Exec(ctx,
		`UPDATE units SET status = 'disbanded', updated_at = now()
		 WHERE id = $1 AND status = 'embarked'`,
		cargoID,
	); err != nil {
		slog.Warn("C6: could not disband cargo unit after ship loss", "ship", ship.id, "cargo", cargoID, "err", err)
		return
	}
	// Demographic loss: fetch cargo size and apply to owner's capital.
	var cargoSize int
	var cargoOwnerID uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT size, owner_id FROM units WHERE id = $1`, cargoID,
	).Scan(&cargoSize, &cargoOwnerID); err != nil {
		slog.Warn("C6: could not read cargo unit for pop loss", "cargo", cargoID, "err", err)
		return
	}
	if cargoSize > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET
			   population = GREATEST(50, population - $2)
			 WHERE owner_id = $1 AND world_id = $3 AND is_capital = true`,
			cargoOwnerID, cargoSize, worldID,
		); err != nil {
			slog.Warn("C6: could not apply cargo pop loss", "cargo", cargoID, "err", err)
		}
	}
	slog.Info("C6: cargo unit disbanded after ship destruction", "ship", ship.id, "cargo", cargoID, "men_lost", cargoSize)
}

// applyAttackerWins: arriving unit captures the settlement; defender units take losses.
func (h *UnitArrivalHandler) applyAttackerWins(
	ctx context.Context, tx pgx.Tx,
	u unitRow, dest destSettlement,
	attSizeAfter, attPopLost int,
	result CombatResult,
	destQ, destR int, worldID uuid.UUID,
) error {
	// Apply attacker losses to the arriving unit.
	if attSizeAfter <= 0 {
		// Attacker destroyed (shouldn't happen on win, but be safe).
		if _, err := tx.Exec(ctx,
			`UPDATE units SET status = 'disbanded', updated_at = now() WHERE id = $1`, u.id,
		); err != nil {
			return fmt.Errorf("disband zeroed attacker: %w", err)
		}
		// C6: if this ship had cargo, disband the cargo too.
		h.disbandCargoIfPresent(ctx, tx, u, worldID)
	} else {
		// Attacker becomes new garrison.
		if _, err := tx.Exec(ctx,
			`UPDATE units SET
			   size         = $2,
			   status       = 'garrison',
			   q            = $3,
			   r            = $4,
			   settlement_id = $5,
			   target_q     = NULL,
			   target_r     = NULL,
			   departs_at   = NULL,
			   arrives_at   = NULL,
			   updated_at   = now()
			 WHERE id = $1`,
			u.id, attSizeAfter, destQ, destR, dest.settlementID,
		); err != nil {
			return fmt.Errorf("place attacker garrison: %w", err)
		}
	}

	// Demographic cost: attacker's origin settlement loses the dead men.
	// We look up the attacker's home settlement by owner.
	if attPopLost > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET
			   population = GREATEST(50, population - $2)
			 WHERE owner_id = $1 AND world_id = $3 AND is_capital = true`,
			u.ownerID, attPopLost, worldID,
		); err != nil {
			slog.Warn("could not apply attacker pop loss", "unit", u.id, "lost", attPopLost, "err", err)
		}
	}

	// Defender garrison units: apply losses and disband zeroed units.
	if err := h.applyDefenderUnitLosses(ctx, tx, *dest.settlementID, result.DefenderLosses, worldID); err != nil {
		return err
	}

	// Defender legacy columns: scale down proportionally.
	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET
		   infantry       = GREATEST(0, FLOOR(infantry       * $1)),
		   chariot        = GREATEST(0, FLOOR(chariot        * $1)),
		   elite_infantry = GREATEST(0, FLOOR(elite_infantry * $1)),
		   ship           = GREATEST(0, FLOOR(ship           * $1)),
		   war_galley     = GREATEST(0, FLOOR(war_galley     * $1)),
		   merchantman    = GREATEST(0, FLOOR(merchantman    * $1))
		 WHERE id = $2`,
		1.0-result.DefenderLosses, *dest.settlementID,
	); err != nil {
		return fmt.Errorf("apply legacy defender losses (attacker win): %w", err)
	}

	// Also subtract dual-write column for the arriving unit's losses.
	colName := unitTypeToColumn(u.utype)
	if colName != "" && attPopLost > 0 {
		// Reduce attacker's home settlement's legacy integer column.
		if _, err := tx.Exec(ctx,
			fmt.Sprintf(`UPDATE settlements SET %s = GREATEST(0, %s - $1)
			 WHERE owner_id = $2 AND world_id = $3 AND is_capital = true`, colName, colName),
			attPopLost, u.ownerID, worldID,
		); err != nil {
			slog.Warn("could not dual-write attacker legacy col loss", "col", colName, "err", err)
		}
	}

	// Transfer settlement ownership.
	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET
		   owner_id     = $2,
		   control_type = 'occupied',
		   kingdom_id   = NULL,
		   updated_at   = now()
		 WHERE id = $1`,
		*dest.settlementID, u.ownerID,
	); err != nil {
		return fmt.Errorf("transfer settlement ownership: %w", err)
	}

	// Update province territory state.
	if _, err := tx.Exec(ctx,
		`UPDATE provinces SET territory_state = 'controlled' WHERE id = $1`, dest.provinceID,
	); err != nil {
		return fmt.Errorf("update territory state: %w", err)
	}

	// Mark old owner dispossessed.
	if dest.ownerID != nil {
		_, _ = tx.Exec(ctx,
			`UPDATE player_world_records SET status = 'dispossessed', settlement_id = NULL
			 WHERE player_id = $1 AND world_id = $2`,
			*dest.ownerID, worldID,
		)
		// Recompute production for defender (pop changed).
		var defSett uuid.UUID
		if err := tx.QueryRow(ctx,
			`SELECT id FROM settlements WHERE owner_id = $1 AND world_id = $2 AND is_capital = true`,
			*dest.ownerID, worldID,
		).Scan(&defSett); err == nil {
			_ = economy.RecomputeProduction(ctx, tx, defSett)
		}
	}

	_, _ = h.eventStore.Append(ctx, dest.provinceID, events.StreamType(unit.StreamUnit), unit.EventUnitArrived,
		unit.UnitArrivedPayload{
			UnitID:    u.id,
			Q:         destQ,
			R:         destR,
			NewStatus: "garrison",
		}, worldID, nil)

	return nil
}

// applyDefenderWins: arriving unit is beaten back, takes losses, may be destroyed.
func (h *UnitArrivalHandler) applyDefenderWins(
	ctx context.Context, tx pgx.Tx,
	u unitRow, dest destSettlement,
	attSizeAfter, attPopLost int,
	result CombatResult,
	worldID uuid.UUID,
) error {
	if attSizeAfter <= 0 {
		// Unit destroyed: demographic loss, unit disbanded.
		if _, err := tx.Exec(ctx,
			`UPDATE units SET status = 'disbanded', updated_at = now() WHERE id = $1`, u.id,
		); err != nil {
			return fmt.Errorf("disband zeroed attacker: %w", err)
		}
		// C6: if this ship had cargo, disband the cargo too.
		h.disbandCargoIfPresent(ctx, tx, u, worldID)
		// Dual-write: remove from legacy column at attacker's home.
		colName := unitTypeToColumn(u.utype)
		if colName != "" {
			_, _ = tx.Exec(ctx,
				fmt.Sprintf(`UPDATE settlements SET %s = GREATEST(0, %s - $1)
				 WHERE owner_id = $2 AND world_id = $3 AND is_capital = true`, colName, colName),
				u.size, u.ownerID, worldID,
			)
		}
	} else {
		// Attacker survives but reduced; they have already left their settlement, so
		// they land at their current position (q/r) as positioned.
		// For simplicity in C3/C4, a beaten attacker is disbanded (they never
		// established a stable position). Return men to population.
		// TODO C5: implement "retreat" to origin instead of disband.
		if _, err := tx.Exec(ctx,
			`UPDATE units SET status = 'disbanded', updated_at = now() WHERE id = $1`, u.id,
		); err != nil {
			return fmt.Errorf("disband beaten attacker: %w", err)
		}
		// C6: if this ship had cargo, disband the cargo too.
		h.disbandCargoIfPresent(ctx, tx, u, worldID)
		colName := unitTypeToColumn(u.utype)
		if colName != "" {
			_, _ = tx.Exec(ctx,
				fmt.Sprintf(`UPDATE settlements SET %s = GREATEST(0, %s - $1)
				 WHERE owner_id = $2 AND world_id = $3 AND is_capital = true`, colName, colName),
				u.size, u.ownerID, worldID,
			)
		}
	}

	// Permanent demographic loss for attacker.
	if attPopLost > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET
			   population = GREATEST(50, population - $2)
			 WHERE owner_id = $1 AND world_id = $3 AND is_capital = true`,
			u.ownerID, attPopLost, worldID,
		); err != nil {
			slog.Warn("could not apply attacker pop loss (def win)", "unit", u.id, "err", err)
		}
	}

	// Defender unit losses.
	if dest.settlementID != nil {
		if err := h.applyDefenderUnitLosses(ctx, tx, *dest.settlementID, result.DefenderLosses, worldID); err != nil {
			return err
		}
		// Defender legacy columns.
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET
			   infantry       = GREATEST(0, FLOOR(infantry       * $1)),
			   chariot        = GREATEST(0, FLOOR(chariot        * $1)),
			   elite_infantry = GREATEST(0, FLOOR(elite_infantry * $1))
			 WHERE id = $2`,
			1.0-result.DefenderLosses, *dest.settlementID,
		); err != nil {
			return fmt.Errorf("apply legacy defender losses (def win): %w", err)
		}
	}

	return nil
}

// applyDefenderUnitLosses reduces sizes of garrison units at a settlement and
// disbands those that reach 0. Also applies demographic cost to defender pop.
func (h *UnitArrivalHandler) applyDefenderUnitLosses(
	ctx context.Context, tx pgx.Tx,
	settlementID uuid.UUID, lossRate float64, worldID uuid.UUID,
) error {
	rows, err := tx.Query(ctx,
		`SELECT id, type, size, cargo_unit_id FROM units WHERE settlement_id = $1 AND status = 'garrison'`,
		settlementID,
	)
	if err != nil {
		return fmt.Errorf("load defender units: %w", err)
	}
	type defUnit struct {
		id          uuid.UUID
		utype       string
		size        int
		cargoUnitID *uuid.UUID
	}
	var defUnits []defUnit
	for rows.Next() {
		var du defUnit
		if scanErr := rows.Scan(&du.id, &du.utype, &du.size, &du.cargoUnitID); scanErr == nil {
			defUnits = append(defUnits, du)
		}
	}
	rows.Close()

	// Load owner for demographic loss.
	var defOwnerID uuid.UUID
	_ = tx.QueryRow(ctx, `SELECT COALESCE(owner_id, gen_random_uuid()) FROM settlements WHERE id = $1`, settlementID).Scan(&defOwnerID)

	totalPopLost := 0
	for _, du := range defUnits {
		newSize := int(float64(du.size) * (1 - lossRate))
		lost := du.size - newSize
		totalPopLost += lost

		if newSize <= 0 {
			if _, err := tx.Exec(ctx,
				`UPDATE units SET status = 'disbanded', size = 0, updated_at = now() WHERE id = $1`, du.id,
			); err != nil {
				slog.Warn("could not disband defender unit", "unit", du.id, "err", err)
			}
			// C6: if this defender ship had cargo, disband the cargo too.
			if du.cargoUnitID != nil {
				shipRow := unitRow{id: du.id, ownerID: defOwnerID, cargoUnitID: du.cargoUnitID}
				h.disbandCargoIfPresent(ctx, tx, shipRow, worldID)
			}
		} else {
			if _, err := tx.Exec(ctx,
				`UPDATE units SET size = $2, updated_at = now() WHERE id = $1`, du.id, newSize,
			); err != nil {
				slog.Warn("could not reduce defender unit size", "unit", du.id, "err", err)
			}
		}
	}

	// Defender demographic loss.
	if totalPopLost > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET population = GREATEST(50, population - $1) WHERE id = $2`,
			totalPopLost, settlementID,
		); err != nil {
			slog.Warn("could not apply defender pop loss", "settlement", settlementID, "err", err)
		}
		if err := economy.RecomputeProduction(ctx, tx, settlementID); err != nil {
			slog.Warn("recompute production after defender losses", "settlement", settlementID, "err", err)
		}
	}

	return nil
}

// ── Internal types ─────────────────────────────────────────────────────────────

type unitRow struct {
	id          uuid.UUID
	ownerID     uuid.UUID
	utype       string
	category    string
	size        int
	crew        int
	cargoUnitID *uuid.UUID
	status      string
	targetQ     *int
	targetR     *int
}

type destSettlement struct {
	provinceID         uuid.UUID
	settlementID       *uuid.UUID
	ownerID            *uuid.UUID
	wallLevel          int
	legacyInfantry     int
	legacyChariot      int
	legacyElite        int
	legacyShip         int
	legacyWarGalley    int
	legacyMerchantman  int
}

// ── Strength helpers ───────────────────────────────────────────────────────────

// unitStrength computes the combat contribution of one unit row.
// Multipliers (per man / per vessel): infantry ×1, elite ×2, chariot ×3,
// galley ×1, war_galley ×3, priest/merchantman ×0.
func unitStrength(utype string, size int) float64 {
	switch utype {
	case "infantry":
		return float64(size) * 1
	case "elite_infantry":
		return float64(size) * 2
	case "chariot":
		return float64(size) * 3
	case "galley", "ship":
		return float64(size) * 1
	case "war_galley":
		return float64(size) * 3
	case "priest", "merchantman":
		return 0
	default:
		return float64(size) * 1
	}
}

// unitTypeToColumn maps a unit type to its legacy integer column name.
func unitTypeToColumn(utype string) string {
	switch utype {
	case "infantry":
		return "infantry"
	case "elite_infantry":
		return "elite_infantry"
	case "chariot":
		return "chariot"
	case "priest":
		return "priest"
	case "galley", "ship":
		return "ship"
	case "war_galley":
		return "war_galley"
	case "merchantman":
		return "merchantman"
	}
	return ""
}

// maxF is a local float64 max helper (avoids importing math for a single call).
func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
