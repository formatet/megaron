package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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
	ID        uuid.UUID       `json:"id"`
	Kind      string          `json:"kind"`
	Level     int             `json:"level"`
	BodyJSON  json.RawMessage `json:"body"`
	CreatedAt time.Time       `json:"created_at"`
	ReadAt    *time.Time      `json:"read_at"`
}

// List returns recent notifications for the authenticated player in this world.
// Query param ?unread=true limits to unread only. ?kind=<k1,k2> restricts to
// those kinds; ?exclude=<k1,k2> drops them (both comma-separated, mutually
// useful e.g. to keep high-frequency kinds like SitosIntervention from
// burying the LIMIT 100 window). Omitting both is unchanged from before.
//
// ?dispatchable=true additionally drops kinds this player has muted as
// dispatches. It exists for the one caller that rebuilds the Dispatches strip
// on load: the strip must show what a live push WOULD have shown, and
// notify.Hub.NotifyPlayer suppresses muted kinds there. Filtering client-side
// instead would need a second round trip and would drift the moment a third
// client (keryx, Lawagetas) grew its own strip — the mute is a server truth,
// so it is applied where the server answers.
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
	dispatchableOnly := r.URL.Query().Get("dispatchable") == "true"
	kinds := splitKinds(r.URL.Query().Get("kind"))
	excludeKinds := splitKinds(r.URL.Query().Get("exclude"))

	query := `
		SELECT id, kind, level, body_json, created_at, read_at
		FROM notifications
		WHERE world_id = $1 AND player_id = $2`
	args := []any{worldID, playerID}
	if onlyUnread {
		query += ` AND read_at IS NULL`
	}
	if len(kinds) > 0 {
		args = append(args, kinds)
		query += fmt.Sprintf(` AND kind = ANY($%d)`, len(args))
	}
	if len(excludeKinds) > 0 {
		args = append(args, excludeKinds)
		query += fmt.Sprintf(` AND kind <> ALL($%d)`, len(args))
	}
	if dispatchableOnly {
		// $2 is playerID, bound above.
		query += ` AND kind NOT IN (SELECT kind FROM dispatch_mutes WHERE player_id = $2)`
	}
	query += ` ORDER BY created_at DESC LIMIT 100`

	rows, err := h.pool.Query(r.Context(), query, args...)
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
		if len(bodyRaw) > 0 {
			n.BodyJSON = json.RawMessage(bodyRaw)
		} else {
			n.BodyJSON = json.RawMessage("{}")
		}
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

// splitKinds parses a comma-separated ?kind=/?exclude= query value into a
// trimmed, non-empty slice. Returns nil for an empty input.
func splitKinds(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	kinds := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			kinds = append(kinds, p)
		}
	}
	return kinds
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

// MarkRead marks ONE notification read — what dismissing a dispatch chip
// means. ReadAll (opening the archive) says "I have read the feed"; this says
// "I have dealt with this one", which is what keeps a dismissed chip from
// coming back on the next page load. Scoped to the caller's own rows, so a
// player can never mark another Wanax's notification read. Idempotent: a row
// already read, or an id that is not theirs, is 204 either way — a dismissal
// must never fail loudly in the UI over bookkeeping.
func (h *NotificationsHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	notifID, err := uuid.Parse(chi.URLParam(r, "notifID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid notification ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if _, err := h.pool.Exec(r.Context(),
		`UPDATE notifications SET read_at = now()
		 WHERE id = $1 AND world_id = $2 AND player_id = $3 AND read_at IS NULL`,
		notifID, worldID, playerID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteAll removes all of the player's notifications in this world — the
// "clear all" button (Megaron). ReadAll only marks read (they linger in the
// feed styled as read); this empties the feed for good. Scoped to the caller's
// own rows, so it can never touch another Wanax's notifications.
func (h *NotificationsHandler) DeleteAll(w http.ResponseWriter, r *http.Request) {
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
		`DELETE FROM notifications WHERE world_id = $1 AND player_id = $2`,
		worldID, playerID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
