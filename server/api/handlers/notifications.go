package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/auth"
)

// NotificationsHandler serves persistent notification records.
type NotificationsHandler struct {
	pool *pgxpool.Pool
}

// NewNotificationsHandler creates a NotificationsHandler.
func NewNotificationsHandler(pool *pgxpool.Pool) *NotificationsHandler {
	return &NotificationsHandler{pool: pool}
}

type notificationRow struct {
	ID        uuid.UUID  `json:"id"`
	Kind      string     `json:"kind"`
	Level     int        `json:"level"`
	BodyJSON  any        `json:"body"`
	CreatedAt time.Time  `json:"created_at"`
	ReadAt    *time.Time `json:"read_at"`
}

// List returns recent notifications for the authenticated player in this world.
// Query param ?unread=true limits to unread only.
func (h *NotificationsHandler) List(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	onlyUnread := r.URL.Query().Get("unread") == "true"

	query := `
		SELECT id, kind, level, body_json, created_at, read_at
		FROM notifications
		WHERE world_id = $1 AND player_id = $2`
	if onlyUnread {
		query += ` AND read_at IS NULL`
	}
	query += ` ORDER BY created_at DESC LIMIT 100`

	rows, err := h.pool.Query(r.Context(), query, worldID, playerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	var items []notificationRow
	var unread int
	for rows.Next() {
		var n notificationRow
		var bodyRaw []byte
		if err := rows.Scan(&n.ID, &n.Kind, &n.Level, &bodyRaw, &n.CreatedAt, &n.ReadAt); err != nil {
			continue
		}
		n.BodyJSON = bodyRaw
		if n.ReadAt == nil {
			unread++
		}
		items = append(items, n)
	}
	if items == nil {
		items = []notificationRow{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"notifications": items,
		"unread":        unread,
	})
}

// ReadAll marks all of the player's notifications in this world as read.
func (h *NotificationsHandler) ReadAll(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if _, err := h.pool.Exec(r.Context(),
		`UPDATE notifications SET read_at = now()
		 WHERE world_id = $1 AND player_id = $2 AND read_at IS NULL`,
		worldID, playerID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
