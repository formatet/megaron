package handlers

import (
	"fmt"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/world"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
)

// Places lists only the known hexes of static geographic objects. Naming rights
// never expose the unknown remainder of a component or its geometry.
func (h *WorldHandler) Places(w http.ResponseWriter, r *http.Request) {
	wid, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, 400, "invalid world ID")
		return
	}
	viewer, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, 401, "authentication required")
		return
	}
	tiles, err := loadGeography(r.Context(), h.pool, wid)
	if err != nil {
		writeError(w, 500, "could not load geography")
		return
	}
	states, err := loadPlaceStates(r.Context(), h.pool, wid)
	if err != nil {
		writeError(w, 500, "could not load place names")
		return
	}
	eyes := loadLiveEyes(r.Context(), h.pool, wid, viewer, h.clk.Now())
	memory := loadRememberedTiles(r.Context(), h.pool, wid, viewer)
	known := map[[2]int]bool{}
	for _, t := range tiles {
		pos := [2]int{t.Q, t.R}
		known[pos] = memory[pos] || province.AnyEyeSees(eyes, province.MapPosition{Q: t.Q, R: t.R}, string(t.Terrain))
	}
	type hex struct {
		Q int `json:"q"`
		R int `json:"r"`
	}
	type place struct {
		Kind    string `json:"kind"`
		ID      string `json:"id"`
		Name    string `json:"name"`
		CanName bool   `json:"can_name,omitempty"`
		Hexes   []hex  `json:"hexes"`
		// Known only from stories told in letters (megaron_plan_vagbeskrivningar
		// v1): those hexes and who told of them. Never retellable.
		ToldHexes []hex    `json:"told_hexes,omitempty"`
		ToldBy    []string `json:"told_by,omitempty"`
		// Tellable: named and seen first-hand, so a letter can tell of it.
		Tellable bool `json:"tellable,omitempty"`
	}
	toldBy := loadToldBy(r.Context(), h.pool, wid, viewer)
	out := []place{}
	for _, p := range world.Places(tiles) {
		s := states[placeKey{p.Kind, p.Anchor}]
		v := place{Kind: p.Kind, ID: fmt.Sprintf("%d,%d", p.Anchor[0], p.Anchor[1]), Name: s.Name, CanName: s.ClaimedBy != nil && *s.ClaimedBy == viewer && !s.Named}
		tellers := map[string]bool{}
		for _, xy := range p.Hexes {
			if known[xy] {
				v.Hexes = append(v.Hexes, hex{xy[0], xy[1]})
			} else if name, ok := toldBy[xy]; ok {
				v.ToldHexes = append(v.ToldHexes, hex{xy[0], xy[1]})
				if !tellers[name] {
					tellers[name] = true
					v.ToldBy = append(v.ToldBy, name)
				}
			}
		}
		v.Tellable = s.Name != "" && len(v.Hexes) > 0
		if len(v.Hexes) > 0 || len(v.ToldHexes) > 0 {
			out = append(out, v)
		}
	}
	writeJSON(w, 200, map[string]any{"places": out})
}
