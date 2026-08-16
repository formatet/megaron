package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/unit"
)

// Repair handles POST /worlds/{worldID}/units/{unitID}/repair
//
// Starts a hull repair job (megaron_plan_skeppsreparation.md Slice C) on a
// damaged naval unit (hull < combat.HullMax) standing in garrison at one of
// the caller's own settlements that has a shipyard. Follows train.go:89-100's
// naval forming→garrison timer shape: the ship flips to unit.StatusRepairing
// with build_complete_at set to the job's ETA; combat.ShipRepairCompleteHandler
// flips it back to garrison at hull=combat.HullMax when the scheduled event
// fires. Timber/cedar cost (combat.RepairCost, ∝ hull points restored, off
// the LIVING training.go build recipe) is deducted up front, same posture as
// Recruit — starting a repair without the goods in stock fails outright.
func (h *UnitHandler) Repair(w http.ResponseWriter, r *http.Request) {
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	unitID, err := uuid.Parse(chi.URLParam(r, "unitID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid unit ID")
		return
	}
	ctx := r.Context()

	// Ownership + existence collapsed into one 404 — same posture as Recall.
	u, err := h.store.Get(ctx, unitID)
	if err != nil || u.OwnerID != playerID || u.WorldID != worldID {
		writeError(w, http.StatusNotFound, "unit not found")
		return
	}
	if u.Category != unit.CategoryNaval {
		writeError(w, http.StatusUnprocessableEntity, "only naval units have a hull to repair")
		return
	}
	if u.Status != unit.StatusGarrison {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("ship must be in garrison to begin repair (status: %s)", string(u.Status)))
		return
	}
	if u.Hull >= combat.HullMax {
		writeError(w, http.StatusUnprocessableEntity, "hull is already at full strength")
		return
	}
	if u.SettlementID == nil {
		writeError(w, http.StatusUnprocessableEntity, "ship has no settlement to repair at")
		return
	}
	settlementID := *u.SettlementID

	// §Slice C point 1: "en egen kuststad med shipyard" — the settlement, not
	// just the ship, must be the caller's own.
	var settlementOwnerID uuid.UUID
	if err := h.pool.QueryRow(ctx,
		`SELECT owner_id FROM settlements WHERE id = $1 AND world_id = $2 AND state = 'active'`,
		settlementID, worldID,
	).Scan(&settlementOwnerID); err != nil || settlementOwnerID != playerID {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	var shipyardLevel int
	if err := h.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(level), 0) FROM buildings WHERE settlement_id = $1 AND building_type = 'shipyard'`,
		settlementID,
	).Scan(&shipyardLevel); err != nil || shipyardLevel < 1 {
		writeError(w, http.StatusUnprocessableEntity, "shipyard required")
		return
	}

	// Capacity gate (§Slice C point 3): a repair occupies a shipyard
	// workplace slot, economy.WorkplaceSlots("shipyard", level) = 3/6/10 by
	// level. This counts ONLY concurrent repairs at this settlement, not ship
	// BUILDS sharing the same yard — Recruit's existing flat build-queue cap
	// (province.go, 10/settlement) is untouched here. Combining the two pools
	// into one true labour-slot accounting is the "full P2 integration" the
	// plan explicitly allows deferring ("bygg kapacitetsgrinden ... men
	// namnge det") — flagged, not built, in this slice.
	slots := economy.WorkplaceSlots("shipyard", shipyardLevel)
	var activeRepairs int
	if err := h.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM units WHERE settlement_id = $1 AND status = 'repairing'`,
		settlementID,
	).Scan(&activeRepairs); err != nil {
		writeError(w, http.StatusInternalServerError, "could not check shipyard capacity")
		return
	}
	if activeRepairs >= slots {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("shipyard full: %d/%d repair berths in use", activeRepairs, slots))
		return
	}

	hullPoints := combat.HullMax - u.Hull
	good, amount, costOK := combat.RepairCost(string(u.Type), hullPoints)
	if !costOK {
		writeError(w, http.StatusUnprocessableEntity, "unknown unit type")
		return
	}

	var currentTick int
	_ = h.pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	durationTicks := combat.RepairDurationTicks(hullPoints)
	dueTick := currentTick + durationTicks
	completeAt := tick.EtaAt(h.clk, dueTick, currentTick)

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start repair")
		return
	}
	defer tx.Rollback(ctx)

	if err := deductGoods(ctx, tx, settlementID, map[string]float64{good: amount}); err != nil {
		var insErr *insufficientGoodsError
		if errors.As(err, &insErr) {
			writeGoodsError(w, insErr)
		} else {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("could not deduct resources: %v", err))
		}
		return
	}
	if _, err := tx.Exec(ctx,
		`UPDATE units SET status = 'repairing', build_complete_at = $2, updated_at = now() WHERE id = $1`,
		unitID, completeAt,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not start repair")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "could not start repair")
		return
	}

	if err := h.scheduler.EnqueueTick(ctx, worldID, events.ScheduledShipRepairComplete,
		combat.ShipRepairCompletePayload{
			UnitID: unitID, SettlementID: settlementID, WorldID: worldID, UnitType: string(u.Type),
		}, dueTick,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not schedule repair completion")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"unit_id":        unitID,
		"hull_before":    u.Hull,
		"hull_target":    combat.HullMax,
		"good":           good,
		"amount":         amount,
		"duration_ticks": durationTicks,
		"complete_at":    completeAt,
	})
}
