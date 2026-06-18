package combat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/economy"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/province"
	"github.com/poleia/server/internal/timescale"
	"github.com/poleia/server/internal/unit"
)

// UnitArrivalHandler processes ScheduledUnitArrival events.
//
// When a marching unit arrives at its destination it either:
//   - Joins garrison (empty/own/allied hex)
//   - Triggers deterministic combat (enemy settlement present)
//
// Idempotency: the arriving unit is fetched with FOR UPDATE and the handler
// exits early if status != 'marching'. ON CONFLICT DO NOTHING is used for
// projection inserts. Re-running the handler is therefore safe.
//
// C5 stance effects (implemented):
//   - fortify: defending garrison units in fortify stance get ×1.5 strength.
//   - storm:   arriving unit with storm stance halves the wall-level bonus of the target.
//
// TODO C5-sentry: sentry interception is NOT yet active.
// Design: a periodic scan (e.g. ScheduledSentryScan every ~30 s, or a ticker
// in main.go mirroring kharis seedDailyTicks) iterates all marching units,
// computes their interpolated hex from departs_at/arrives_at on a straight line,
// and for each active sentry unit (status='positioned', stance='sentry') within
// 3 hex of sentry_q/r triggers an UnitIntercepted combat using resolveCombat.
// Guard: intercepted_at column on units (or a separate table) prevents the same
// march from being intercepted twice by the same sentry. Stub wired to
// SetStance (stance can be set to 'sentry', sentry_q/r is persisted), but no
// scan goroutine is started yet.
type UnitArrivalHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
	hub        Broadcaster
	scheduler  *events.Scheduler
	clk        clock.Clock
}

