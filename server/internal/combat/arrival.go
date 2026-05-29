package combat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/notify"
	"github.com/poleia/server/internal/province"
)

// ArmyArrivalPayload is the scheduled_events payload for ArmyArrival events.
type ArmyArrivalPayload struct {
	MarchingArmyID uuid.UUID `json:"marching_army_id"`
}

// ArrivalHandler resolves army arrivals. Register with events.Worker at startup.
type ArrivalHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
	hub        *notify.Hub
	clk        clock.Clock
}

// NewArrivalHandler creates an ArrivalHandler.
func NewArrivalHandler(pool *pgxpool.Pool, store *events.Store, hub *notify.Hub, clk clock.Clock) *ArrivalHandler {
	return &ArrivalHandler{pool: pool, eventStore: store, hub: hub, clk: clk}
}

// Handle processes a single ArmyArrival scheduled event.
func (h *ArrivalHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var payload ArmyArrivalPayload
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal arrival payload: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := h.resolve(ctx, tx, payload.MarchingArmyID, e.WorldID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (h *ArrivalHandler) resolve(ctx context.Context, tx pgx.Tx, marchID, worldID uuid.UUID) error {
	var march struct {
		ID       uuid.UUID
		OriginID uuid.UUID
		TargetID uuid.UUID
		Intent   string
		Army     province.ArmyComposition
		Resolved bool
	}
	err := tx.QueryRow(ctx,
		`SELECT id, origin_id, target_id, intent,
		        infantry, cavalry, catapult, priest, ship, elite_infantry, resolved
		 FROM marching_armies WHERE id = $1 FOR UPDATE`,
		marchID,
	).Scan(&march.ID, &march.OriginID, &march.TargetID, &march.Intent,
		&march.Army.Infantry, &march.Army.Cavalry, &march.Army.Catapult,
		&march.Army.Priest, &march.Army.Ship, &march.Army.EliteInfantry, &march.Resolved)
	if err != nil {
		return fmt.Errorf("load march: %w", err)
	}
	if march.Resolved {
		return nil // idempotent
	}

	if _, err := tx.Exec(ctx, `UPDATE marching_armies SET resolved = true WHERE id = $1`, march.ID); err != nil {
		return fmt.Errorf("mark resolved: %w", err)
	}

	if march.Intent == "reinforce" || march.Intent == "support" {
		return mergeArmy(ctx, tx, march.TargetID, march.Army)
	}

	// Load attacker support from allied marches at same target.
	var supportStr float64
	_ = tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(infantry + elite_infantry*2 + cavalry*3 + priest*2), 0)
		 FROM marching_armies
		 WHERE target_id = $1 AND intent = 'support' AND resolved = false`,
		march.TargetID,
	).Scan(&supportStr)

	// Load defender settlement (looked up by province_id).
	var def struct {
		OwnerID *uuid.UUID
		Army    province.ArmyComposition
		Walls   int
	}
	err = tx.QueryRow(ctx,
		`SELECT owner_id, infantry, cavalry, catapult, priest, ship, elite_infantry, wall_level
		 FROM settlements WHERE province_id = $1`,
		march.TargetID,
	).Scan(&def.OwnerID,
		&def.Army.Infantry, &def.Army.Cavalry, &def.Army.Catapult,
		&def.Army.Priest, &def.Army.Ship, &def.Army.EliteInfantry, &def.Walls)
	if err != nil {
		return fmt.Errorf("load defending settlement: %w", err)
	}

	result := Resolve(
		AttackForce{Army: march.Army, SupportStrength: supportStr},
		DefenceForce{Army: def.Army, WallLevel: def.Walls},
	)

	slog.Info("combat resolved",
		"march", march.ID, "target", march.TargetID,
		"outcome", result.Outcome,
		"att", result.AttackStrength, "def", result.DefenceStrength,
	)

	if result.Outcome == OutcomeAttackerWins {
		if err := applyAttackerVictory(ctx, tx, march.OriginID, march.TargetID, def.OwnerID, march.Army, result, worldID); err != nil {
			return err
		}
	} else {
		if err := applyDefenderVictory(ctx, tx, march.OriginID, march.TargetID, march.Army, def.Army, result); err != nil {
			return err
		}
	}

	h.recordEvent(ctx, march.TargetID, worldID, result, march.ID)
	if h.hub != nil {
		h.hub.Broadcast(worldID, notify.Msg{
			Kind: "ArmyArrival",
			Payload: map[string]any{
				"outcome":   result.Outcome,
				"target_id": march.TargetID,
				"origin_id": march.OriginID,
			},
		})
	}
	return nil
}

func applyAttackerVictory(ctx context.Context, tx pgx.Tx, originID, targetID uuid.UUID, defOwnerID *uuid.UUID, attackArmy province.ArmyComposition, result CombatResult, worldID uuid.UUID) error {
	survivingInf := int(float64(attackArmy.Infantry) * (1 - result.AttackerLosses))
	survivingCav := int(float64(attackArmy.Cavalry) * (1 - result.AttackerLosses))
	survivingElite := int(float64(attackArmy.EliteInfantry) * (1 - result.AttackerLosses))

	// Get attacker's owner from their settlement.
	var attackerOwnerID *uuid.UUID
	_ = tx.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE province_id = $1`, originID).Scan(&attackerOwnerID)

	// Settlement changes hands.
	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET
		   owner_id       = $2,
		   infantry       = $3,
		   cavalry        = $4,
		   elite_infantry = $5,
		   catapult       = 0,
		   priest         = 0,
		   state          = 'active',
		   kingdom_id     = NULL,
		   control_type   = 'occupied',
		   updated_at     = now()
		 WHERE province_id = $1`,
		targetID, attackerOwnerID, survivingInf, survivingCav, survivingElite,
	); err != nil {
		return fmt.Errorf("transfer settlement: %w", err)
	}

	// Province territory state follows.
	if _, err := tx.Exec(ctx,
		`UPDATE provinces SET territory_state = 'controlled' WHERE id = $1`, targetID,
	); err != nil {
		return fmt.Errorf("update territory state: %w", err)
	}

	// Units were already deducted when the march was sent — nothing to deduct here.

	// Mark old owner dispossessed.
	if defOwnerID != nil {
		_, _ = tx.Exec(ctx,
			`UPDATE player_world_records SET status = 'dispossessed', settlement_id = NULL
			 WHERE player_id = $1 AND world_id = $2`,
			*defOwnerID, worldID,
		)
	}
	return nil
}

func applyDefenderVictory(ctx context.Context, tx pgx.Tx, originID, targetID uuid.UUID, attackArmy, defArmy province.ArmyComposition, result CombatResult) error {
	survivingAttInf := int(float64(attackArmy.Infantry) * (1 - result.AttackerLosses))
	survivingAttCav := int(float64(attackArmy.Cavalry) * (1 - result.AttackerLosses))
	survivingAttElite := int(float64(attackArmy.EliteInfantry) * (1 - result.AttackerLosses))

	// Units were already deducted at march time — surviving attackers return home.
	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET
		   infantry       = infantry       + $1,
		   cavalry        = cavalry        + $2,
		   elite_infantry = elite_infantry + $3
		 WHERE province_id = $4`,
		survivingAttInf, survivingAttCav, survivingAttElite, originID,
	); err != nil {
		return fmt.Errorf("return survivors: %w", err)
	}

	// Defender takes losses.
	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET
		   infantry       = GREATEST(0, FLOOR(infantry       * $1)),
		   cavalry        = GREATEST(0, FLOOR(cavalry        * $1)),
		   elite_infantry = GREATEST(0, FLOOR(elite_infantry * $1))
		 WHERE province_id = $2`,
		1.0-result.DefenderLosses, targetID,
	); err != nil {
		return fmt.Errorf("apply defender losses: %w", err)
	}
	return nil
}

func mergeArmy(ctx context.Context, tx pgx.Tx, targetID uuid.UUID, army province.ArmyComposition) error {
	_, err := tx.Exec(ctx,
		`UPDATE settlements SET
		   infantry       = infantry       + $1,
		   cavalry        = cavalry        + $2,
		   catapult       = catapult       + $3,
		   priest         = priest         + $4,
		   ship           = ship           + $5,
		   elite_infantry = elite_infantry + $6
		 WHERE province_id = $7`,
		army.Infantry, army.Cavalry, army.Catapult, army.Priest, army.Ship, army.EliteInfantry, targetID,
	)
	return err
}

func (h *ArrivalHandler) recordEvent(ctx context.Context, streamID, worldID uuid.UUID, result CombatResult, marchID uuid.UUID) {
	_, err := h.eventStore.Append(ctx, streamID, events.StreamCombat, "CombatResolved", map[string]any{
		"outcome":          string(result.Outcome),
		"attack_strength":  result.AttackStrength,
		"defence_strength": result.DefenceStrength,
		"attacker_losses":  result.AttackerLosses,
		"defender_losses":  result.DefenderLosses,
		"march_id":         marchID,
		"resolved_at":      h.clk.Now(),
	}, worldID, nil)
	if err != nil {
		slog.Error("record combat event", "err", err)
	}
}
