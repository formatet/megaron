package handlers

import (
	"errors"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/transport"
	"formatet/megaron/server/internal/unit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"math"
	"net/http"
	"strconv"
)

// TradePreview reports the current route and available hull. Dispatch rechecks
// both in its transaction; this read is a preview, never a reservation.
func (h *ProvinceHandler) TradePreview(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	originColumn := "s.province_id"
	originText := chi.URLParam(r, "provinceID")
	if sid := chi.URLParam(r, "settlementID"); sid != "" {
		originText = sid
		originColumn = "s.id"
	}
	provinceID, err := uuid.Parse(originText)
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
         FROM settlements s JOIN provinces prov ON prov.id=s.province_id
         WHERE `+originColumn+`=$1 AND s.world_id=$2 AND s.owner_id=$3`,
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

	selectedBy := r.URL.Query().Get("by")
	if err := province.ValidateTransportBy(selectedBy); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	quantity := 0.0
	if q := r.URL.Query().Get("quantity"); q != "" {
		quantity, err = strconv.ParseFloat(q, 64)
		if err != nil || math.IsNaN(quantity) || math.IsInf(quantity, 0) || quantity < 0 {
			writeError(w, 400, "invalid cargo quantity")
			return
		}
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())
	options := []map[string]any{}
	var selected map[string]any
	for _, by := range []string{"land", "sea"} {
		category := "land"
		risk := "Interception"
		requirements := []string{"A passable land route"}
		if by == "sea" {
			category = "naval"
			risk = "Storms and interception"
			requirements = []string{"Coastal access or a harbour at both cities", "A free galley or Emporos with cargo space at the origin", "Voyage provisions, checked at dispatch"}
		}
		o := map[string]any{"by": by, "available": true, "reason": "", "risk": risk, "requirements": requirements, "category": category, "cargo_capacity": nil}
		_, _, routeErr := province.ResolveTradeRouteBy(r.Context(), tx, worldID, originCoastal || originHarboured, destCoastal || destHarboured, province.MapPosition{Q: originQ, R: originR}, province.MapPosition{Q: destQ, R: destR}, by)
		if routeErr != nil {
			if !errors.Is(routeErr, province.ErrNoTradePath) {
				writeTradeJourneyError(w, routeErr)
				return
			}
			o["available"] = false
			o["reason"] = "No passable " + by + " route between these cities"
		} else {
			j, jerr := province.PlanTradeJourney(r.Context(), tx, worldID, province.MapPosition{Q: originQ, R: originR}, province.MapPosition{Q: destQ, R: destR}, category)
			if jerr != nil {
				writeTradeJourneyError(w, jerr)
				return
			}
			// This entire transaction is rolled back: a preview cannot consume a rite.
			if err := transport.BlessDeparture(r.Context(), tx, worldID, playerID, originID, &j, true); err != nil {
				writeError(w, 500, "could not preview departure")
				return
			}
			now := h.clk.Now()
			if err := economy.ClockTradeJourney(r.Context(), tx, worldID, &j, now); err != nil {
				writeError(w, 500, "could not anchor preview")
				return
			}
			at := j.Route(0, 0).WallAt(int64(j.Clock.EndTick)*1000 + j.Clock.OffsetMilli)
			o["travel_ticks"] = j.TravelTicks
			o["travel_seconds"] = at.Sub(now).Seconds()
			o["arrives_at"] = at
			if by == "sea" {
				ship, found, err := transport.FindFreeShip(r.Context(), tx, worldID, playerID, originID)
				if err != nil {
					writeError(w, 500, "could not look for free ship")
					return
				}
				if !found {
					o["available"] = false
					o["reason"] = "Sea travel requires a free galley or Emporos at the origin"
				} else {
					o["ship_id"] = ship.ID
					o["ship_type"] = ship.Type
					o["cargo_capacity"] = ship.Capacity
					name := ""
					if ship.Name != nil {
						name = *ship.Name
					}
					o["ship_name"] = unit.ShipDisplayName(ship.Type, name, originName).DisplayName
					if quantity > ship.Capacity {
						o["available"] = false
						o["reason"] = "Cargo exceeds the free ship's capacity"
					}
				}
			}
		}
		options = append(options, o)
		if selectedBy == by {
			selected = o
		}
	}
	// Omitted choices retain the historic sea preference when a free hull exists.
	if selectedBy == "" {
		for _, by := range []string{"sea", "land"} {
			for _, o := range options {
				if o["by"] == by && o["available"] == true {
					selected = o
					selectedBy = by
					break
				}
			}
			if selected != nil {
				break
			}
		}
	}
	resp := map[string]any{"options": options, "selected_by": selectedBy, "category": "", "cargo_capacity": nil}
	if selected != nil {
		for _, key := range []string{"category", "ship_id", "ship_type", "ship_name", "cargo_capacity", "travel_ticks", "travel_seconds", "arrives_at"} {
			if value, ok := selected[key]; ok {
				resp[key] = value
			}
		}
	}
	writeJSON(w, 200, resp)
}

// TransportOptions is the settlement-addressed form shared by all cargo clients.
func (h *ProvinceHandler) TransportOptions(w http.ResponseWriter, r *http.Request) {
	h.TradePreview(w, r)
}
