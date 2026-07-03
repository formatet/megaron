package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/capabilities"
)

// Actions handles GET /worlds/:worldID/provinces/:provinceID/actions.
// Returns the capabilities surface (temenos_capabilities.md): every mutating
// verb, whether it is available right now, and for locked verbs exactly what
// live gap blocks it (detail) and how to close it (hint). Server-authoritative
// — the same shape keryx's `poleia actions` and (later, Fas 4) agent.py consume,
// so there is one perceptible truth about what a Wanax can do next.
func (h *ProvinceHandler) Actions(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Verify ownership via settlement (mirrors Build/Recruit's ownership gate).
	var settlementID uuid.UUID
	var ownerID *uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT id, owner_id FROM settlements WHERE province_id = $1 AND world_id = $2`,
		provinceID, worldID,
	).Scan(&settlementID, &ownerID)
	if err != nil || ownerID == nil || *ownerID != playerID {
		writeError(w, http.StatusForbidden, "not your province")
		return
	}

	verbs := capabilities.List(r.Context(), h.pool, h.clk, worldID, provinceID, playerID, settlementID)
	writeJSON(w, http.StatusOK, verbs)
}
