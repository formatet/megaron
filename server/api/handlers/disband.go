package handlers

import (
	"encoding/json"
	"net/http"
	"sort"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/unit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Disband returns a selected whole unit's actual people, never a requested quantity.
func (h *ProvinceHandler) Disband(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, 400, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, 400, "invalid province ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}
	var raw map[string]json.RawMessage
	const whole = "disband a whole unit by its id: send unit_ids, not a quantity of soldiers"
	if json.NewDecoder(r.Body).Decode(&raw) != nil || len(raw) != 1 || raw["unit_ids"] == nil {
		writeError(w, 400, whole)
		return
	}
	var ids []uuid.UUID
	if json.Unmarshal(raw["unit_ids"], &ids) != nil || len(ids) == 0 {
		writeError(w, 400, whole)
		return
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range ids {
		if id == uuid.Nil || seen[id] {
			writeError(w, 400, "choose each whole unit id only once")
			return
		}
		seen[id] = true
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, 500, "transaction error")
		return
	}
	defer tx.Rollback(ctx)
	// Same city -> unit lock order as Lay up / Fit out, with stable unit order for batches.
	var settlementID uuid.UUID
	var city string
	if err = tx.QueryRow(ctx, `SELECT id,name FROM settlements WHERE province_id=$1 AND world_id=$2 AND owner_id=$3 AND state='active' FOR UPDATE`, provinceID, worldID, playerID).Scan(&settlementID, &city); err != nil {
		writeError(w, 403, "not your active settlement")
		return
	}
	type result struct {
		ID          uuid.UUID `json:"id"`
		Type        string    `json:"type"`
		DisplayName string    `json:"display_name"`
		People      int       `json:"people_returned"`
	}
	selected := make([]result, 0, len(ids))
	popRestored := 0
	for _, id := range ids {
		var owner, world uuid.UUID
		var at *uuid.UUID
		var typ, category, status string
		var size, crew int
		err = tx.QueryRow(ctx, `SELECT owner_id,world_id,settlement_id,type,category,status,size,COALESCE(crew,0) FROM units WHERE id=$1 FOR UPDATE`, id).Scan(&owner, &world, &at, &typ, &category, &status, &size, &crew)
		if err != nil || owner != playerID || world != worldID {
			writeError(w, 403, "not your unit in this world")
			return
		}
		if at == nil || *at != settlementID || (status != "garrison" && status != "laid_up") {
			writeError(w, 422, "the whole unit must be in this city's garrison or laid up here")
			return
		}
		switch typ {
		case "spearman", "elite_infantry", "war_chariot", "galley", "war_galley", "merchantman":
		default:
			writeError(w, 422, "this unit cannot be disbanded in a city")
			return
		}
		people := size
		if category == "naval" {
			people = crew
			if status == "laid_up" {
				people = 0
			}
		}
		selected = append(selected, result{id, typ, unit.LoadDisplayName(ctx, tx, id), people})
		popRestored += people
	}
	// Validate the entire batch before changing any of it. No partial shrink path exists.
	for _, u := range selected {
		if _, err = tx.Exec(ctx, `UPDATE units SET status='disbanded',updated_at=now() WHERE id=$1`, u.ID); err != nil {
			writeError(w, 500, "disband failed")
			return
		}
	}
	var popAfter int
	if err = tx.QueryRow(ctx, `UPDATE settlements SET population=population+$1 WHERE id=$2 RETURNING population`, popRestored, settlementID).Scan(&popAfter); err != nil {
		writeError(w, 500, "could not restore population")
		return
	}
	if err = economy.RecomputeProduction(ctx, tx, settlementID); err != nil {
		writeError(w, 500, "recompute production failed")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		writeError(w, 500, "commit failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"disbanded": selected, "pop_restored": popRestored, "population": popAfter, "settlement_name": city})
}