// NewUnitArrivalHandler creates a UnitArrivalHandler.
func NewUnitArrivalHandler(pool *pgxpool.Pool, store *events.Store, hub Broadcaster, scheduler *events.Scheduler, clk clock.Clock) *UnitArrivalHandler {
	return &UnitArrivalHandler{pool: pool, eventStore: store, hub: hub, scheduler: scheduler, clk: clk}
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
		        status, q, r, target_q, target_r, stance
		 FROM units WHERE id = $1 FOR UPDATE`,
		unitID,
	).Scan(&u.id, &u.ownerID, &u.utype, &u.category, &u.size, &u.crew, &u.cargoUnitID,
		&u.status, &u.q, &u.r, &u.targetQ, &u.targetR, &u.stance); err != nil {
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
		`SELECT s.id, s.owner_id, s.wall_level, p.id, p.terrain_type
		 FROM provinces p
		 LEFT JOIN settlements s ON s.province_id = p.id
		 WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3`,
		worldID, destQ, destR,
	).Scan(&dest.settlementID, &dest.ownerID, &dest.wallLevel,
		&dest.provinceID, &dest.terrain)

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
	// C5: units in fortify stance receive a +50% defensive multiplier (tunable).
	// Rationale: fortify represents dug-in defensive positions. Applied per-unit so
	// a mixed garrison gets partial benefit. Constant: fortifyBonus = 1.5.
	const fortifyBonus = 1.5
	var defUnitStr float64
	garrisonRows, err := tx.Query(ctx,
		`SELECT type, size, stance FROM units
		 WHERE settlement_id = $1 AND status = 'garrison' AND status != 'disbanded'`,
		*dest.settlementID,
	)
	if err == nil {
		for garrisonRows.Next() {
			var utype string
			var usize int
			var ustance *string
			if scanErr := garrisonRows.Scan(&utype, &usize, &ustance); scanErr == nil {
				str := unitStrength(utype, usize)
				// C5: fortify stance grants +50% defence.
				if ustance != nil && *ustance == "fortify" {
					str *= fortifyBonus
				}
				defUnitStr += str
			}
		}
		garrisonRows.Close()
	}

	// C5: storm stance halves the wall bonus for the attacker.
	// Normal wall multiplier: 1 + level×0.25.
	// Storm effective wall:   1 + level×0.25/2  (tunable; stormWallDivisor = 2.0).
	// Rationale: the attacking unit is carrying siege equipment and focusing on
	// breaching rather than holding field position. The bonus only reduces the wall
	// multiplier, not base unit-vs-unit strength.
	const stormWallDivisor = 2.0
	wallMod := WallModifier(dest.wallLevel)
	if u.stance != nil && *u.stance == "storm" {
		// Halve the extra bonus (the +0.25×level part); the base 1.0 is unchanged.
		extra := float64(dest.wallLevel) * 0.25
		wallMod = 1.0 + extra/stormWallDivisor
	}
	defStr := defUnitStr * wallMod

	// ── Fortune (W5): roll once, bias by kharis delta ─────────────────────────
	var attackerKharis, defenderKharis float64
	_ = tx.QueryRow(ctx,
		`SELECT GREATEST(0, settled(kharis_amount, kharis_rate, kharis_calc_at))
		 FROM player_world_records WHERE player_id = $1 AND world_id = $2`,
		u.ownerID, worldID,
	).Scan(&attackerKharis)
	if dest.ownerID != nil {
		_ = tx.QueryRow(ctx,
			`SELECT GREATEST(0, settled(kharis_amount, kharis_rate, kharis_calc_at))
			 FROM player_world_records WHERE player_id = $1 AND world_id = $2`,
			*dest.ownerID, worldID,
		).Scan(&defenderKharis)
	}
	fortune := rollFortune(attackerKharis, defenderKharis)
	attStrWithFortune := attStr * (1 + fortune)

	// ── Resolve ───────────────────────────────────────────────────────────────
	result := ResolveStrengths(attStrWithFortune, defStr, fortune)

	slog.Info("unit combat resolved",
		"unit", u.id, "q", destQ, "r", destR,
		"att", attStr, "fortune", fortune, "def", defStr, "outcome", result.Outcome,
		"rounds", result.Rounds, "att_routed", result.AttackerRouted)

	// ── Apply losses ──────────────────────────────────────────────────────────
	attSizeBefore := u.size
	attSizeAfter := int(float64(u.size) * (1 - result.AttackerLosses))
	attPopLost := attSizeBefore - attSizeAfter

	if result.Outcome == OutcomeAttackerWins {
		if err := h.applyAttackerWins(ctx, tx, u, dest, attSizeAfter, attPopLost, result, destQ, destR, worldID); err != nil {
			return err
		}
	} else {
		if err := h.applyDefenderWins(ctx, tx, u, dest, attSizeAfter, attPopLost, result, destQ, destR, worldID); err != nil {
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
			"unit_id":        u.id,
			"outcome":        string(result.Outcome),
			"att":            attStr,
			"fortune":        result.Fortune,
			"def":            defStr,
			"att_losses":     result.AttackerLosses,
			"def_losses":     result.DefenderLosses,
			"att_routed":     result.AttackerRouted,
			"def_routed":     result.DefenderRouted,
			"rounds":         result.Rounds,
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

// applyDefenderWins: arriving unit is beaten back, takes losses.
// If the unit routed (result.AttackerRouted), it survives with remaining men and
// marches back to its origin. If destroyed (size after losses = 0), it is disbanded.
func (h *UnitArrivalHandler) applyDefenderWins(
	ctx context.Context, tx pgx.Tx,
	u unitRow, dest destSettlement,
	attSizeAfter, attPopLost int,
	result CombatResult,
	destQ, destR int,
	worldID uuid.UUID,
) error {
	if attSizeAfter <= 0 {
		// Unit destroyed: demographic loss, unit disbanded.
		if _, err := tx.Exec(ctx,
			`UPDATE units SET status = 'disbanded', updated_at = now() WHERE id = $1`, u.id,
		); err != nil {
			return fmt.Errorf("disband zeroed attacker: %w", err)
		}
		h.disbandCargoIfPresent(ctx, tx, u, worldID)
	} else if result.AttackerRouted && h.scheduler != nil {
		// Rout (W5): unit survives with remaining men, retreats to origin.
		// origin = (u.q, u.r) — the hex the unit marched FROM (set by march handler).
		dist := province.HexDistance(
			province.MapPosition{Q: destQ, R: destR},
			province.MapPosition{Q: u.q, R: u.r},
		)
		if dist < 1 {
			dist = 1
		}
		var originTerrain string
		_ = tx.QueryRow(ctx,
			`SELECT terrain_type FROM provinces WHERE world_id = $1 AND map_q = $2 AND map_r = $3`,
			worldID, u.q, u.r,
		).Scan(&originTerrain)
		if originTerrain == "" {
			originTerrain = "plains"
		}
		moveHours := province.TerrainMoveHours(originTerrain) * float64(dist)
		arrivesAt := h.clk.Now().Add(timescale.Apply(time.Duration(moveHours * float64(time.Hour))))

		if _, err := tx.Exec(ctx,
			`UPDATE units SET
			   size        = $2,
			   status      = 'marching',
			   q           = $3,
			   r           = $4,
			   target_q    = $5,
			   target_r    = $6,
			   departs_at  = now(),
			   arrives_at  = $7,
			   settlement_id = NULL,
			   updated_at  = now()
			 WHERE id = $1`,
			u.id, attSizeAfter, destQ, destR, u.q, u.r, arrivesAt,
		); err != nil {
			return fmt.Errorf("route unit back to origin: %w", err)
		}
		arrPayload := unit.ScheduledUnitArrivalPayload{UnitID: u.id, WorldID: worldID}
		if err := h.scheduler.EnqueueTx(ctx, tx, worldID, events.ScheduledUnitArrival, arrPayload, arrivesAt); err != nil {
			return fmt.Errorf("schedule rout return march: %w", err)
		}
		slog.Info("unit routed, returning to origin", "unit", u.id, "origin_q", u.q, "origin_r", u.r, "size", attSizeAfter)
	} else {
		// Beaten (not routed) — unit disbanded, treat as destroyed.
		if _, err := tx.Exec(ctx,
			`UPDATE units SET status = 'disbanded', updated_at = now() WHERE id = $1`, u.id,
		); err != nil {
			return fmt.Errorf("disband beaten attacker: %w", err)
		}
		h.disbandCargoIfPresent(ctx, tx, u, worldID)
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
	q           int    // origin hex (set by march handler; used for rout routing)
	r           int
	targetQ     *int
	targetR     *int
	stance      *string // C5: fortify/storm/sentry or nil
}

type destSettlement struct {
	provinceID   uuid.UUID
	settlementID *uuid.UUID
	ownerID      *uuid.UUID
	wallLevel    int
	terrain      string
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


