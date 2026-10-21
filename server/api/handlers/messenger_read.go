package handlers

import (
	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
)

// Read acknowledges only the exact letter opened by its recipient. It never
// replies, accepts an offer or changes the physical messenger's journey.
func (h *MessengerHandler) Read(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "messengerID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid messenger ID")
		return
	}
	player, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	tag, err := h.pool.Exec(r.Context(), `UPDATE messengers m SET read_at=COALESCE(m.read_at,$4) FROM settlements s WHERE m.id=$1 AND m.world_id=$2 AND m.destination_id=s.id AND s.owner_id=$3 AND NOT m.withdrawn AND m.status IN ('delivered','returning','arrived')`, id, worldID, player, h.clk.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not mark letter read")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "letter not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"read": true})
}
