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
// UnitID (C2): the units-table row being trained. Zero value = legacy (pre-C2) job.
type TrainCompletePayload struct {
	SettlementID uuid.UUID `json:"settlement_id"`
	UnitType     string    `json:"unit_type"`
	Count        int       `json:"count"`
	UnitID       uuid.UUID `json:"unit_id,omitempty"` // C2: units table row; zero = legacy
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
//
// If p.UnitID is set, this is a C2 batch (10 men). The unit's size was already
// written at recruit time; here we only need to check if the unit has reached
// 100 men and flip it to garrison status. The units table is the single source
// of truth for a settlement's army (the old settlements.* integer columns were
// retired in SB7); nothing is dual-written here.
//
// Naval (ship-build overhaul 2026-07-09): each vessel schedules exactly one
// TrainComplete (its build time, not a per-10-crew batch); this handler flips
// it forming→garrison unconditionally (no size threshold — naval size is
// always 1) and clears build_complete_at.
//
// Legacy path (p.UnitID zero): behaves as before — increments the column by Count.
//
// Idempotent: the units UPDATEs use a conditional status check so re-running
// a completed batch is a safe no-op.
func (h *TrainCompleteHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p TrainCompletePayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal train payload: %w", err)
	}

	col := unitColumn(p.UnitType)
	if col == "" {
		return fmt.Errorf("unknown unit type: %s", p.UnitType)
	}

	isNaval := col == "ship" || col == "war_galley" || col == "merchantman"

	// ── C2 units table: check forming→garrison transition ──────────────────────
	var newSize int
	unitNotZero := p.UnitID != uuid.Nil
	if unitNotZero && !isNaval {
		// For land units: flip to garrison when size reaches 100.
		// The size was already set at recruit time; just check current size.
		if scanErr := h.pool.QueryRow(ctx,
			`SELECT size FROM units WHERE id = $1`, p.UnitID,
		).Scan(&newSize); scanErr != nil {
			slog.Warn("C2 unit size check failed", "unit", p.UnitID, "err", scanErr)
		} else if newSize >= 100 {
			// Idempotent: only flip if still forming.
			if _, flipErr := h.pool.Exec(ctx,
				`UPDATE units SET status = 'garrison', updated_at = now()
				 WHERE id = $1 AND status = 'forming'`,
				p.UnitID,
			); flipErr != nil {
				slog.Warn("C2 forming→garrison flip failed", "unit", p.UnitID, "err", flipErr)
			}
		}
	}
	// Naval (ship-build overhaul 2026-07-09): a vessel is created in
	// 'forming' status with build_complete_at set to this event's due time —
	// one TrainComplete fires per vessel (build time), not per 10-crew batch
	// like land. Flip forming→garrison and clear the ETA. Idempotent: only
	// flips if still forming, same guard as the land path.
	if unitNotZero && isNaval {
		if _, flipErr := h.pool.Exec(ctx,
			`UPDATE units SET status = 'garrison', build_complete_at = NULL, updated_at = now()
			 WHERE id = $1 AND status = 'forming'`,
			p.UnitID,
		); flipErr != nil {
			slog.Warn("naval forming→garrison flip failed", "unit", p.UnitID, "err", flipErr)
		}
	}

	// Recompute production: labor rates may depend on population (already deducted at recruit).
	if err := economy.RecomputeProduction(ctx, h.pool, p.SettlementID); err != nil {
		slog.Warn("recompute production after training", "settlement", p.SettlementID, "err", err)
	}

	if _, err := h.eventStore.Append(ctx, p.SettlementID, events.StreamProvince, "TrainComplete", map[string]any{
		"unit_type":  p.UnitType,
		"count":      p.Count,
		"unit_id":    p.UnitID,
		"size_after": newSize,
	}, e.WorldID, nil); err != nil {
		slog.Error("record TrainComplete event", "err", err)
	}

	if h.hub != nil {
		var ownerID uuid.UUID
		_ = h.pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id = $1`, p.SettlementID).Scan(&ownerID)
		_ = h.hub.NotifyPlayer(ctx, e.WorldID, ownerID, "TrainComplete", 4, map[string]any{
			"settlement_id": p.SettlementID,
			"unit_type":     p.UnitType,
			"count":         p.Count,
			"unit_id":       p.UnitID,
		})
	}
	slog.Info("training complete", "settlement", p.SettlementID, "unit", p.UnitType, "count", p.Count)
	return nil
}

func unitColumn(unitType string) string {
	switch unitType {
	case "spearman":
		return "infantry" // legacy integer army column kept as `infantry`
	case "war_chariot":
		return "chariot" // legacy integer army column kept as `chariot`
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
