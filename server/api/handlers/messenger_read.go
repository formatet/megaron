package handlers

import (
	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
)

// Completion statuses alone are insufficient: a rescued sealed runner can turn
// home without ever delivering its letter. The durable arrival outcome proves it.
const receivedLetterSQL = `(m.status='delivered' OR EXISTS(SELECT 1 FROM events e
 WHERE e.world_id=m.world_id AND e.stream_id=m.destination_id AND e.event_type='MessengerArrived'
 AND e.payload->>'messenger_id'=m.id::text))`

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
	tag, err := h.pool.Exec(r.Context(), `UPDATE messengers m SET read_at=COALESCE(m.read_at,$4) FROM settlements s WHERE m.id=$1 AND m.world_id=$2 AND m.destination_id=s.id AND s.owner_id=$3 AND NOT m.withdrawn AND `+receivedLetterSQL, id, worldID, player, h.clk.Now())
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
