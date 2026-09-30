package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"formatet/megaron/server/internal/auth"
)

// ruralProjectionTypes is the building set projected onto catchment hexes in the
// Fas A2 slice (megaron_lokal_varld.md). The set is a sequencing limit, not
// canon — widen the slice by adding here (and giving the client a sprite in
// render/map.js). stonequarry is hex-bound too (province.HexBoundBuildings)
// but has no sprite yet, so it stays out of this list on purpose.
var ruralProjectionTypes = []string{"farm", "mine", "lumbermill"}

// ruralProjection is one map-anchored representation of a city building on a
// compatible catchment hex — a CARTOGRAPHIC PROJECTION of an existing settlement
// building, never a standalone economic object (megaron_lokal_varld.md
// §Ruralprojektion). The mechanical building still lives in the settlement's
// building row; the client draws a sprite here and its object card leads back to
// the city's building context.
type ruralProjection struct {
	SettlementID uuid.UUID `json:"settlement_id"`
	ProvinceID   uuid.UUID `json:"province_id"`
	Name         string    `json:"name"`
	BuildingType string    `json:"building_type"`
	Q            int       `json:"q"`
	R            int       `json:"r"`
}

// RuralProjections returns rural building projections for the authenticated
// player's OWN settlements. Buildings are hex-bound now (migration 154,
// megaron_plan_byggnad_pa_hex.md §A) — the building already carries its real
// hex_q/hex_r, chosen by the player at build time and validated by the build
// endpoint (catchment reach + terrain/deposit match). This handler no longer
// invents a plausible hex per building TYPE; it emits one projection per
// hex-bound building ROW, at its actual hex — two farms on two different
// hexes yield two sprites. Only own cities are emitted, so the owner's
// catchment is inherently within their FOW; no extra fog gating is needed.
func (h *WorldHandler) RuralProjections(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, authenticated := auth.PlayerIDFromContext(r.Context())
	if !authenticated {
		writeJSON(w, http.StatusOK, []ruralProjection{})
		return
	}

	// One row per hex-bound building of a sprite-supported type
	// (ruralProjectionTypes). Hexes occupied by any other province are
	// excluded so a projection never lands on a neighbour's city (kept from
	// before the hex-bound slice — still possible via hexägarskap's shared
	// catchments, megaron_plan_hexagarskap_och_stadsavstand.md §3).
	rows, err := h.pool.Query(r.Context(),
		`SELECT s.id, s.province_id, s.name, b.building_type, b.hex_q, b.hex_r
		 FROM settlements s
		 JOIN buildings b ON b.settlement_id = s.id
		     AND b.building_type = ANY($3) AND b.hex_q IS NOT NULL
		 WHERE s.world_id = $1 AND s.owner_id = $2
		   AND NOT EXISTS (
		       SELECT 1 FROM provinces p2
		       WHERE p2.world_id = $1 AND p2.map_q = b.hex_q AND p2.map_r = b.hex_r)
		 ORDER BY b.hex_q, b.hex_r`,
		worldID, playerID, ruralProjectionTypes,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load rural projections")
		return
	}
	defer rows.Close()

	out := []ruralProjection{}
	for rows.Next() {
		var sid, pid uuid.UUID
		var name, btype string
		var q, rr int
		if err := rows.Scan(&sid, &pid, &name, &btype, &q, &rr); err != nil {
			continue
		}
		out = append(out, ruralProjection{
			SettlementID: sid,
			ProvinceID:   pid,
			Name:         name,
			BuildingType: btype,
			Q:            q,
			R:            rr,
		})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load rural projections")
		return
	}

	writeJSON(w, http.StatusOK, out)
}
