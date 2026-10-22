package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/kharis"
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
	var godID *string
	queued := false
	err = h.pool.QueryRow(ctx,
		`SELECT b.settlement_id, s.owner_id, b.building_type, b.level, b.hex_q, b.hex_r, b.god_id
		 FROM buildings b JOIN settlements s ON s.id = b.settlement_id
		 WHERE b.id = $1 AND s.world_id = $2 AND s.province_id = $3`,
		buildingID, worldID, provinceID,
	).Scan(&settlementID, &ownerID, &bType, &level, &hexQ, &hexR, &godID)
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

	// The initial lookup is only routing. Charge the finished level that actually
	// exists under lock, never the stale level from before a concurrent upgrade.
	if err := tx.QueryRow(ctx, `SELECT building_type,level,hex_q,hex_r,god_id FROM buildings WHERE id=$1 AND settlement_id=$2 FOR UPDATE`, buildingID, settlementID).Scan(&bType, &level, &hexQ, &hexR, &godID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 404, "no such building")
			return
		}
		fail("could not lock building", err)
		return
	}
	var demolition *kharis.TempleDemolition
	if bType == "temple" {
		// Serialize debt across simultaneous demolitions in different owned cities.
		var record uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT player_id FROM player_world_records WHERE player_id=$1 AND world_id=$2 FOR UPDATE`, playerID, worldID).Scan(&record); err != nil {
			fail("could not lock kharis", err)
			return
		}
		demolition, err = kharis.ProjectTempleDemolition(ctx, tx, playerID, worldID, buildingID)
		if err != nil {
			fail("could not calculate temple demolition", err)
			return
		}
		if _, err = tx.Exec(ctx, `UPDATE player_world_records SET kharis_amount=$3,kharis_calc_tick=current_world_tick() WHERE player_id=$1 AND world_id=$2`, playerID, worldID, demolition.KharisAfter); err != nil {
			fail("could not apply temple wrath", err)
			return
		}
	}

	// A temple's workers stand at this one building and would cascade away
	// with it (mig 202) — lift them first so they are counted. Every other
	// workplace is lifted after the delete, against the capacity left behind.
	var lifted []int
	if godID != nil {
		trows, err := tx.Query(ctx, `DELETE FROM settlement_placement WHERE building_id = $1 RETURNING gubbe_ordinal`, buildingID)
		if err != nil {
			fail("could not lift temple workers", err)
			return
		}
		for trows.Next() {
			var o int
			if err := trows.Scan(&o); err == nil {
				lifted = append(lifted, o)
			}
		}
		trows.Close()
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
		   AND hex_q IS NOT DISTINCT FROM $3 AND hex_r IS NOT DISTINCT FROM $4
		   AND god_id IS NOT DISTINCT FROM $5`,
		settlementID, bType, hexQ, hexR, godID)
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

	rest, err := economy.LiftPlacementsAfterDemolish(ctx, tx, settlementID, buildingID, bType, hexQ, hexR)
	if err != nil {
		fail("could not lift citizens", err)
		return
	}
	lifted = append(lifted, rest...)

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
	var wrathEvent *events.Event
	var wrathNoticeID string
	var wrathBody any
	if demolition != nil {
		var name string
		var q, r int
		if err = tx.QueryRow(ctx, `SELECT s.name,p.map_q,p.map_r FROM settlements s JOIN provinces p ON p.id=s.province_id WHERE s.id=$1`, settlementID).Scan(&name, &q, &r); err != nil {
			fail("could not describe temple wrath", err)
			return
		}
		wrathBody = struct {
			*kharis.TempleDemolition
			BuildingID   uuid.UUID `json:"building_id"`
			SettlementID uuid.UUID `json:"settlement_id"`
			ProvinceID   uuid.UUID `json:"province_id"`
			Name         string    `json:"name"`
			Q            int       `json:"q"`
			R            int       `json:"r"`
		}{demolition, buildingID, settlementID, provinceID, name, q, r}
		if h.eventStore != nil {
			wrathEvent, err = h.eventStore.AppendTx(ctx, tx, settlementID, events.StreamReligion, "TempleDemolitionWrath", wrathBody, worldID, nil)
			if err != nil {
				fail("could not record temple wrath", err)
				return
			}
		}
		if err = tx.QueryRow(ctx, `INSERT INTO notifications(world_id,player_id,kind,level,body_json,world_tick) VALUES($1,$2,'TempleDemolitionWrath',2,$3,current_world_tick()) RETURNING id`, worldID, playerID, wrathBody).Scan(&wrathNoticeID); err != nil {
			fail("could not notify temple wrath", err)
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

	if wrathEvent != nil {
		h.eventStore.RecordCommitted(ctx, wrathEvent)
	}
	if demolition != nil && h.hub != nil {
		h.hub.DeliverCommittedNotification(ctx, worldID, playerID, wrathNoticeID, "TempleDemolitionWrath", 2, wrathBody)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"demolition_kharis": demolition,
		"demolished":        true, "building_type": bType, "level": level,
		"hex_q": hexQ, "hex_r": hexR, "citizens_lifted": len(lifted),
	})
}
