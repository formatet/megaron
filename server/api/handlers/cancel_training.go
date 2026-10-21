package handlers

import (
	"context"
	"errors"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/province"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
)

func (h *UnitHandler) CancelTraining(w http.ResponseWriter, r *http.Request) {
	wid, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "unitID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid unit ID")
		return
	}
	owner, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	result, err := combat.CancelTraining(r.Context(), h.pool, h.eventStore, wid, owner, id, h.clk.Now())
	if err != nil {
		var rej *combat.OrderReject
		if errors.As(err, &rej) {
			writeError(w, rej.Status, rej.Reason)
		} else {
			writeError(w, http.StatusInternalServerError, "could not cancel training")
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func attachRecruitCancelNotes(ctx context.Context, db province.Queryer, wid, owner uuid.UUID, out []unitSummary) {
	rows, err := db.Query(ctx, `SELECT id FROM units WHERE world_id=$1 AND owner_id=$2 AND status IN ('forming','training') AND recruitment_costs IS NOT NULL`, wid, owner)
	if err != nil {
		return
	}
	defer rows.Close()
	eligible := map[uuid.UUID]bool{}
	for rows.Next() {
		var id uuid.UUID
		if rows.Scan(&id) == nil {
			eligible[id] = true
		}
	}
	for i := range out {
		out[i].CanCancelTraining = eligible[out[i].ID]
	}
}
