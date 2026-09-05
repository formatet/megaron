package handlers

import (
	"net/http"

	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DispatchPreferencesHandler serves the per-player "which notification kinds
// become a dispatch" preference (megaron_plan_dispatches.md §2/§6:4).
// Server-side, per player, granularity = exact kind (Timothy: "inga
// cookies!" — keryx and Lawagetas read the same preference this way).
// Absence of a row means enabled, so a brand new notification kind is a
// dispatch from the day it ships — no migration, no seeding.
//
// This never touches the notifications archive: muting only stops the
// transient dispatch chip. "Allt hamnar alltid" in Notifications regardless
// (megaron_plan_dispatches.md §1) — see notify.Hub.NotifyPlayer, which always
// inserts the row and only conditionally pushes the WS chip.
type DispatchPreferencesHandler struct {
	pool *pgxpool.Pool
}

// NewDispatchPreferencesHandler creates a DispatchPreferencesHandler.
func NewDispatchPreferencesHandler(pool *pgxpool.Pool) *DispatchPreferencesHandler {
	return &DispatchPreferencesHandler{pool: pool}
}

// List returns the notification kinds this player has muted as dispatches.
func (h *DispatchPreferencesHandler) List(w http.ResponseWriter, r *http.Request) {
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	rows, err := h.pool.Query(r.Context(),
		`SELECT kind FROM dispatch_mutes WHERE player_id = $1`, playerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	kinds := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			continue
		}
		kinds = append(kinds, k)
	}
	writeJSON(w, http.StatusOK, map[string]any{"muted_kinds": kinds})
}

// Mute stops a notification kind from becoming a dispatch chip. The event
// still lands in the Notifications archive — see notify.Hub.NotifyPlayer.
func (h *DispatchPreferencesHandler) Mute(w http.ResponseWriter, r *http.Request) {
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	kind := chi.URLParam(r, "kind")
	if kind == "" {
		writeError(w, http.StatusBadRequest, "missing kind")
		return
	}
	if _, err := h.pool.Exec(r.Context(),
		`INSERT INTO dispatch_mutes (player_id, kind) VALUES ($1, $2)
		 ON CONFLICT (player_id, kind) DO NOTHING`,
		playerID, kind,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Unmute lets a notification kind become a dispatch chip again — "kryssa i
// den nerifrån arkivet" (megaron_plan_dispatches.md §1).
func (h *DispatchPreferencesHandler) Unmute(w http.ResponseWriter, r *http.Request) {
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	kind := chi.URLParam(r, "kind")
	if kind == "" {
		writeError(w, http.StatusBadRequest, "missing kind")
		return
	}
	if _, err := h.pool.Exec(r.Context(),
		`DELETE FROM dispatch_mutes WHERE player_id = $1 AND kind = $2`,
		playerID, kind,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
