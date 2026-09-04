package combat

// megaron_plan_skeppsreparation.md Slice C — a damaged ship (hull < HullMax,
// Slice B) docked in garrison at an own coastal settlement with a shipyard
// can start a repair job that restores it to HullMax over time, consuming
// timber/cedar up front and occupying a shipyard workplace slot for the
// duration. api/handlers.UnitHandler.Repair starts the job (validates,
// deducts goods, flips the unit to StatusRepairing, schedules the completion
// event); ShipRepairCompleteHandler below finishes it — the same
// forming→garrison timer shape train.go:89-100 uses for a freshly built
// vessel, just flipping 'repairing'→'garrison' instead of 'forming'→
// 'garrison', and setting hull=HullMax instead of leaving it untouched.
//
// Calibration (delegated to the implementer by the plan, strawman not canon —
// pinned in ship_repair_test.go):
//   - cost: repairCostFractionPerHullPoint, of the LIVING build cost in
//     internal/province/training.go (never the taxonomy's §11.1 table — see
//     that plan's "Kodmätt nuläge" cost-divergence note. KOPPLAT TAL: if a
//     ship's training.go cost ever changes, this must be re-derived from it,
//     not left stale).
//   - duration: repairTicksPerHullPoint, deliberately NOT derived from the
//     ship's build DurationTicks — the plan only pins repair COST to the
//     living recipe, duration is a free strawman.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// repairCostFractionPerHullPoint is the plan's own recommendation
	// (§Slice C point 2, "~8% av byggkostnaden per hull-poäng") — a full
	// HullMax→0 repair costs 5×8% = 40% of a fresh build in the same material.
	repairCostFractionPerHullPoint = 0.08
	// repairTicksPerHullPoint: one world tick per hull point restored. A ship
	// limping home at hull 2 takes 3 ticks (3 game days) to reach hull 5.
	repairTicksPerHullPoint = 1
)

// repairMaterial returns the hull-repair good and the vessel's full per-ship
// build cost in that good. province.UnitSpecs[unitType].Costs is PER-MAN
// (per crew member) — see training.go's own doc comment — so the whole-ship
// figure is Costs[good] × unit.CrewFor(type), the same multiplication
// api/handlers/province.go's Recruit performs for its totalCosts. War
// galleys are cedar-hulled (their recipe carries no timber at all, see
// training.go); every other naval type repairs with timber. ok is false for
// anything this system does not know how to cost (i.e. not a known naval
// type — land units never reach this).
func repairMaterial(unitType string) (good string, shipCost float64, ok bool) {
	spec, specOK := province.UnitSpecs[unitType]
	if !specOK {
		return "", 0, false
	}
	crew := unit.CrewFor(unit.Type(unitType))
	if cost, has := spec.Costs["cedar"]; has {
		return "cedar", cost * float64(crew), true
	}
	if cost, has := spec.Costs["timber"]; has {
		return "timber", cost * float64(crew), true
	}
	return "", 0, false
}

// RepairCost returns the good and amount required to restore hullPoints of a
// unitType vessel's hull (Slice C point 2). ok is false when unitType has no
// known repair material (repairMaterial's doc comment).
func RepairCost(unitType string, hullPoints int) (good string, amount float64, ok bool) {
	good, shipCost, ok := repairMaterial(unitType)
	if !ok {
		return "", 0, false
	}
	if hullPoints <= 0 {
		return good, 0, true
	}
	return good, shipCost * repairCostFractionPerHullPoint * float64(hullPoints), true
}

// RepairDurationTicks is Slice C's repair timer (§Röd-före C: "över N ticks").
func RepairDurationTicks(hullPoints int) int {
	if hullPoints < 1 {
		return 1
	}
	return hullPoints * repairTicksPerHullPoint
}

// EventShipRepaired (Slice C point 4) is appended to the ship's own
// unit.StreamUnit stream when a repair job completes — same stream choice as
// EventShipDamaged (battle_events.go), a ship's own history.
const EventShipRepaired = "ShipRepaired"

