package combat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

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

// RespawnPayload is the scheduled_events payload for Respawn events.
type RespawnPayload struct {
	PlayerID uuid.UUID `json:"player_id"`
	WorldID  uuid.UUID `json:"world_id"`
	Culture  string    `json:"culture"`
}

// ArrivalHandler resolves army arrivals. Register with events.Worker at startup.
type ArrivalHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
	hub        *notify.Hub
	clk        clock.Clock
	scheduler  *events.Scheduler
}

// NewArrivalHandler creates an ArrivalHandler.
func NewArrivalHandler(pool *pgxpool.Pool, store *events.Store, hub *notify.Hub, clk clock.Clock, scheduler *events.Scheduler) *ArrivalHandler {
	return &ArrivalHandler{pool: pool, eventStore: store, hub: hub, clk: clk, scheduler: scheduler}
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
		ID         uuid.UUID
		OriginID   uuid.UUID
		TargetID   uuid.UUID
		Intent     string
		Army       province.ArmyComposition
		Resolved   bool
		ColonyName *string
	}
	err := tx.QueryRow(ctx,
		`SELECT id, origin_id, target_id, intent,
		        infantry, cavalry, catapult, priest, ship, elite_infantry, resolved, colony_name
		 FROM marching_armies WHERE id = $1 FOR UPDATE`,
		marchID,
	).Scan(&march.ID, &march.OriginID, &march.TargetID, &march.Intent,
		&march.Army.Infantry, &march.Army.Cavalry, &march.Army.Catapult,
		&march.Army.Priest, &march.Army.Ship, &march.Army.EliteInfantry, &march.Resolved,
		&march.ColonyName)
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

	if march.Intent == "colonize" {
		return h.colonize(ctx, tx, march.OriginID, march.TargetID, march.Army, worldID, march.ColonyName)
	}

	// Load attacker support from allied marches at same target.
	var supportStr float64
	_ = tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(infantry + elite_infantry*2 + cavalry*3), 0)
		 FROM marching_armies
		 WHERE target_id = $1 AND intent = 'support' AND resolved = false`,
		march.TargetID,
	).Scan(&supportStr)

	// Check for active battle frenzy on the attacker's home settlement.
	// If active: infantry fights at ×1.5 strength this battle; frenzy is consumed.
	var frenzySettlementID uuid.UUID
	_ = tx.QueryRow(ctx,
		`SELECT id FROM settlements
		 WHERE province_id = $1 AND battle_frenzy_until IS NOT NULL AND battle_frenzy_until > now()`,
		march.OriginID,
	).Scan(&frenzySettlementID)
	frenzied := frenzySettlementID != uuid.Nil
	if frenzied {
		_, _ = tx.Exec(ctx,
			`UPDATE settlements SET battle_frenzy_until = NULL WHERE id = $1`,
			frenzySettlementID,
		)
	}

	// Build effective attack army — frenzy inflates infantry strength for this battle only.
	effectiveArmy := march.Army
	if frenzied && effectiveArmy.Infantry > 0 {
		effectiveArmy.Infantry = int(float64(effectiveArmy.Infantry) * 1.5)
	}

	// Load defender settlement (looked up by province_id). Culture is used for respawn.
	var def struct {
		OwnerID        *uuid.UUID
		Army           province.ArmyComposition
		Walls          int
		InvasionsToday int
		Culture        string
	}
	err = tx.QueryRow(ctx,
		`SELECT owner_id, infantry, cavalry, catapult, priest, ship, elite_infantry, wall_level, invasions_today, culture_id
		 FROM settlements WHERE province_id = $1`,
		march.TargetID,
	).Scan(&def.OwnerID,
		&def.Army.Infantry, &def.Army.Cavalry, &def.Army.Catapult,
		&def.Army.Priest, &def.Army.Ship, &def.Army.EliteInfantry, &def.Walls, &def.InvasionsToday,
		&def.Culture)
	if err != nil {
		return fmt.Errorf("load defending settlement: %w", err)
	}

	result := Resolve(
		AttackForce{Army: effectiveArmy, SupportStrength: supportStr},
		DefenceForce{Army: def.Army, WallLevel: def.Walls},
	)

	// Scale defender losses down for repeated invasions on the same day.
	// Each prior invasion today reduces defender casualties by 25% (floor 25%).
	if def.InvasionsToday > 0 {
		scale := 1.0 - float64(def.InvasionsToday)*0.25
		if scale < 0.25 {
			scale = 0.25
		}
		result.DefenderLosses *= scale
	}

	// Increment invasion counter on target settlement.
	_, _ = tx.Exec(ctx,
		`UPDATE settlements SET invasions_today = invasions_today + 1 WHERE province_id = $1`,
		march.TargetID,
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
		// Queue respawn if the defeated player has no remaining settlements.
		if def.OwnerID != nil && h.scheduler != nil {
			var remaining int
			_ = tx.QueryRow(ctx,
				`SELECT COUNT(*) FROM settlements WHERE world_id = $1 AND owner_id = $2`,
				worldID, *def.OwnerID,
			).Scan(&remaining)
			if remaining == 0 {
				_ = h.scheduler.EnqueueAfterTx(ctx, tx, worldID, events.ScheduledRespawn,
					RespawnPayload{PlayerID: *def.OwnerID, WorldID: worldID, Culture: def.Culture},
					30*time.Second,
				)
			}
		}
	} else {
		if err := applyDefenderVictory(ctx, tx, march.OriginID, march.TargetID, march.Army, def.Army, result); err != nil {
			return err
		}
	}

	report := buildCombatReport(march.Army, def.Army, def.Walls, result, supportStr, frenzied)
	_, _ = tx.Exec(ctx, `UPDATE marching_armies SET combat_report = $1 WHERE id = $2`, report, march.ID)

	h.insertBattleGossip(ctx, tx, march.OriginID, march.TargetID, worldID, result.Outcome)
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

// buildCombatReport generates a human-readable battle summary stored once at resolution.
func buildCombatReport(att, def province.ArmyComposition, walls int, result CombatResult, support float64, frenzy bool) string {
	var sb strings.Builder

	armyStr := func(a province.ArmyComposition) string {
		parts := []string{}
		if a.Infantry > 0 {
			parts = append(parts, fmt.Sprintf("%d Hoplites", a.Infantry))
		}
		if a.EliteInfantry > 0 {
			parts = append(parts, fmt.Sprintf("%d Agema", a.EliteInfantry))
		}
		if a.Cavalry > 0 {
			parts = append(parts, fmt.Sprintf("%d Hippeis", a.Cavalry))
		}
		if a.Priest > 0 {
			parts = append(parts, fmt.Sprintf("%d Hiereus", a.Priest))
		}
		if a.Ship > 0 {
			parts = append(parts, fmt.Sprintf("%d Trireme", a.Ship))
		}
		if a.Catapult > 0 {
			parts = append(parts, fmt.Sprintf("%d Siege", a.Catapult))
		}
		if len(parts) == 0 {
			return "none"
		}
		return strings.Join(parts, " · ")
	}

	if frenzy {
		sb.WriteString("⚡ BATTLE FRENZY — infantry fights at ×1.5 strength\n")
	}
	sb.WriteString(fmt.Sprintf("ATTACKER  %s  [DP %.0f", armyStr(att), result.AttackStrength-support))
	if support > 0 {
		sb.WriteString(fmt.Sprintf(" + %.0f support", support))
	}
	if frenzy {
		sb.WriteString(" (frenzy)")
	}
	sb.WriteString(fmt.Sprintf(" = %.0f]\n", result.AttackStrength))

	wallMod := 1.0 + float64(walls)*0.25
	rawDef := result.DefenceStrength / wallMod
	sb.WriteString(fmt.Sprintf("DEFENDER  %s  [DP %.0f", armyStr(def), rawDef))
	if walls > 0 {
		sb.WriteString(fmt.Sprintf(" × walls L%d (×%.2f) = %.0f", walls, wallMod, result.DefenceStrength))
	}
	sb.WriteString("]\n")

	if result.Outcome == OutcomeAttackerWins {
		sb.WriteString(fmt.Sprintf("RESULT    Attacker victory  (%.0f vs %.0f)\n", result.AttackStrength, result.DefenceStrength))
		sb.WriteString(fmt.Sprintf("          Attacker losses %.0f%%  ·  Settlement captured", result.AttackerLosses*100))
	} else {
		sb.WriteString(fmt.Sprintf("RESULT    Defender holds  (%.0f vs %.0f)\n", result.AttackStrength, result.DefenceStrength))
		sb.WriteString(fmt.Sprintf("          Attacker losses %.0f%%  ·  Defender losses %.0f%%", result.AttackerLosses*100, result.DefenderLosses*100))
	}

	return sb.String()
}

// insertBattleGossip writes rumour events to players within 5 hexes of the battle,
// excluding the direct combatants who already receive WebSocket notifications.
func (h *ArrivalHandler) insertBattleGossip(ctx context.Context, tx pgx.Tx, originID, targetID, worldID uuid.UUID, outcome Outcome) {
	type provInfo struct {
		Name string
		Q, R int
	}
	var orig, tgt provInfo
	_ = tx.QueryRow(ctx,
		`SELECT COALESCE(s.name, 'unknown'), p.map_q, p.map_r
		 FROM provinces p LEFT JOIN settlements s ON s.province_id = p.id
		 WHERE p.id = $1`, originID,
	).Scan(&orig.Name, &orig.Q, &orig.R)
	_ = tx.QueryRow(ctx,
		`SELECT COALESCE(s.name, 'unknown'), p.map_q, p.map_r
		 FROM provinces p LEFT JOIN settlements s ON s.province_id = p.id
		 WHERE p.id = $1`, targetID,
	).Scan(&tgt.Name, &tgt.Q, &tgt.R)

	var text string
	if outcome == OutcomeAttackerWins {
		text = fmt.Sprintf("Rumour: A force from %s has seized %s. The province has changed hands.", orig.Name, tgt.Name)
	} else {
		text = fmt.Sprintf("Travellers speak of a failed assault on %s. The defenders held their ground.", tgt.Name)
	}

	var attOwner, defOwner uuid.UUID
	_ = tx.QueryRow(ctx, `SELECT COALESCE(owner_id, gen_random_uuid()) FROM settlements WHERE province_id = $1`, originID).Scan(&attOwner)
	_ = tx.QueryRow(ctx, `SELECT COALESCE(owner_id, gen_random_uuid()) FROM settlements WHERE province_id = $1`, targetID).Scan(&defOwner)

	// Direct notification for combatants (persists for offline players).
	var attText, defText string
	if outcome == OutcomeAttackerWins {
		attText = fmt.Sprintf("Your forces seized %s. The province is now yours.", tgt.Name)
		defText = fmt.Sprintf("Your settlement fell. Forces from %s broke through your defences.", orig.Name)
	} else {
		attText = fmt.Sprintf("Your assault on %s was repelled. Your forces withdrew.", tgt.Name)
		defText = fmt.Sprintf("Your settlement held. The attack from %s was beaten back.", orig.Name)
	}
	_, _ = tx.Exec(ctx,
		`INSERT INTO gossip_events (world_id, recipient_id, source_region, category, text) VALUES ($1,$2,$3,$4,$5)`,
		worldID, attOwner, tgt.Name, "battle", attText,
	)
	if defOwner != attOwner {
		_, _ = tx.Exec(ctx,
			`INSERT INTO gossip_events (world_id, recipient_id, source_region, category, text) VALUES ($1,$2,$3,$4,$5)`,
			worldID, defOwner, tgt.Name, "battle", defText,
		)
	}

	rows, err := tx.Query(ctx,
		`SELECT DISTINCT s.owner_id
		 FROM settlements s
		 JOIN provinces p ON p.id = s.province_id
		 WHERE p.world_id = $1
		   AND s.owner_id IS NOT NULL
		   AND (
		       (ABS(p.map_q - $2) + ABS(p.map_r - $3) + ABS((p.map_q + p.map_r) - ($2 + $3))) / 2 <= 5
		       OR
		       (ABS(p.map_q - $4) + ABS(p.map_r - $5) + ABS((p.map_q + p.map_r) - ($4 + $5))) / 2 <= 5
		   )`,
		worldID, orig.Q, orig.R, tgt.Q, tgt.R,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var recipID uuid.UUID
		if err := rows.Scan(&recipID); err != nil {
			continue
		}
		if recipID == attOwner || recipID == defOwner {
			continue
		}
		_, _ = tx.Exec(ctx,
			`INSERT INTO gossip_events (world_id, recipient_id, source_region, category, text)
			 VALUES ($1, $2, $3, $4, $5)`,
			worldID, recipID, tgt.Name, "battle", text,
		)
	}
}

// colonize creates a new colony settlement in an unclaimed province.
// If a settlement already exists, the colonists return home without creating a new one.
func (h *ArrivalHandler) colonize(ctx context.Context, tx pgx.Tx, originID, targetID uuid.UUID, army province.ArmyComposition, worldID uuid.UUID, chosenName *string) error {
	// Idempotency: if someone got here first, return army home.
	var existingID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM settlements WHERE province_id = $1`, targetID).Scan(&existingID); err == nil {
		return mergeArmy(ctx, tx, originID, army)
	}

	// Load attacker's owner and culture from home settlement.
	var attackerOwnerID uuid.UUID
	var culture string
	if err := tx.QueryRow(ctx,
		`SELECT owner_id, culture_id FROM settlements WHERE province_id = $1`,
		originID,
	).Scan(&attackerOwnerID, &culture); err != nil {
		return fmt.Errorf("load attacker settlement: %w", err)
	}

	// Load target province terrain and deposits.
	var terrainType string
	var copperDeposit, tinDeposit bool
	if err := tx.QueryRow(ctx,
		`SELECT terrain_type, copper_deposit, tin_deposit FROM provinces WHERE id = $1`,
		targetID,
	).Scan(&terrainType, &copperDeposit, &tinDeposit); err != nil {
		return fmt.Errorf("load target province: %w", err)
	}

	name := province.SettlementNameForCulture(culture)
	if chosenName != nil && *chosenName != "" {
		name = *chosenName
	}

	var settlementID uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO settlements
		 (world_id, province_id, name, culture_id, owner_id, control_type, is_capital,
		  loyalty, governor_is_ai, kharis_rate, kharis_calc_at)
		 VALUES ($1,$2,$3,$4,$5,'colony',false,2,true,0.02,now())
		 RETURNING id`,
		worldID, targetID, name, culture, attackerOwnerID,
	).Scan(&settlementID); err != nil {
		return fmt.Errorf("create colony: %w", err)
	}

	// Mark province as controlled.
	_, _ = tx.Exec(ctx,
		`UPDATE provinces SET territory_state='controlled', controller_id=$1 WHERE id=$2`,
		settlementID, targetID,
	)

	// Seed all goods rows (zero amounts), then apply terrain production rates.
	_, _ = tx.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
		 SELECT $1, g.key, 0, 0,
		        CASE g.key WHEN 'grain' THEN 1000 WHEN 'cedar' THEN 500 WHEN 'stone' THEN 1000
		                   WHEN 'copper' THEN 300  WHEN 'tin' THEN 300  ELSE 200 END,
		        now()
		 FROM goods g
		 ON CONFLICT (settlement_id, good_key) DO NOTHING`,
		settlementID,
	)
	_, _ = tx.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
		 SELECT $1, pr.good_key, 0, pr.rate_per_min,
		        CASE pr.good_key WHEN 'grain' THEN 1000 WHEN 'cedar' THEN 500 WHEN 'stone' THEN 1000
		                        WHEN 'copper' THEN 300  WHEN 'tin' THEN 300  ELSE 200 END,
		        now()
		 FROM production_rules pr
		 WHERE pr.building_type IS NULL AND pr.terrain_type = $2
		   AND (pr.requires_deposit IS NULL
		        OR (pr.requires_deposit = 'copper' AND $3)
		        OR (pr.requires_deposit = 'tin' AND $4))
		 ON CONFLICT (settlement_id, good_key) DO UPDATE
		     SET rate = settlement_goods.rate + EXCLUDED.rate`,
		settlementID, terrainType, copperDeposit, tinDeposit,
	)

	// Colonists become the garrison.
	if army.Infantry > 0 || army.Cavalry > 0 || army.EliteInfantry > 0 || army.Priest > 0 || army.Ship > 0 {
		_, _ = tx.Exec(ctx,
			`UPDATE settlements SET
			   infantry       = infantry       + $1,
			   cavalry        = cavalry        + $2,
			   elite_infantry = elite_infantry + $3,
			   priest         = priest         + $4,
			   ship           = ship           + $5
			 WHERE id = $6`,
			army.Infantry, army.Cavalry, army.EliteInfantry, army.Priest, army.Ship,
			settlementID,
		)
	}

	slog.Info("colony founded", "settlement", settlementID, "name", name, "province", targetID, "owner", attackerOwnerID)

	if h.hub != nil {
		h.hub.Broadcast(worldID, notify.Msg{
			Kind: "ColonyFounded",
			Payload: map[string]any{
				"settlement_id": settlementID,
				"name":          name,
				"province_id":   targetID,
			},
		})
	}

	// Gossip: inform nearby settlements.
	var origName string
	_ = tx.QueryRow(ctx, `SELECT name FROM settlements WHERE province_id = $1`, originID).Scan(&origName)
	_, _ = tx.Exec(ctx,
		`INSERT INTO gossip_events (world_id, recipient_id, source_region, category, text)
		 SELECT $1, s.owner_id, $2, 'political',
		        $3 || ' has been established near your domain.'
		 FROM settlements s
		 JOIN provinces p ON p.id = s.province_id
		 WHERE p.world_id = $1 AND s.owner_id IS NOT NULL AND s.owner_id != $4
		   AND (ABS(p.map_q - (SELECT map_q FROM provinces WHERE id = $5))
		        + ABS(p.map_r - (SELECT map_r FROM provinces WHERE id = $5))
		        + ABS((p.map_q + p.map_r) -
		              ((SELECT map_q FROM provinces WHERE id = $5)+(SELECT map_r FROM provinces WHERE id = $5)))) / 2 <= 5`,
		worldID, name, name, attackerOwnerID, targetID,
	)

	return nil
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
