package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"

	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type casePrivate struct {
	Draft   string `json:"draft"`
	Note    string `json:"note"`
	Version int64  `json:"version"`
}

// The historical people own private material, not the current owner of either
// city. A guessed case UUID cannot expose an as-yet undelivered first letter.
const privateCaseAccessSQL = `c.id=$1 AND c.world_id=$2
 AND ($3=c.first_player_id OR $3=c.second_player_id)
 AND EXISTS(SELECT 1 FROM correspondence_letters l WHERE l.case_id=c.id AND (l.sender_id=$3 OR l.delivered))`

func privateCaseIdentity(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	world, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, 400, "invalid world ID")
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "caseID"))
	if err != nil {
		writeError(w, 400, "invalid case ID")
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	player, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, 401, "not authenticated")
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	return world, id, player, true
}

func (h *MessengerHandler) CasePrivate(w http.ResponseWriter, r *http.Request) {
	world, id, player, ok := privateCaseIdentity(w, r)
	if !ok {
		return
	}
	var result casePrivate
	err := h.pool.QueryRow(r.Context(), `SELECT COALESCE(p.draft,''),COALESCE(p.note,''),COALESCE(p.version,0)
 FROM correspondence_cases c LEFT JOIN correspondence_private p ON p.case_id=c.id AND p.player_id=$3
 WHERE `+privateCaseAccessSQL, id, world, player).Scan(&result.Draft, &result.Note, &result.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "case not found")
		return
	}
	if err != nil {
		writeError(w, 500, "could not load private draft and note")
		return
	}
	writeJSON(w, 200, result)
}

func (h *MessengerHandler) SaveCasePrivate(w http.ResponseWriter, r *http.Request) {
	world, id, player, ok := privateCaseIdentity(w, r)
	if !ok {
		return
	}
	var req struct {
		Draft           *string `json:"draft"`
		Note            *string `json:"note"`
		ExpectedVersion *int64  `json:"expected_version"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil {
		writeError(w, 400, "invalid private draft and note JSON")
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		writeError(w, 400, "one JSON object required")
		return
	}
	if req.Draft == nil || req.Note == nil || req.ExpectedVersion == nil || *req.ExpectedVersion < 0 || *req.ExpectedVersion == math.MaxInt64 {
		writeError(w, 400, "draft, note and nonnegative expected_version are required")
		return
	}
	if len(*req.Draft) > 1000 || len(*req.Note) > 10000 {
		writeError(w, 400, "draft must be at most 1000 bytes and private note at most 10000 bytes")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "could not save private draft and note")
		return
	}
	defer tx.Rollback(r.Context())
	var found uuid.UUID
	err = tx.QueryRow(r.Context(), `SELECT c.id FROM correspondence_cases c WHERE `+privateCaseAccessSQL+` FOR SHARE OF c`, id, world, player).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "case not found")
		return
	}
	if err != nil {
		writeError(w, 500, "could not authorize private save")
		return
	}
	var result casePrivate
	// INSERT and UPDATE share one atomic compare-and-swap. Concurrent first saves
	// cannot both create version1, and stale tabs never overwrite a newer value.
	err = tx.QueryRow(r.Context(), `INSERT INTO correspondence_private(case_id,player_id,draft,note,version)
 SELECT $1,$2,$3,$4,$5::bigint+1 WHERE $5::bigint=0 OR EXISTS(SELECT 1 FROM correspondence_private p WHERE p.case_id=$1 AND p.player_id=$2 AND p.version=$5)
 ON CONFLICT(case_id,player_id) DO UPDATE SET draft=EXCLUDED.draft,note=EXCLUDED.note,version=correspondence_private.version+1
 WHERE correspondence_private.version=$5
 RETURNING draft,note,version`, id, player, *req.Draft, *req.Note, *req.ExpectedVersion).Scan(&result.Draft, &result.Note, &result.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		var current int64
		err = tx.QueryRow(r.Context(), `SELECT COALESCE((SELECT version FROM correspondence_private WHERE case_id=$1 AND player_id=$2),0)`, id, player).Scan(&current)
		if err != nil {
			writeError(w, 500, "could not load private save version")
			return
		}
		writeJSON(w, 409, map[string]any{"error": "private draft or note changed in another tab; keep your text and reload the saved version before retrying", "current_version": current})
		return
	}
	if err != nil {
		writeError(w, 500, "could not save private draft and note")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "could not commit private draft and note")
		return
	}
	writeJSON(w, 200, result)
}
