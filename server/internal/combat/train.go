package combat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/economy"
	"github.com/poleia/server/internal/events"
)

// TrainCompletePayload is the scheduled event payload for finished unit training.
type TrainCompletePayload struct {
	SettlementID uuid.UUID `json:"settlement_id"`
	UnitType     string    `json:"unit_type"`
	Count        int       `json:"count"`
}

// TrainCompleteHandler resolves completed unit training.
type TrainCompleteHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
	hub        Broadcaster
}

// NewTrainCompleteHandler creates a TrainCompleteHandler.
func NewTrainCompleteHandler(pool *pgxpool.Pool, eventStore *events.Store, hub Broadcaster) *TrainCompleteHandler {
	return &TrainCompleteHandler{pool: pool, eventStore: eventStore, hub: hub}
}

// Handle processes a ScheduledTrainComplete event.
func (h *TrainCompleteHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p TrainCompletePayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal train payload: %w", err)
	}

	col := unitColumn(p.UnitType)
	if col == "" {
		return fmt.Errorf("unknown unit type: %s", p.UnitType)
	}

	_, err := h.pool.Exec(ctx,
		fmt.Sprintf(`UPDATE settlements SET %s = %s + $1 WHERE id = $2`, col, col),
		p.Count, p.SettlementID,
	)
	if err != nil {
		return fmt.Errorf("add units: %w", err)
	}

	// Recompute production: army columns changed, so labor_pool changed.
	if err := economy.RecomputeProduction(ctx, h.pool, p.SettlementID); err != nil {
		slog.Warn("recompute production after training", "settlement", p.SettlementID, "err", err)
	}

	if _, err := h.eventStore.Append(ctx, p.SettlementID, events.StreamProvince, "TrainComplete", map[string]any{
		"unit_type": p.UnitType,
		"count":     p.Count,
	}, e.WorldID, nil); err != nil {
		slog.Error("record TrainComplete event", "err", err)
	}

	if h.hub != nil {
		h.hub.BroadcastEvent(e.WorldID, "TrainComplete", map[string]any{
			"settlement_id": p.SettlementID,
			"unit_type":     p.UnitType,
			"count":         p.Count,
		})
	}
	slog.Info("training complete", "settlement", p.SettlementID, "unit", p.UnitType, "count", p.Count)
	return nil
}

func unitColumn(unitType string) string {
	switch unitType {
	case "infantry":
		return "infantry"
	case "cavalry":
		return "cavalry"
	case "catapult":
		return "catapult"
	case "priest":
		return "priest"
	case "ship":
		return "ship" // galley — DB-kolumn heter ship
	case "elite_infantry":
		return "elite_infantry"
	case "war_galley":
		return "war_galley"
	case "merchantman":
		return "merchantman"
	}
	return ""
}
