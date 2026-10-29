package handlers

// Build Road (megaron_plan_vagar.md V2): POST /worlds/{worldID}/units/{unitID}/build-road,
// GET /worlds/{worldID}/roads (finished edges the caller knows of),
// GET /worlds/{worldID}/road-sites (the caller's own unfinished roads).

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/messenger"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
)

// BuildRoad orders an army to build ONE road edge from→to (neighbouring hexes).
// The stone leaves the nearest own city now and travels as a caravan to the
// "from" hex; a Runner carries the order to the army; building begins when both
// are at "from". Body: {"from_q","from_r","to_q","to_r"}.
func (h *UnitHandler) BuildRoad(w http.ResponseWriter, r *http.Request) {
	playerID, ok := auth.PlayerIDFromContext(r.Context())
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
	var req struct {
		FromQ *int `json:"from_q"`
		FromR *int `json:"from_r"`
		ToQ   *int `json:"to_q"`
		ToR   *int `json:"to_r"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.FromQ == nil || req.FromR == nil || req.ToQ == nil || req.ToR == nil {
		writeError(w, http.StatusBadRequest, "must provide from_q, from_r, to_q and to_r (the two neighbouring hexes the road joins)")
		return
	}
	ctx := r.Context()

	u, err := h.store.Get(ctx, unitID)
	if err != nil || u.OwnerID != playerID || u.WorldID != worldID {
		writeError(w, http.StatusNotFound, "unit not found")
		return
	}
	if unit.CategoryOf(u.Type) != unit.CategoryLand || u.Type == unit.TypeNomadicHost {
		writeError(w, http.StatusUnprocessableEntity, "only a land army can build a road")
		return
	}
	if u.Status != unit.StatusGarrison && u.Status != unit.StatusPositioned {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf(
			"this army is %s and cannot build a road now — it must stand in a city or in the field (a marching army can be recalled first)", string(u.Status)))
		return
	}

	// Where the army stands: its city's hex, or its field hex.
	var unitPos province.MapPosition
	if u.SettlementID != nil {
		if err := h.pool.QueryRow(ctx,
			`SELECT p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id WHERE s.id = $1`,
			*u.SettlementID).Scan(&unitPos.Q, &unitPos.R); err != nil {
			writeError(w, http.StatusInternalServerError, "could not locate the army")
			return
		}
	} else if u.Q != nil && u.R != nil {
		unitPos = province.MapPosition{Q: *u.Q, R: *u.R}
	} else {
		writeError(w, http.StatusUnprocessableEntity, "the army has no position")
		return
	}

	origin, found := h.resolveOrderOrigin(w, ctx, worldID, playerID, unitPos)
	if !found {
		return
	}
	if origin.settlementID == nil {
		writeError(w, http.StatusUnprocessableEntity, "a road needs one of your cities to supply the stone")
		return
	}

	order := combat.BuildRoadOrder{
		WorldID: worldID, PlayerID: playerID, UnitID: unitID,
		FromQ: *req.FromQ, FromR: *req.FromR, ToQ: *req.ToQ, ToR: *req.ToR,
	}
	payload := messenger.OrderDeliveryPayload{
		WorldID: worldID, PlayerID: playerID, UnitID: unitID, Verb: "build_road", BuildRoad: &order,
	}

	// The army stands with its commanding city: the order is heard at once (no
	// Runner) — the stone still travels as a caravan.
	if origin.dist == 0 {
		tx, err := h.pool.Begin(ctx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not begin transaction")
			return
		}
		defer tx.Rollback(ctx)
		siteID, stone, err := combat.PlaceRoadSite(ctx, tx, h.scheduler, h.clk, order, *origin.settlementID)
		if err != nil {
			var rej *combat.OrderReject
			if errors.As(err, &rej) {
				writeError(w, rej.Status, rej.Reason)
				return
			}
			writeError(w, http.StatusInternalServerError, "could not place the road order")
			return
		}
		if err := tx.Commit(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, "could not commit the road order")
			return
		}
		order.SiteID = siteID
		if err := combat.ExecuteBuildRoad(ctx, h.pool, h.scheduler, h.eventStore, h.clk, order); err != nil {
			var rej *combat.OrderReject
			if errors.As(err, &rej) {
				writeError(w, rej.Status, rej.Reason+" — the stone is already on its way and the road waits (see road-sites)")
				return
			}
			writeError(w, http.StatusInternalServerError, "the army could not start for the road; the stone caravan is on its way")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status": "road_ordered", "verb": "build_road", "unit_id": unitID, "site_id": siteID, "stone": stone,
			"from_q": order.FromQ, "from_r": order.FromR, "to_q": order.ToQ, "to_r": order.ToR,
		})
		return
	}

	extra := map[string]any{"from_q": order.FromQ, "from_r": order.FromR, "to_q": order.ToQ, "to_r": order.ToR}
	h.sendOrderCourierTx(w, ctx, payload,
		fmt.Sprintf("Runner — build a road from (%d,%d) to (%d,%d).", order.FromQ, order.FromR, order.ToQ, order.ToR),
		origin, unitPos, extra,
		func(tx pgx.Tx, p *messenger.OrderDeliveryPayload) error {
			siteID, stone, err := combat.PlaceRoadSite(ctx, tx, h.scheduler, h.clk, *p.BuildRoad, *origin.settlementID)
			if err != nil {
				return err
			}
			p.BuildRoad.SiteID = siteID
			extra["site_id"] = siteID
			extra["stone"] = stone
			return nil
		})
}

// Roads handles GET /worlds/{worldID}/roads — the finished road edges the caller
// has seen or remembers at BOTH ends (F5: a road never reveals a hex the player
// knows nothing of). Anonymous callers get none.
func (h *UnitHandler) Roads(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	type edge struct {
		Q1        int `json:"q1"`
		R1        int `json:"r1"`
		Q2        int `json:"q2"`
		R2        int `json:"r2"`
		BuiltTick int `json:"built_tick"`
	}
	out := []edge{}
	playerID, authenticated := auth.PlayerIDFromContext(r.Context())
	if !authenticated {
		writeJSONETag(w, r, map[string]any{"edges": out})
		return
	}
	ctx := r.Context()
	rows, err := h.pool.Query(ctx, `SELECT q1, r1, q2, r2, built_tick FROM road_edges WHERE world_id = $1 ORDER BY built_tick, q1, r1, q2, r2`, worldID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load roads")
		return
	}
	var all []edge
	for rows.Next() {
		var e edge
		if err := rows.Scan(&e.Q1, &e.R1, &e.Q2, &e.R2, &e.BuiltTick); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "could not read roads")
			return
		}
		all = append(all, e)
	}
	rows.Close()
	if len(all) == 0 {
		writeJSONETag(w, r, map[string]any{"edges": out})
		return
	}
	g, err := province.LoadTileGraph(ctx, h.pool, worldID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load terrain")
		return
	}
	eyes := loadLiveEyes(ctx, h.pool, worldID, playerID, h.clk.Now())
	remembered := loadRememberedTiles(ctx, h.pool, worldID, playerID)
	known := func(q, r int) bool {
		terrain, ok := g[[2]int{q, r}]
		if !ok {
			return false
		}
		return knownToPlayer(eyes, remembered, province.MapPosition{Q: q, R: r}, terrain)
	}
	for _, e := range all {
		if known(e.Q1, e.R1) && known(e.Q2, e.R2) {
			out = append(out, e)
		}
	}
	writeJSONETag(w, r, map[string]any{"edges": out})
}

// RoadSites handles GET /worlds/{worldID}/road-sites — the caller's own unfinished
// roads, so a road stuck waiting for its stone or its army is visible, not silent.
func (h *UnitHandler) RoadSites(w http.ResponseWriter, r *http.Request) {
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	rows, err := h.pool.Query(r.Context(),
		`SELECT rs.id, rs.unit_id, rs.from_q, rs.from_r, rs.to_q, rs.to_r, rs.stone, rs.state,
		        rs.stone_arrived, rs.army_arrived, rs.build_due_tick, rs.created_tick,
		        COALESCE(t.status, ''), COALESCE(t.due_tick, 0)
		   FROM road_sites rs
		   LEFT JOIN transports t ON t.id = rs.stone_transport_id
		  WHERE rs.world_id = $1 AND rs.owner_id = $2 AND rs.state IN ('waiting', 'building')
		  ORDER BY rs.created_tick, rs.id`, worldID, playerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load road sites")
		return
	}
	defer rows.Close()
	type site struct {
		ID           uuid.UUID  `json:"id"`
		UnitID       *uuid.UUID `json:"unit_id"`
		FromQ        int        `json:"from_q"`
		FromR        int        `json:"from_r"`
		ToQ          int        `json:"to_q"`
		ToR          int        `json:"to_r"`
		Stone        float64    `json:"stone"`
		State        string     `json:"state"`
		StoneArrived bool       `json:"stone_arrived"`
		ArmyArrived  bool       `json:"army_arrived"`
		BuildDueTick *int       `json:"build_due_tick,omitempty"`
		CreatedTick  int        `json:"created_tick"`
		StoneStatus  string     `json:"stone_caravan_status,omitempty"`
		StoneDueTick int        `json:"stone_caravan_due_tick,omitempty"`
		// Waiting names what the road is still waiting for — never silence.
		Waiting []string `json:"waiting_for"`
	}
	out := []site{}
	for rows.Next() {
		var s site
		if err := rows.Scan(&s.ID, &s.UnitID, &s.FromQ, &s.FromR, &s.ToQ, &s.ToR, &s.Stone, &s.State,
			&s.StoneArrived, &s.ArmyArrived, &s.BuildDueTick, &s.CreatedTick, &s.StoneStatus, &s.StoneDueTick); err != nil {
			writeError(w, http.StatusInternalServerError, "could not read road sites")
			return
		}
		s.Waiting = []string{}
		if s.State == "waiting" {
			if !s.StoneArrived {
				if s.StoneStatus == "intercepted" || s.StoneStatus == "lost" || s.StoneStatus == "foundered" {
					s.Waiting = append(s.Waiting, "stone (the caravan was "+s.StoneStatus+" — the road will not be finished; give a new order)")
				} else {
					s.Waiting = append(s.Waiting, "stone caravan")
				}
			}
			if !s.ArmyArrived {
				s.Waiting = append(s.Waiting, "army")
			}
		}
		out = append(out, s)
	}
	writeJSONETag(w, r, map[string]any{"sites": out})
}
