package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// MarchPreview estimates arrival without dispatching an order. Timing describes
// current conditions, not a reservation or a guarantee that dispatch will succeed.
func (h *UnitHandler) MarchPreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ctx := r.Context()
	playerID, ok := auth.PlayerIDFromContext(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	unitID, err := uuid.Parse(chi.URLParam(r, "unitID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid unit ID")
		return
	}
	query := r.URL.Query()
	q, err := strconv.Atoi(query.Get("target_q"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid target_q")
		return
	}
	rr, err := strconv.Atoi(query.Get("target_r"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid target_r")
		return
	}
	u, err := h.store.Get(ctx, unitID)
	if err != nil {
		writeError(w, http.StatusNotFound, "unit not found")
		return
	}
	if u.OwnerID != playerID || u.WorldID != worldID {
		writeError(w, http.StatusForbidden, "not your unit in this world")
		return
	}
	order := combat.MarchOrder{WorldID: worldID, PlayerID: playerID, UnitID: unitID,
		TargetQ: q, TargetR: rr, Intent: query.Get("intent"), Stance: query.Get("stance"),
		Name: query.Get("name"), Mode: query.Get("mode"), CargoIntent: query.Get("cargo_intent")}
	if raw := query.Get("ticks"); raw != "" {
		length, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid ticks")
			return
		}
		order.ExpeditionTicks = length
	}
	// Validate the same domain rule before returning an unavailable forecast.
	if order.Intent == "explore" {
		length, rej := combat.NormalizeExpeditionLength(order.ExpeditionTicks)
		if rej != nil {
			writeError(w, rej.Status, rej.Reason)
			return
		}
		order.ExpeditionTicks = length
	}
	targetKnown := func(fctx context.Context, target province.MapPosition, terrain string) bool {
		eyes := loadLiveEyes(fctx, h.pool, worldID, playerID, h.clk.Now())
		return province.AnyEyeSees(eyes, target, terrain) || loadRememberedTiles(fctx, h.pool, worldID, playerID)[[2]int{target.Q, target.R}]
	}
	var terrain string
	if err := h.pool.QueryRow(ctx, `SELECT terrain FROM map_tiles WHERE world_id=$1 AND q=$2 AND r=$3`, worldID, q, rr).Scan(&terrain); err != nil {
		writeError(w, http.StatusNotFound, "target hex not found")
		return
	}
	known := targetKnown(ctx, province.MapPosition{Q: q, R: rr}, terrain)
	// An area expedition selects an unseen first leg, even if the chosen
	// centre is known. Its travel time could disclose hidden geography.
	if order.Intent == "explore" {
		writeMarchPreviewUnavailable(w, "unknown_terrain")
		return
	}
	if !known {
		writeError(w, http.StatusUnprocessableEntity, "none of your men have ever seen that target — send a scout first")
		return
	}
	if u.Status == unit.StatusPositioned && u.SettlementID == nil && u.Q != nil && u.R != nil {
		if rej := combat.RequireShipInPort(ctx, h.pool, worldID, playerID, unit.CategoryOf(u.Type), u.Status, unit.LoadDisplayName(ctx, h.pool, u.ID), nil, nil, nil); rej != nil {
			writeError(w, rej.Status, rej.Reason)
			return
		}
		origin, ok := h.resolveOrderOrigin(w, ctx, worldID, playerID, province.MapPosition{Q: *u.Q, R: *u.R})
		if !ok {
			return
		}
		// A Runner can require passage on a real ship; its delivery cannot in
		// general be predicted from distance. Never present immediate departure.
		if origin.dist > 0 {
			writeMarchPreviewUnavailable(w, "courier_required")
			return
		}
	}
	result, err := combat.PreviewMarch(ctx, h.pool, h.clk, order, targetKnown)
	if err != nil {
		var rej *combat.OrderReject
		if errors.As(err, &rej) {
			writeError(w, rej.Status, rej.Reason)
		} else {
			writeError(w, http.StatusInternalServerError, "march preview failed")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"available": true, "arrival_tick": result.ArrivalTick, "duration_ticks": result.DurationTicks, "arrives_at_utc": result.ArrivesAt.UTC(), "expedition": expeditionJSON(result.Expedition)})
}

func writeMarchPreviewUnavailable(w http.ResponseWriter, reason string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"available": false, "reason": reason})
}