// ShipRepairedPayload is the OUTCOME recorded when a repair completes: the
// ship is back at HullMax. No "before" hull is carried — a Wanax reads that
// off the preceding ShipDamaged notification if they want it; this event
// only needs to say the repair finished.
type ShipRepairedPayload struct {
	UnitID       uuid.UUID `json:"unit_id"`
	WorldID      uuid.UUID `json:"world_id"`
	SettlementID uuid.UUID `json:"settlement_id"`
	UnitType     string    `json:"unit_type"`
	Hull         int       `json:"hull"`
}

// ShipRepairCompletePayload is the scheduled event payload for a finished
// hull repair job (api/handlers.UnitHandler.Repair enqueues one per started
// job, keyed by UnitID — mirrors TrainCompletePayload's shape).
type ShipRepairCompletePayload struct {
	UnitID       uuid.UUID `json:"unit_id"`
	SettlementID uuid.UUID `json:"settlement_id"`
	WorldID      uuid.UUID `json:"world_id"`
	UnitType     string    `json:"unit_type"`
}

// ShipRepairCompleteHandler resolves a finished hull repair job.
type ShipRepairCompleteHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
	hub        Broadcaster
}

// NewShipRepairCompleteHandler creates a ShipRepairCompleteHandler.
func NewShipRepairCompleteHandler(pool *pgxpool.Pool, eventStore *events.Store, hub Broadcaster) *ShipRepairCompleteHandler {
	return &ShipRepairCompleteHandler{pool: pool, eventStore: eventStore, hub: hub}
}

// Handle processes a ScheduledShipRepairComplete event: flips the ship
// 'repairing'→'garrison' and its hull to HullMax.
//
// Idempotent (G2): the UPDATE's WHERE status = 'repairing' guard makes a
// re-run a safe no-op — and, unlike train.go's TrainCompleteHandler (which
// notifies unconditionally after its guarded flip), this handler only
// appends the event / notifies when the flip actually happened this call
// (RowsAffected() == 1), so a retry after the first run already completed
// the job cannot re-fire ShipRepaired a second time.
func (h *ShipRepairCompleteHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p ShipRepairCompletePayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal ship repair payload: %w", err)
	}

	tag, err := h.pool.Exec(ctx,
		`UPDATE units SET status = 'garrison', hull = $2, build_complete_at = NULL, updated_at = now()
		 WHERE id = $1 AND status = 'repairing'`,
		p.UnitID, HullMax,
	)
	if err != nil {
		return fmt.Errorf("ship repair complete: flip unit: %w", err)
	}
	if tag.RowsAffected() == 0 {
		slog.Info("ship repair complete: unit not in repairing status, skipping (already completed?)", "unit", p.UnitID)
		return nil
	}

	if err := economy.RecomputeProduction(ctx, h.pool, p.SettlementID); err != nil {
		slog.Warn("recompute production after ship repair", "settlement", p.SettlementID, "err", err)
	}

	var ownerID uuid.UUID
	_ = h.pool.QueryRow(ctx, `SELECT owner_id FROM units WHERE id = $1`, p.UnitID).Scan(&ownerID)

	if h.eventStore != nil {
		_, _ = h.eventStore.Append(ctx, p.UnitID, events.StreamType(unit.StreamUnit), EventShipRepaired,
			ShipRepairedPayload{
				UnitID: p.UnitID, WorldID: p.WorldID, SettlementID: p.SettlementID,
				UnitType: p.UnitType, Hull: HullMax,
			}, p.WorldID, nil,
		)
	}
	if h.hub != nil {
		if err := h.hub.NotifyPlayer(ctx, p.WorldID, ownerID, EventShipRepaired, 3, map[string]any{
			"unit_id":       p.UnitID,
			"unit_type":     p.UnitType,
			"name":          unit.LoadDisplayName(ctx, h.pool, p.UnitID),
			"settlement_id": p.SettlementID,
			"hull":          HullMax,
		}); err != nil {
			slog.Warn("ship repair complete: notify failed", "unit", p.UnitID, "err", err)
		}
	}
	slog.Info("ship repair complete", "unit", p.UnitID, "type", p.UnitType, "settlement", p.SettlementID)
	return nil
}
