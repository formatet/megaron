package handlers

import (
	"net/http"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/province"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// foreignUnit is the JSON shape returned by ForeignUnits for one enemy/neutral
// unit the caller currently has eyes on.
//
// Q/R here are the unit's INTERPOLATED CURRENT position — not the stored origin
// hex. This is the opposite convention from unitSummary (unit.go), where q/r is
// the stored origin and the client interpolates itself; a foreign unit's stored
// row is never exposed, only what the viewer's eyes actually see right now, so
// the server does the interpolation once instead of teaching the client to
// re-derive a stranger's march the way it derives its own.
type foreignUnit struct {
	ID       uuid.UUID `json:"id"`
	Owner    string    `json:"owner"`
	OwnerID  uuid.UUID `json:"owner_id"`
	Type     string    `json:"type"`
	Category string    `json:"category"`
	Size     int       `json:"size"`
	Crew     int       `json:"crew,omitempty"`
	Status   string    `json:"status"`
	Stance   string    `json:"stance,omitempty"`
	Q        int       `json:"q"`
	R        int       `json:"r"`
	// TargetQ/TargetR/DepartsAt/ArrivesAt/DepartTick/ArrivalTick/Path describe the
	// march itself (kanonbeslut 3, 2026-08-03): full disclosure of what the unit is
	// doing, since that is knowledge about the UNIT, not the map. They never cause
	// the target/route hexes to be revealed — no terrain, no city, no
	// player_scouted_tiles row is written or read for them here.
	TargetQ     *int       `json:"target_q,omitempty"`
	TargetR     *int       `json:"target_r,omitempty"`
	DepartsAt   *time.Time `json:"departs_at,omitempty"`
	ArrivesAt   *time.Time `json:"arrives_at,omitempty"`
	DepartTick  *int       `json:"depart_tick,omitempty"`
	ArrivalTick *int       `json:"arrival_tick,omitempty"`
	Path        [][2]int   `json:"path,omitempty"`
}

// ForeignUnits handles GET /worlds/{worldID}/foreign-units — every unit NOT owned
// by the caller whose current position sits in the caller's tier-1 (live) vision.
//
// This is the informational surface temenos_synlighet.md/megaron_kartaktorer §Relation
// always specified but that was never built: enemy/neutral units had no visibility
// path at all (garrison/forming units are reported via a city's army_total instead,
// see world.go's provinceMarker.ArmyTotal gate). Kanonbeslut Timothy 2026-08-03:
//  1. Live tier reveals a foreign unit in full — type, size AND stance. No blur.
//  2. Remembered (tier 2) reveals nothing — memory carries no activity.
//  3. A march is revealed in full (target/timing/route) as knowledge about the
//     UNIT, but never turns the target/route hexes into map knowledge — the gate
//     that decides whether the unit itself is listed is its CURRENT interpolated
//     position, exactly the same two calls (FindPath + interpolatedEyePos) used to
//     place the viewer's own eyes in loadLiveEyes, so the watcher and the watched
//     never disagree about where anything is.
//
// Unauthenticated callers always get an empty list — unlike /map, which shows the
// whole world to a logged-out caller, no unit data leaves the server for someone
// who isn't a player.
func (h *WorldHandler) ForeignUnits(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, authenticated := auth.PlayerIDFromContext(r.Context())
	if !authenticated {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	now := h.clk.Now()
	eyes := loadLiveEyes(r.Context(), h.pool, worldID, playerID, now)

	g, err := province.LoadTileGraph(r.Context(), h.pool, worldID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load terrain")
		return
	}

	// garrison/forming units live inside a settlement and have no q/r of their
	// own — they surface via the city's army_total (world.go §2b), not here.
	// embarked units carry no position; they move with their ship. Kingdoms are
	// post-MVP and disabled, so every non-owner row here is treated as foreign.
	rows, err := h.pool.Query(r.Context(),
		`SELECT u.id, u.owner_id, COALESCE(pl.wanax_name, pl.username, ''), u.type, u.category, u.size, u.crew,
		        u.status, u.stance, u.q, u.r,
		        u.target_q, u.target_r, u.departs_at, u.arrives_at, u.depart_tick, u.arrive_tick
		 FROM units u
		 LEFT JOIN players pl ON pl.id = u.owner_id
		 WHERE u.world_id = $1 AND u.owner_id IS DISTINCT FROM $2
		   AND u.status IN ('positioned','marching')
		   AND u.q IS NOT NULL AND u.r IS NOT NULL`,
		worldID, playerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load foreign units")
		return
	}
	defer rows.Close()

	var out []foreignUnit
	for rows.Next() {
		var fu foreignUnit
		var stance *string
		var storedQ, storedR int
		var targetQ, targetR, departTick, arriveTick *int
		var departsAt, arrivesAt *time.Time
		if err := rows.Scan(&fu.ID, &fu.OwnerID, &fu.Owner, &fu.Type, &fu.Category, &fu.Size, &fu.Crew,
			&fu.Status, &stance, &storedQ, &storedR,
			&targetQ, &targetR, &departsAt, &arrivesAt, &departTick, &arriveTick); err != nil {
			continue
		}
		if stance != nil {
			fu.Stance = *stance
		}

		// Current position: interpolate a marching unit exactly like loadLiveEyes
		// interpolates the caller's own eyes, so watcher and watched share one
		// position model. Fall back to the stored (origin) hex when pathfinding
		// fails or the unit is merely 'positioned'.
		pos := province.MapPosition{Q: storedQ, R: storedR}
		var path []province.MapPosition
		if fu.Status == "marching" && targetQ != nil && targetR != nil && departsAt != nil && arrivesAt != nil {
			p, _, ok := g.FindPath(pos, province.MapPosition{Q: *targetQ, R: *targetR}, fu.Category)
			if ok && len(p) > 0 {
				path = p
				pos = interpolatedEyePos(now, *departsAt, *arrivesAt, p)
			}
		}

		// Fail closed: no map_tiles row for the unit's current hex means we cannot
		// determine visibility for it at all — omit it rather than guess.
		terrain, ok := g[[2]int{pos.Q, pos.R}]
		if !ok {
			continue
		}
		if !province.AnyEyeSees(eyes, pos, terrain) {
			continue
		}

		fu.Q = pos.Q
		fu.R = pos.R
		if fu.Status == "marching" {
			fu.TargetQ = targetQ
			fu.TargetR = targetR
			fu.DepartsAt = departsAt
			fu.ArrivesAt = arrivesAt
			fu.DepartTick = departTick
			fu.ArrivalTick = arriveTick
			if len(path) > 0 {
				wp := make([][2]int, len(path))
				for i, p := range path {
					wp[i] = [2]int{p.Q, p.R}
				}
				fu.Path = wp
			}
		}
		out = append(out, fu)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load foreign units")
		return
	}
	if out == nil {
		out = []foreignUnit{}
	}
	writeJSON(w, http.StatusOK, out)
}
