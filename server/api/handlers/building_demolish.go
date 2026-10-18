package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// BuildingDemolishedPayload is the outcome stored with the BuildingDemolished
// event (CLAUDE.md: events store outcomes). CitizensLifted counts gubbar taken
// off their places; they are idle afterwards, never removed from the population.
type BuildingDemolishedPayload struct {
	BuildingID     uuid.UUID `json:"building_id"`
	SettlementID   uuid.UUID `json:"settlement_id"`
	BuildingType   string    `json:"building_type"`
	Level          int       `json:"level"`
	HexQ           *int      `json:"hex_q,omitempty"`
	HexR           *int      `json:"hex_r,omitempty"`
	CitizensLifted int       `json:"citizens_lifted"`
}

// DemolishBuilding handles DELETE
// /worlds/:worldID/provinces/:provinceID/buildings/:buildingID — a Wanax tears
// down a finished building in their own city so the hex/slot is free at once
// for a new build (Timothy: "absolut kunna riva för att bygga om"). The id may
// also be a build_queue id: demolishing something still under construction is
// cancelling it (refunded, as CancelBuild). No refund for a finished building.
func (h *ProvinceHandler) DemolishBuilding(w http.ResponseWriter, r *http.Request) {
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
	buildingID, err := uuid.Parse(chi.URLParam(r, "buildingID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid building ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	ctx := r.Context()

	var settlementID uuid.UUID
	var ownerID *uuid.UUID
	var bType string
	var level int
	var hexQ, hexR *int
	queued := false
	err = h.pool.QueryRow(ctx,
		`SELECT b.settlement_id, s.owner_id, b.building_type, b.level, b.hex_q, b.hex_r
		 FROM buildings b JOIN settlements s ON s.id = b.settlement_id
		 WHERE b.id = $1 AND s.world_id = $2 AND s.province_id = $3`,
		buildingID, worldID, provinceID,
	).Scan(&settlementID, &ownerID, &bType, &level, &hexQ, &hexR)
	if errors.Is(err, pgx.ErrNoRows) {
		err = h.pool.QueryRow(ctx,
			`SELECT q.settlement_id, s.owner_id, q.building_type, q.hex_q, q.hex_r
			 FROM build_queue q JOIN settlements s ON s.id = q.settlement_id
			 WHERE q.id = $1 AND s.world_id = $2 AND s.province_id = $3`,
			buildingID, worldID, provinceID,
		).Scan(&settlementID, &ownerID, &bType, &hexQ, &hexR)
		queued = true
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "no such building")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load building")
		return
	}
	if ownerID == nil || *ownerID != playerID {
		writeError(w, http.StatusForbidden, "not your city")
		return
	}

	fail := func(msg string, err error) {
		slog.ErrorContext(ctx, "demolish failed", "stage", msg, "world_id", worldID,
			"settlement_id", settlementID, "building_id", buildingID, "player_id", playerID, "err", err)
		writeError(w, http.StatusInternalServerError, msg)
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		fail("transaction error", err)
		return
	}
	defer tx.Rollback(ctx)

	// Lock the settlement (and re-check ownership) so concurrent placement
	// changes and a second demolish serialize behind this one.
	var lockedOwner *uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT owner_id FROM settlements WHERE id = $1 FOR UPDATE`, settlementID,
	).Scan(&lockedOwner); err != nil {
		fail("could not lock settlement", err)
		return
	}
	if lockedOwner == nil || *lockedOwner != playerID {
		writeError(w, http.StatusForbidden, "not your city")
		return
	}

	if queued {
		if _, err := cancelQueuedBuild(ctx, tx, buildingID); err != nil {
			if errors.Is(err, errQueuedBuildGone) {
				writeError(w, http.StatusNotFound, "no such building")
				return
			}
			fail("could not cancel build", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			fail("commit failed", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"cancelled": true, "building_type": bType, "hex_q": hexQ, "hex_r": hexR, "citizens_lifted": 0,
		})
		return
	}

	// Delete first: a concurrent demolish that got here before us leaves 0 rows.
	ct, err := tx.Exec(ctx, `DELETE FROM buildings WHERE id = $1`, buildingID)
	if err != nil {
		fail("could not demolish", err)
		return
	}
	if ct.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "no such building")
		return
	}

	// A queued upgrade of the same building would otherwise complete as a new
	// level-1 building later — cancel (and refund) it with the building.
	qrows, err := tx.Query(ctx,
		`SELECT id FROM build_queue
		 WHERE settlement_id = $1 AND building_type = $2
		   AND hex_q IS NOT DISTINCT FROM $3 AND hex_r IS NOT DISTINCT FROM $4`,
		settlementID, bType, hexQ, hexR)
	if err != nil {
		fail("could not read build queue", err)
		return
	}
	var queueIDs []uuid.UUID
	for qrows.Next() {
		var id uuid.UUID
		if err := qrows.Scan(&id); err == nil {
			queueIDs = append(queueIDs, id)
		}
	}
	qrows.Close()
	for _, id := range queueIDs {
		if _, err := cancelQueuedBuild(ctx, tx, id); err != nil && !errors.Is(err, errQueuedBuildGone) {
			fail("could not cancel queued upgrade", err)
			return
		}
	}

	lifted, err := economy.LiftPlacementsAfterDemolish(ctx, tx, settlementID, bType, hexQ, hexR)
	if err != nil {
		fail("could not lift citizens", err)
		return
	}
	if err := economy.RecomputeProduction(ctx, tx, settlementID); err != nil {
		fail("recompute production failed", err)
		return
	}

	var ev *events.Event
	if h.eventStore != nil {
		ev, err = h.eventStore.AppendTx(ctx, tx, settlementID, events.StreamProvince, "BuildingDemolished",
			BuildingDemolishedPayload{
				BuildingID: buildingID, SettlementID: settlementID, BuildingType: bType, Level: level,
				HexQ: hexQ, HexR: hexR, CitizensLifted: len(lifted),
			}, worldID, nil)
		if err != nil {
			fail("could not record event", err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		fail("commit failed", err)
		return
	}
	if ev != nil {
		h.eventStore.RecordCommitted(ctx, ev)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"demolished": true, "building_type": bType, "level": level,
		"hex_q": hexQ, "hex_r": hexR, "citizens_lifted": len(lifted),
	})
}
