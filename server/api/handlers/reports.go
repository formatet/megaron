package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReportsHandler serves the player_reports feedback channel (B1,
// megaron_mvp_mandag.md §B1) — deliberately primitive: a testing Wanax can
// say "this is broken" or "this feels wrong" and have it land somewhere,
// with the context they'd otherwise forget to write (who, when, where)
// stamped by the server instead of typed by hand.
type ReportsHandler struct {
	pool *pgxpool.Pool

	// logDir is where every report is also appended as one JSON line
	// (reports.jsonl), outside the database — the copy that survives a
	// reseed (TRUNCATE worlds CASCADE) and a DB reset. tools/reports_to_vault.py
	// renders it into the vault once a day. Empty = disabled (tests).
	logDir string
	logMu  sync.Mutex
}

// NewReportsHandler creates a ReportsHandler.
func NewReportsHandler(pool *pgxpool.Pool) *ReportsHandler {
	return &ReportsHandler{pool: pool}
}

// SetLogDir enables the append-only report log in dir (REPORTS_DIR).
func (h *ReportsHandler) SetLogDir(dir string) { h.logDir = dir }

// reportLogLine is one line of reports.jsonl. Names are resolved at write
// time because the world row is gone after the next reseed.
type reportLogLine struct {
	ID        uuid.UUID       `json:"id"`
	CreatedAt time.Time       `json:"created_at"`
	WorldID   uuid.UUID       `json:"world_id"`
	WorldName string          `json:"world_name"`
	PlayerID  uuid.UUID       `json:"player_id"`
	Wanax     string          `json:"wanax"`
	Username  string          `json:"username"`
	Kind      string          `json:"kind"`
	Body      string          `json:"body"`
	Q         *int            `json:"q,omitempty"`
	R         *int            `json:"r,omitempty"`
	View      string          `json:"view,omitempty"`
	Tick      int             `json:"tick"`
	Context   json.RawMessage `json:"context,omitempty"`
}

// appendLog writes one report to reports.jsonl. A failure is logged, never
// returned: the report is already saved in the database, and the player
// must not see an error for a disk problem on our side.
func (h *ReportsHandler) appendLog(line reportLogLine) {
	if h.logDir == "" {
		return
	}
	raw, err := json.Marshal(line)
	if err != nil {
		slog.Error("report log: marshal", "id", line.ID, "err", err)
		return
	}
	h.logMu.Lock()
	defer h.logMu.Unlock()
	if err := os.MkdirAll(h.logDir, 0o755); err != nil {
		slog.Error("report log: mkdir", "dir", h.logDir, "err", err)
		return
	}
	f, err := os.OpenFile(filepath.Join(h.logDir, "reports.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		slog.Error("report log: open", "dir", h.logDir, "err", err)
		return
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		slog.Error("report log: write", "id", line.ID, "err", err)
	}
}

var validReportKinds = map[string]bool{"bug": true, "design": true, "confused": true}

// Create handles POST /worlds/{worldID}/reports. kind must be one of
// bug/design/confused; body is the free text. q/r/view are optional context
// the caller supplies (current hex, current drawer) — tick is never taken
// from the request, it is read server-side so it can't be stale or spoofed.
func (h *ReportsHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		Kind    string          `json:"kind"`
		Body    string          `json:"body"`
		Q       *int            `json:"q"`
		R       *int            `json:"r"`
		View    string          `json:"view"`
		Context json.RawMessage `json:"context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Body = strings.TrimSpace(req.Body)
	if req.Body == "" {
		writeError(w, http.StatusBadRequest, "body must not be empty")
		return
	}
	if !validReportKinds[req.Kind] {
		writeError(w, http.StatusBadRequest, "kind must be bug, design or confused")
		return
	}

	var reportContext []byte
	if len(req.Context) > 0 && string(req.Context) != "null" {
		reportContext = req.Context
	}

	var id uuid.UUID
	var tick int
	var createdAt time.Time
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO player_reports (world_id, player_id, kind, body, q, r, view, context, tick)
		 VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, current_world_tick())
		 RETURNING id, tick, created_at`,
		worldID, playerID, req.Kind, req.Body, req.Q, req.R, req.View, reportContext,
	).Scan(&id, &tick, &createdAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "insert failed")
		return
	}

	if h.logDir != "" {
		line := reportLogLine{
			ID: id, CreatedAt: createdAt.UTC(), WorldID: worldID, PlayerID: playerID,
			Kind: req.Kind, Body: req.Body, Q: req.Q, R: req.R, View: req.View,
			Tick: tick, Context: reportContext,
		}
		// Names are best effort: a missing row leaves the field empty, the ids stay.
		_ = h.pool.QueryRow(r.Context(),
			`SELECT COALESCE((SELECT name FROM worlds WHERE id = $1), ''),
			        COALESCE((SELECT COALESCE(wanax_name, '') FROM players WHERE id = $2), ''),
			        COALESCE((SELECT username FROM players WHERE id = $2), '')`,
			worldID, playerID,
		).Scan(&line.WorldName, &line.Wanax, &line.Username)
		h.appendLog(line)
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "tick": tick})
}

// List handles GET /admin/worlds/{worldID}/reports — Timothy's own read path,
// gated by X-Admin-Key like god.go's god-view. No admin UI; keryx renders
// this (see cmd/keryx/cmd_reports.go).
func (h *ReportsHandler) List(w http.ResponseWriter, r *http.Request) {
	if !requireAdminKey(w, r) {
		return
	}
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT pr.id, COALESCE(pl.wanax_name, pl.username, ''), pr.kind, pr.body,
		        pr.q, pr.r, pr.view, pr.context, pr.tick, pr.created_at
		 FROM player_reports pr
		 JOIN players pl ON pl.id = pr.player_id
		 WHERE pr.world_id = $1
		 ORDER BY pr.created_at DESC`,
		worldID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type reportRow struct {
		ID        uuid.UUID       `json:"id"`
		Player    string          `json:"player"`
		Kind      string          `json:"kind"`
		Body      string          `json:"body"`
		Q         *int            `json:"q"`
		R         *int            `json:"r"`
		View      *string         `json:"view"`
		Context   json.RawMessage `json:"context,omitempty"`
		Tick      int             `json:"tick"`
		CreatedAt time.Time       `json:"created_at"`
	}
	var items []reportRow
	for rows.Next() {
		var rr reportRow
		if err := rows.Scan(&rr.ID, &rr.Player, &rr.Kind, &rr.Body, &rr.Q, &rr.R, &rr.View, &rr.Context, &rr.Tick, &rr.CreatedAt); err != nil {
			continue
		}
		items = append(items, rr)
	}
	if items == nil {
		items = []reportRow{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"reports": items})
}
