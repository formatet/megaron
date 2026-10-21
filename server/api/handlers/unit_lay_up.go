package handlers

import (
	"fmt"
	"net/http"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/tick"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// shipPortAction keeps ownership, hull and population under the same row locks.
// Both verbs require an active, owned port; no concurrent request can double
// return or double draft a crew.
func (h *UnitHandler) shipPortAction(w http.ResponseWriter, r *http.Request, fitOut bool) {
	owner, ok := auth.PlayerIDFromContext(r.Context())
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
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start ship action")
		return
	}
	defer tx.Rollback(ctx)

	// Settlement ownership changes (annex) lock the city before its ships.
	// Use the same order here, then re-read the unit under lock.
	var sid *uuid.UUID
	err = tx.QueryRow(ctx, `SELECT settlement_id FROM units WHERE id=$1 AND world_id=$2 AND owner_id=$3`, unitID, worldID, owner).Scan(&sid)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "unit not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load ship")
		return
	}
	if sid == nil {
		writeError(w, http.StatusUnprocessableEntity, "ship is not in a port")
		return
	}
	var portOwner uuid.UUID
	var population int
	var active, harbour bool
	err = tx.QueryRow(ctx, `SELECT s.owner_id,s.population,s.state='active',
		EXISTS(SELECT 1 FROM buildings b WHERE b.settlement_id=s.id AND b.building_type='harbour')
		FROM settlements s WHERE s.id=$1 AND s.world_id=$2 FOR UPDATE`, *sid, worldID).
		Scan(&portOwner, &population, &active, &harbour)
	if err != nil || portOwner != owner || !active || !harbour {
		writeError(w, http.StatusUnprocessableEntity, "ship must be in your active harbour")
		return
	}
	var shipOwner uuid.UUID
	var lockedSID *uuid.UUID
	var category, status string
	var crew int
	var cargo *uuid.UUID
	err = tx.QueryRow(ctx, `SELECT owner_id,settlement_id,category,status,crew,cargo_unit_id FROM units
		WHERE id=$1 AND world_id=$2 FOR UPDATE`, unitID, worldID).Scan(&shipOwner, &lockedSID, &category, &status, &crew, &cargo)
	if err == pgx.ErrNoRows || shipOwner != owner {
		writeError(w, http.StatusNotFound, "unit not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load ship")
		return
	}
	if lockedSID == nil || *lockedSID != *sid {
		writeError(w, http.StatusUnprocessableEntity, "ship changed port")
		return
	}
	if category != "naval" {
		writeError(w, http.StatusUnprocessableEntity, "only ships can be laid up")
		return
	}
	want := "garrison"
	if fitOut {
		want = "laid_up"
	}
	if status != want {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("ship must be %s", want))
		return
	}
	if crew <= 0 {
		writeError(w, http.StatusUnprocessableEntity, "ship has no crew requirement")
		return
	}
	if !fitOut && cargo != nil {
		writeError(w, http.StatusUnprocessableEntity, "unload the ship before laying it up")
		return
	}
	if !fitOut {
		var passengers bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM messengers WHERE carrier_unit_id=$1 AND passage_status='aboard')`, unitID).Scan(&passengers)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not inspect passengers")
			return
		}
		if passengers {
			writeError(w, http.StatusUnprocessableEntity, "passengers must disembark before lay up")
			return
		}
	}
	if fitOut && population < crew {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("need %d people to fit out ship (have %d)", crew, population))
		return
	}
	newStatus, delta := "laid_up", crew
	if fitOut {
		newStatus, delta = "fitting_out", -crew
	}
	var dueTick int
	var completeAt any
	if fitOut {
		var current int
		if err = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&current); err != nil {
			writeError(w, http.StatusInternalServerError, "could not read world time")
			return
		}
		dueTick = current + combat.FitOutTicks
		completeAt = tick.EtaAt(h.clk, dueTick, current)
	}
	if _, err = tx.Exec(ctx, `UPDATE units SET status=$2,build_complete_at=$3,updated_at=now() WHERE id=$1`, unitID, newStatus, completeAt); err != nil {
		writeError(w, http.StatusInternalServerError, "could not change ship status")
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE settlements SET population=population+$2 WHERE id=$1`, *sid, delta); err != nil {
		writeError(w, http.StatusInternalServerError, "could not move crew")
		return
	}
	if fitOut {
		if err = h.scheduler.EnqueueTickTx(ctx, tx, worldID, events.ScheduledShipFitOutComplete,
			combat.ShipFitOutCompletePayload{UnitID: unitID, WorldID: worldID}, dueTick); err != nil {
			writeError(w, http.StatusInternalServerError, "could not schedule fit out")
			return
		}
	}
	if err = economy.RecomputeProduction(ctx, tx, *sid); err != nil {
		writeError(w, http.StatusInternalServerError, "could not recompute city production")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "could not commit ship action")
		return
	}
	code := http.StatusOK
	if fitOut {
		code = http.StatusAccepted
	}
	writeJSON(w, code, map[string]any{"unit_id": unitID, "status": newStatus, "population": population + delta,
		"crew": crew, "duration_ticks": func() int {
			if fitOut {
				return combat.FitOutTicks
			}
			return 0
		}(),
		"ready_tick": dueTick, "complete_at": completeAt})
}

func (h *UnitHandler) LayUp(w http.ResponseWriter, r *http.Request)  { h.shipPortAction(w, r, false) }
func (h *UnitHandler) FitOut(w http.ResponseWriter, r *http.Request) { h.shipPortAction(w, r, true) }
