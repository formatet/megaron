package handlers

import (
	"encoding/json"
	"fmt"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/world"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

func cleanPlaceName(raw string) (string, string) {
	name := strings.Join(strings.Fields(raw), " ")
	n := utf8.RuneCountInString(name)
	if n < stormNameMin || n > stormNameMax {
		return "", "a place's name must be 2 to 30 characters"
	}
	if !stormNameRe.MatchString(name) {
		return "", "a place's name may hold letters, digits, spaces, apostrophes and hyphens only"
	}
	return name, ""
}

func placeIDs(w http.ResponseWriter, r *http.Request) (world uuid.UUID, kind string, anchor [2]int, ok bool) {
	world, err := uuid.Parse(chi.URLParam(r, "worldID"))
	kind = chi.URLParam(r, "kind")
	parts := strings.Split(chi.URLParam(r, "placeID"), ",")
	if err != nil || len(parts) != 2 {
		writeError(w, 400, "invalid world or place ID")
		return
	}
	anchor[0], err = strconv.Atoi(parts[0])
	if err == nil {
		anchor[1], err = strconv.Atoi(parts[1])
	}
	if err != nil || !(kind == "river" || kind == "massif" || kind == "desert" || kind == "forest") {
		writeError(w, 400, "invalid place kind or anchor")
		return
	}
	ok = true
	return
}

func readPlaceName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return "", false
	}
	name, msg := cleanPlaceName(body.Name)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return "", false
	}
	return name, true
}

// ensurePlaceRow validates the requested canonical object, then creates only its
// empty identity row. It never grants a claim or replays historical movement.
func (h *WorldHandler) ensurePlaceRow(r *http.Request, wid uuid.UUID, kind string, anchor [2]int) (bool, error) {
	tiles, err := loadGeography(r.Context(), h.pool, wid)
	if err != nil {
		return false, err
	}
	found := false
	for _, p := range world.Places(tiles) {
		if p.Kind == kind && p.Anchor == anchor {
			found = true
			break
		}
	}
	if !found {
		return false, nil
	}
	_, err = h.pool.Exec(r.Context(), `INSERT INTO place_names(world_id,kind,anchor_q,anchor_r) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, wid, kind, anchor[0], anchor[1])
	return true, err
}

// NamePlace handles POST /worlds/{worldID}/places/{anchor}/name — the first Wanax to reach a place names it.
func (h *WorldHandler) NamePlace(w http.ResponseWriter, r *http.Request) {
	worldID, kind, anchor, ok := placeIDs(w, r)
	if !ok {
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	name, ok := readPlaceName(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	var claimedBy *uuid.UUID
	var namedTick *int
	var tick int
	if err := h.pool.QueryRow(ctx,
		`SELECT s.claimed_by, s.named_tick, w.current_tick FROM place_names s JOIN worlds w ON w.id = s.world_id
		  WHERE s.anchor_q = $1 AND s.anchor_r = $2 AND s.world_id = $3 AND s.kind = $4`, anchor[0], anchor[1], worldID, kind).Scan(&claimedBy, &namedTick, &tick); err != nil {
		writeError(w, http.StatusNotFound, "no such place")
		return
	}
	if claimedBy == nil || *claimedBy != playerID {
		writeError(w, http.StatusForbidden, "only the first Wanax to reach a place may name it")
		return
	}
	if namedTick != nil {
		writeError(w, http.StatusConflict, "this place has already been named")
		return
	}
	// named_tick IS NULL in the WHERE keeps two racing requests to a single winner.
	tag, err := h.pool.Exec(ctx,
		`UPDATE place_names SET name = $5, named_tick = $6 WHERE anchor_q = $1 AND anchor_r = $2 AND world_id=$3 AND kind=$4 AND named_tick IS NULL AND claimed_by=$7`, anchor[0], anchor[1], worldID, kind, name, tick, playerID)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "another place in this world already has that name")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not name the place")
		return
	}
	if tag.RowsAffected() != 1 {
		writeError(w, http.StatusConflict, "this place has already been named")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"kind": kind, "place_id": fmt.Sprintf("%d,%d", anchor[0], anchor[1]), "name": name})
}

// AdminNamePlace handles PUT /admin/worlds/{worldID}/places/{anchor}/name (X-Admin-Key):
// set or change a name, bypassing the once-only rule.
func (h *WorldHandler) AdminNamePlace(w http.ResponseWriter, r *http.Request) {
	if !requireAdminKey(w, r) {
		return
	}
	worldID, kind, anchor, ok := placeIDs(w, r)
	if !ok {
		return
	}
	if exists, err := h.ensurePlaceRow(r, worldID, kind, anchor); err != nil {
		writeError(w, 500, "could not load geography")
		return
	} else if !exists {
		writeError(w, 404, "no such place")
		return
	}

	name, ok := readPlaceName(w, r)
	if !ok {
		return
	}
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE place_names s SET name = $5, named_tick = COALESCE(s.named_tick, w.current_tick)
		   FROM worlds w WHERE s.anchor_q=$1 AND s.anchor_r=$2 AND s.world_id=$3 AND s.kind=$4 AND w.id=s.world_id`, anchor[0], anchor[1], worldID, kind, name)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "another place in this world already has that name")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not name the place")
		return
	}
	if tag.RowsAffected() != 1 {
		writeError(w, http.StatusNotFound, "no such place")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"kind": kind, "place_id": fmt.Sprintf("%d,%d", anchor[0], anchor[1]), "name": name})
}

// AdminClearPlaceName handles DELETE /admin/worlds/{worldID}/places/{anchor}/name: the place
// becomes unnamed and its namer's right stays spent (named_tick is kept, or set now).
func (h *WorldHandler) AdminClearPlaceName(w http.ResponseWriter, r *http.Request) {
	if !requireAdminKey(w, r) {
		return
	}
	worldID, kind, anchor, ok := placeIDs(w, r)
	if !ok {
		return
	}
	if exists, err := h.ensurePlaceRow(r, worldID, kind, anchor); err != nil {
		writeError(w, 500, "could not load geography")
		return
	} else if !exists {
		writeError(w, 404, "no such place")
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`UPDATE place_names s SET name = NULL, named_tick = COALESCE(s.named_tick, w.current_tick)
		   FROM worlds w WHERE s.anchor_q=$1 AND s.anchor_r=$2 AND s.world_id=$3 AND s.kind=$4 AND w.id=s.world_id`, anchor[0], anchor[1], worldID, kind)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not clear the name")
		return
	}
	if tag.RowsAffected() != 1 {
		writeError(w, http.StatusNotFound, "no such place")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
