package handlers

import (
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/transport"
	"formatet/megaron/server/internal/unit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
)

// TradePreview reports the current route and available hull. Dispatch rechecks
// both in its transaction; this read is a preview, never a reservation.
func (h *ProvinceHandler) TradePreview(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct{ DestinationID uuid.UUID }
	req.DestinationID, err = uuid.Parse(r.URL.Query().Get("destination_id"))
	if err != nil {
		writeError(w, 400, "invalid destination ID")
		return
	}
	// Find origin settlement and verify ownership.
	var originID uuid.UUID
	var originName string
	var originQ, originR int
	var originCoastal bool
	err = h.pool.QueryRow(r.Context(),
		`SELECT s.id, s.name, prov.map_q, prov.map_r, COALESCE(prov.coastal, false)
		 FROM settlements s
		 JOIN provinces prov ON prov.id = s.province_id
		 WHERE s.province_id = $1 AND s.world_id = $2 AND s.owner_id = $3`,
		provinceID, worldID, playerID,
	).Scan(&originID, &originName, &originQ, &originR, &originCoastal)
	if err != nil {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	// A foreign transfer is a gift, gated exactly like letters.
	var destQ, destR int
	var destCoastal bool
	var destOwnerID *uuid.UUID
	var destName, destOwnerName, destState, destTerrain string
	err = h.pool.QueryRow(r.Context(), `SELECT prov.map_q,prov.map_r,COALESCE(prov.coastal,false),s.owner_id,s.name,COALESCE(NULLIF(pl.wanax_name,''),pl.username,''),s.state,prov.terrain_type FROM settlements s JOIN provinces prov ON prov.id=s.province_id LEFT JOIN players pl ON pl.id=s.owner_id WHERE s.id=$1 AND s.world_id=$2`, req.DestinationID, worldID).Scan(&destQ, &destR, &destCoastal, &destOwnerID, &destName, &destOwnerName, &destState, &destTerrain)
	if err != nil {
		writeError(w, 404, "destination settlement not found")
		return
	}
	if destOwnerID == nil {
		writeError(w, 403, "destination has no Wanax to receive goods")
		return
	}
	isGift := *destOwnerID != playerID
	if isGift {
		if destState != "active" {
			writeError(w, 403, "destination settlement is not active")
			return
		}
		if !knownToPlayer(loadLiveEyes(r.Context(), h.pool, worldID, playerID, h.clk.Now()), loadRememberedTiles(r.Context(), h.pool, worldID, playerID), province.MapPosition{Q: destQ, R: destR}, destTerrain) {
			writeError(w, 403, "this city is not known on your map — discover or contact it before sending a gift")
			return
		}
	}

	// A navigable coastal route still needs a real ship; otherwise use a real land path.
	originHarboured, err := settlementHasHarbour(r.Context(), h.pool, originID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check origin harbour")
		return
	}
	destHarboured, err := settlementHasHarbour(r.Context(), h.pool, req.DestinationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check destination harbour")
		return
	}
	category, _, err := province.ResolveTradeRoute(r.Context(), h.pool, worldID,
		originCoastal || originHarboured, destCoastal || destHarboured,
		province.MapPosition{Q: originQ, R: originR},
		province.MapPosition{Q: destQ, R: destR},
	)
	if err != nil {
		writeTradeJourneyError(w, err)
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())
	resp := map[string]any{"category": category, "cargo_capacity": nil}
	if category == "naval" {
		ship, found, err := transport.FindFreeShip(r.Context(), tx, worldID, playerID, originID)
		if err != nil {
			writeError(w, 500, "could not look for free ship")
			return
		}
		if found {
			resp["ship_id"] = ship.ID
			resp["ship_type"] = ship.Type
			resp["cargo_capacity"] = ship.Capacity
			name := ""
			if ship.Name != nil {
				name = *ship.Name
			}
			resp["ship_name"] = unit.ShipDisplayName(ship.Type, name, originName).DisplayName
		} else {
			if _, err := province.PlanTradeJourney(r.Context(), tx, worldID, province.MapPosition{Q: originQ, R: originR}, province.MapPosition{Q: destQ, R: destR}, "land"); err != nil {
				writeError(w, 422, "no free galley or merchantman and no land route")
				return
			}
			resp["category"] = "land"
		}
	}
	writeJSON(w, 200, resp)
}
