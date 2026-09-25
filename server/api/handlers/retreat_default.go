package handlers

// Realm-wide retreat default (War → "When to retreat"; keryx retreat-default).
// GET/PUT /worlds/{worldID}/retreat-default. The rules — three states,
// validation, seeding onto battle participants — live in
// internal/combat/retreat_default.go; this file only speaks HTTP.
//
// Applies at once (no Runner): it is a standing doctrine, not an order to a
// unit, and it only reaches units that enter a battle after the change.

import (
	"encoding/json"
	"errors"
	"net/http"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/combat"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RetreatDefaultHandler serves the realm-wide retreat default.
type RetreatDefaultHandler struct {
	pool *pgxpool.Pool
}

// NewRetreatDefaultHandler creates a RetreatDefaultHandler.
func NewRetreatDefaultHandler(pool *pgxpool.Pool) *RetreatDefaultHandler {
	return &RetreatDefaultHandler{pool: pool}
}

type retreatDefaultOut struct {
	RetreatAtLoss *float64 `json:"retreat_at_loss"`
	HoldToLastMan bool     `json:"hold_to_last_man"`
	ByLoyalty     bool     `json:"by_loyalty"`
}

func toRetreatDefaultOut(d combat.RetreatDefault) retreatDefaultOut {
	return retreatDefaultOut{RetreatAtLoss: d.RetreatAtLoss, HoldToLastMan: d.HoldToLastMan, ByLoyalty: d.ByLoyalty()}
}

func writeOrderErr(w http.ResponseWriter, err error, fallback string) {
	var rej *combat.OrderReject
	if errors.As(err, &rej) {
		writeError(w, rej.Status, rej.Reason)
		return
	}
	writeError(w, http.StatusInternalServerError, fallback)
}

// Get handles GET /worlds/{worldID}/retreat-default.
func (h *RetreatDefaultHandler) Get(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	d, err := combat.LoadRetreatDefault(r.Context(), h.pool, worldID, playerID)
	if err != nil {
		writeOrderErr(w, err, "could not load retreat default")
		return
	}
	writeJSON(w, http.StatusOK, toRetreatDefaultOut(d))
}

// Put handles PUT /worlds/{worldID}/retreat-default.
// Body: exactly one of {"retreat_at_loss": 0-1} · {"hold_to_last_man": true} · {"by_loyalty": true}.
func (h *RetreatDefaultHandler) Put(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req struct {
		RetreatAtLoss *float64 `json:"retreat_at_loss"`
		HoldToLastMan bool     `json:"hold_to_last_man"`
		ByLoyalty     bool     `json:"by_loyalty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	d, err := combat.SetRetreatDefault(r.Context(), h.pool, worldID, playerID, combat.RetreatDefaultOrder{
		RetreatAtLoss: req.RetreatAtLoss, HoldToLastMan: req.HoldToLastMan, ByLoyalty: req.ByLoyalty,
	})
	if err != nil {
		writeOrderErr(w, err, "could not save retreat default")
		return
	}
	writeJSON(w, http.StatusOK, toRetreatDefaultOut(d))
}
