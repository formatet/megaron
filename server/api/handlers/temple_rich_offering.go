package handlers

import (
	"encoding/json"
	"net/http"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/kharis"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// SetRichOffering turns a temple's rich offering on (PUT {"good": ...}) or off (DELETE).
// Bonus and cost live in kharis.planCityOffers; this only stores the choice.
func (h *ProvinceHandler) SetRichOffering(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	buildingID, err := uuid.Parse(chi.URLParam(r, "buildingID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid temple ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var good *string
	if r.Method == http.MethodPut {
		var body struct {
			Good string `json:"good"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Good == "" {
			writeError(w, http.StatusBadRequest, "good is required")
			return
		}
		if !kharis.RichOfferingGood(body.Good) {
			writeError(w, http.StatusBadRequest, "the gods do not price that good")
			return
		}
		good = &body.Good
	}
	tag, err := h.pool.Exec(r.Context(), `UPDATE buildings b SET rich_offering_good=$4
	 FROM settlements s WHERE b.id=$1 AND b.building_type='temple' AND s.id=b.settlement_id
	 AND s.world_id=$2 AND s.owner_id=$3`, buildingID, worldID, playerID, good)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save the rich offering")
		return
	}
	if tag.RowsAffected() != 1 {
		writeError(w, http.StatusNotFound, "no such temple of yours")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"temple_id": buildingID, "rich_offering_good": good})
}
