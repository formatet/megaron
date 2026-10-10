package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// Stormnamn (megaron_plan_stormnamn.md): the owner of the first ship a storm strikes
// (sea_storms.claimed_by, set by combat.claimStorm) may name it, once. The name stays with
// the storm. Admin can set, change or clear a name with X-Admin-Key; a cleared name keeps
// named_tick, so the same player cannot set a foul name again.

const (
	stormNameMin = 2
	stormNameMax = 30
)

var stormNameRe = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} '\-]*[\p{L}\p{N}]$`)

// cleanStormName trims, collapses runs of spaces and checks length and characters.
func cleanStormName(raw string) (string, string) {
	name := strings.Join(strings.Fields(raw), " ")
	n := utf8.RuneCountInString(name)
	if n < stormNameMin || n > stormNameMax {
		return "", "a storm's name must be 2 to 30 characters"
	}
	if !stormNameRe.MatchString(name) {
		return "", "a storm's name may hold letters, digits, spaces, apostrophes and hyphens only"
	}
	return name, ""
}

func isUniqueViolation(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

func stormIDs(w http.ResponseWriter, r *http.Request) (world, storm uuid.UUID, ok bool) {
	world, err1 := uuid.Parse(chi.URLParam(r, "worldID"))
	storm, err2 := uuid.Parse(chi.URLParam(r, "stormID"))
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "invalid world or storm ID")
		return world, storm, false
	}
	return world, storm, true
}

func readStormName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return "", false
	}
	name, msg := cleanStormName(body.Name)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return "", false
	}
	return name, true
}

// NameStorm handles POST /worlds/{worldID}/storms/{stormID}/name — the first Wanax to meet a storm names it.
func (h *WorldHandler) NameStorm(w http.ResponseWriter, r *http.Request) {
	worldID, stormID, ok := stormIDs(w, r)
	if !ok {
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	name, ok := readStormName(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	var claimedBy *uuid.UUID
	var namedTick *int
	var tick int
	if err := h.pool.QueryRow(ctx,
		`SELECT s.claimed_by, s.named_tick, w.current_tick FROM sea_storms s JOIN worlds w ON w.id = s.world_id
		  WHERE s.id = $1 AND s.world_id = $2`, stormID, worldID).Scan(&claimedBy, &namedTick, &tick); err != nil {
		writeError(w, http.StatusNotFound, "no such storm")
		return
	}
	if claimedBy == nil || *claimedBy != playerID {
		writeError(w, http.StatusForbidden, "only the first Wanax to meet a storm may name it")
		return
	}
	if namedTick != nil {
		writeError(w, http.StatusConflict, "this storm has already been named")
		return
	}
	// named_tick IS NULL in the WHERE keeps two racing requests to a single winner.
	tag, err := h.pool.Exec(ctx,
		`UPDATE sea_storms SET name = $2, named_tick = $3 WHERE id = $1 AND named_tick IS NULL`, stormID, name, tick)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "another storm in this world already has that name")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not name the storm")
		return
	}
	if tag.RowsAffected() != 1 {
		writeError(w, http.StatusConflict, "this storm has already been named")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"storm_id": stormID, "name": name})
}

// AdminNameStorm handles PUT /admin/worlds/{worldID}/storms/{stormID}/name (X-Admin-Key):
// set or change a name, bypassing the once-only rule.
func (h *WorldHandler) AdminNameStorm(w http.ResponseWriter, r *http.Request) {
	if !requireAdminKey(w, r) {
		return
	}
	worldID, stormID, ok := stormIDs(w, r)
	if !ok {
		return
	}
	name, ok := readStormName(w, r)
	if !ok {
		return
	}
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE sea_storms s SET name = $3, named_tick = COALESCE(s.named_tick, w.current_tick)
		   FROM worlds w WHERE s.id = $1 AND s.world_id = $2 AND w.id = s.world_id`, stormID, worldID, name)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "another storm in this world already has that name")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not name the storm")
		return
	}
	if tag.RowsAffected() != 1 {
		writeError(w, http.StatusNotFound, "no such storm")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"storm_id": stormID, "name": name})
}

// AdminClearStormName handles DELETE /admin/worlds/{worldID}/storms/{stormID}/name: the storm
// becomes unnamed and its namer's right stays spent (named_tick is kept, or set now).
func (h *WorldHandler) AdminClearStormName(w http.ResponseWriter, r *http.Request) {
	if !requireAdminKey(w, r) {
		return
	}
	worldID, stormID, ok := stormIDs(w, r)
	if !ok {
		return
	}
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE sea_storms s SET name = NULL, named_tick = COALESCE(s.named_tick, w.current_tick)
		   FROM worlds w WHERE s.id = $1 AND s.world_id = $2 AND w.id = s.world_id`, stormID, worldID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not clear the name")
		return
	}
	if tag.RowsAffected() != 1 {
		writeError(w, http.StatusNotFound, "no such storm")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
