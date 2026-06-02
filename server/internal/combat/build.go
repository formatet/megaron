package combat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/events"
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
	hub        Broadcaster
}

// NewBuildCompleteHandler creates a BuildCompleteHandler.
func NewBuildCompleteHandler(pool *pgxpool.Pool, eventStore *events.Store, hub Broadcaster) *BuildCompleteHandler {
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

	// Update settlement_goods production rates from production_rules for this building.
	if _, err = tx.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
		 SELECT $1, pr.good_key, 0, pr.rate_per_min, 100, now()
		 FROM production_rules pr
		 JOIN settlements s ON s.id = $1
		 JOIN provinces prov ON prov.id = s.province_id
		 WHERE pr.building_type = $2
		   AND (pr.terrain_type IS NULL OR pr.terrain_type = prov.terrain_type)
		   AND (pr.requires_deposit IS NULL
		        OR (pr.requires_deposit = 'copper' AND prov.copper_deposit)
		        OR (pr.requires_deposit = 'tin'    AND prov.tin_deposit)
		        OR (pr.requires_deposit = 'silver' AND prov.silver_deposit)
		        OR (pr.requires_deposit = 'cedar'  AND prov.cedar_deposit))
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
		     amount = settlement_goods.amount
		         + EXTRACT(EPOCH FROM (now() - settlement_goods.calc_at))/60 * settlement_goods.rate,
		     rate   = settlement_goods.rate + EXCLUDED.rate,
		     calc_at = now()`,
		p.SettlementID, p.BuildingType,
	); err != nil {
		return fmt.Errorf("update goods rates: %w", err)
	}

	// Apply gold and kharis rate bonuses to settlement columns.
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
			`UPDATE player_world_records SET
			   kharis_amount = kharis_amount + (EXTRACT(EPOCH FROM (now() - kharis_calc_at))/60 * kharis_rate),
			   kharis_rate = kharis_rate + $1,
			   kharis_calc_at = now()
			 WHERE player_id = (SELECT owner_id FROM settlements WHERE id = $2)
			   AND world_id = (SELECT world_id FROM settlements WHERE id = $2)`,
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
	}, e.WorldID, nil); err != nil {
		slog.Error("record BuildComplete event", "err", err)
	}

	if h.hub != nil {
		h.hub.BroadcastEvent(e.WorldID, "BuildComplete", map[string]any{
			"settlement_id": p.SettlementID,
			"building_type": p.BuildingType,
		})
	}
	slog.Info("build complete", "settlement", p.SettlementID, "building", p.BuildingType)
	return nil
}
