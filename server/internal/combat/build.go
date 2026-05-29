package combat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/notify"
	"github.com/poleia/server/internal/province"
)

// BuildCompletePayload is the scheduled event payload for a completed building.
type BuildCompletePayload struct {
	SettlementID uuid.UUID `json:"settlement_id"`
	BuildQueueID uuid.UUID `json:"build_queue_id"`
	BuildingType string    `json:"building_type"`
}

// BuildCompleteHandler resolves a completed building construction.
type BuildCompleteHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
	hub        *notify.Hub
}

// NewBuildCompleteHandler creates a BuildCompleteHandler.
func NewBuildCompleteHandler(pool *pgxpool.Pool, eventStore *events.Store, hub *notify.Hub) *BuildCompleteHandler {
	return &BuildCompleteHandler{pool: pool, eventStore: eventStore, hub: hub}
}

// Handle processes a ScheduledBuildComplete event.
func (h *BuildCompleteHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p BuildCompletePayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal build payload: %w", err)
	}

	spec, ok := province.BuildingSpecs[province.BuildingType(p.BuildingType)]
	if !ok {
		return fmt.Errorf("unknown building type: %s", p.BuildingType)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Verify queue entry still exists (idempotency guard).
	var existingID uuid.UUID
	err = tx.QueryRow(ctx,
		`SELECT id FROM build_queue WHERE id = $1 AND settlement_id = $2`,
		p.BuildQueueID, p.SettlementID,
	).Scan(&existingID)
	if err != nil {
		return nil // Already resolved.
	}

	// Insert completed building into settlement.
	_, err = tx.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, $2, 1)
		 ON CONFLICT (settlement_id, building_type)
		 DO UPDATE SET level = buildings.level + 1`,
		p.SettlementID, p.BuildingType,
	)
	if err != nil {
		return fmt.Errorf("insert building: %w", err)
	}

	// Apply rate bonuses to settlement resources (additive per level).
	if spec.FoodRate > 0 {
		_, err = tx.Exec(ctx,
			`UPDATE settlements SET
			   food_amount = food_amount + (EXTRACT(EPOCH FROM (now() - food_calc_at))/60 * food_rate),
			   food_rate = food_rate + $1,
			   food_calc_at = now()
			 WHERE id = $2`,
			spec.FoodRate, p.SettlementID)
		if err != nil {
			return fmt.Errorf("update food rate: %w", err)
		}
	}
	if spec.LumberRate > 0 {
		_, err = tx.Exec(ctx,
			`UPDATE settlements SET
			   lumber_amount = lumber_amount + (EXTRACT(EPOCH FROM (now() - lumber_calc_at))/60 * lumber_rate),
			   lumber_rate = lumber_rate + $1,
			   lumber_calc_at = now()
			 WHERE id = $2`,
			spec.LumberRate, p.SettlementID)
		if err != nil {
			return fmt.Errorf("update lumber rate: %w", err)
		}
	}
	if spec.StoneRate > 0 {
		_, err = tx.Exec(ctx,
			`UPDATE settlements SET
			   stone_amount = stone_amount + (EXTRACT(EPOCH FROM (now() - stone_calc_at))/60 * stone_rate),
			   stone_rate = stone_rate + $1,
			   stone_calc_at = now()
			 WHERE id = $2`,
			spec.StoneRate, p.SettlementID)
		if err != nil {
			return fmt.Errorf("update stone rate: %w", err)
		}
	}
	if spec.IronRate > 0 {
		_, err = tx.Exec(ctx,
			`UPDATE settlements SET
			   iron_amount = iron_amount + (EXTRACT(EPOCH FROM (now() - iron_calc_at))/60 * iron_rate),
			   iron_rate = iron_rate + $1,
			   iron_calc_at = now()
			 WHERE id = $2`,
			spec.IronRate, p.SettlementID)
		if err != nil {
			return fmt.Errorf("update iron rate: %w", err)
		}
	}
	if spec.GoldRate > 0 {
		_, err = tx.Exec(ctx,
			`UPDATE settlements SET
			   gold_amount = gold_amount + (EXTRACT(EPOCH FROM (now() - gold_calc_at))/60 * gold_rate),
			   gold_rate = gold_rate + $1,
			   gold_calc_at = now()
			 WHERE id = $2`,
			spec.GoldRate, p.SettlementID)
		if err != nil {
			return fmt.Errorf("update gold rate: %w", err)
		}
	}
	if spec.KharisRate > 0 {
		_, err = tx.Exec(ctx,
			`UPDATE settlements SET
			   kharis_amount = kharis_amount + (EXTRACT(EPOCH FROM (now() - kharis_calc_at))/60 * kharis_rate),
			   kharis_rate = kharis_rate + $1,
			   kharis_calc_at = now()
			 WHERE id = $2`,
			spec.KharisRate, p.SettlementID)
		if err != nil {
			return fmt.Errorf("update kharis rate: %w", err)
		}
	}
	if spec.WallsBonus > 0 {
		_, err = tx.Exec(ctx,
			`UPDATE settlements SET wall_level = LEAST(wall_level + $1, 3) WHERE id = $2`,
			spec.WallsBonus, p.SettlementID)
		if err != nil {
			return fmt.Errorf("update wall level: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM build_queue WHERE id = $1`, p.BuildQueueID); err != nil {
		return fmt.Errorf("delete build queue entry: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	if _, err := h.eventStore.Append(ctx, p.SettlementID, events.StreamProvince, "BuildComplete", map[string]any{
		"building_type": p.BuildingType,
	}, e.WorldID, &e.ID); err != nil {
		slog.Error("record BuildComplete event", "err", err)
	}

	if h.hub != nil {
		h.hub.Broadcast(e.WorldID, notify.Msg{
			Kind:    "BuildComplete",
			Payload: map[string]any{"settlement_id": p.SettlementID, "building_type": p.BuildingType},
		})
	}
	slog.Info("build complete", "settlement", p.SettlementID, "building", p.BuildingType)
	return nil
}
