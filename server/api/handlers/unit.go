package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/unit"
)

// UnitHandler handles HTTP requests for the unit endpoints (C3).
type UnitHandler struct {
	pool       *pgxpool.Pool
	scheduler  *events.Scheduler
	eventStore *events.Store
	clk        clock.Clock
	store      *unit.Store
}

// NewUnitHandler creates a UnitHandler.
func NewUnitHandler(pool *pgxpool.Pool, scheduler *events.Scheduler, eventStore *events.Store, clk clock.Clock) *UnitHandler {
	return &UnitHandler{
		pool:       pool,
		scheduler:  scheduler,
		eventStore: eventStore,
		clk:        clk,
		store:      unit.NewStore(pool),
	}
}

// March handles POST /worlds/{worldID}/units/{unitID}/march
//
// Moves a discrete unit from its current settlement to a target hex.
// Validations (C3 plan):
//   - Caller must own the unit
//   - Unit must be in status='garrison'
//   - Land units: size must be exactly 100 (forming units cannot march)
//   - Naval: deployable (status='garrison')
//   - Stance (if provided) is persisted on the unit for C5; no behaviour enforced here
func (h *UnitHandler) March(w http.ResponseWriter, r *http.Request) {
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
		TargetQ int    `json:"target_q"`
		TargetR int    `json:"target_r"`
		Stance  string `json:"stance"` // optional; fortify|storm|sentry — persisted for C5
		Intent  string `json:"intent"` // optional; "" = plain march, "colonize" = found a colony on arrival, "land" = land troops from a ship (R1)
		Name    string `json:"name"`   // optional colony name (intent=colonize, or intent=land + cargo_intent=colonize)
		Mode    string `json:"mode"`   // optional; "" = sack (default) | "annex" — conquest choice on arrival (Del 2b)
		// CargoIntent (megaron_plan_skeppsuppdrag_landsatt.md R1) is only
		// meaningful with intent=land: "" (just land) or "colonize" (found a
		// colony on arrival, no further order needed).
		CargoIntent string `json:"cargo_intent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	ctx := r.Context()

	// FOW march rule (Fas 0): knowledge is checked at dispatch, in the API
	// layer that owns the fog helpers — tier-1 live ∪ tier-2 remembered.
	targetKnown := func(fctx context.Context, target province.MapPosition, terrain string) bool {
		eyes := loadLiveEyes(fctx, h.pool, worldID, playerID, h.clk.Now())
		if province.AnyEyeSees(eyes, target, terrain) {
			return true
		}
		return loadRememberedTiles(fctx, h.pool, worldID, playerID)[[2]int{target.Q, target.R}]
	}

	// Order latency (temenos_orderlopare_plan.md Fas 2): a unit outside any
	// settlement cannot be commanded instantly — the order travels by runner
	// from the nearest own city (Timothy 2026-07-16) and executes only on
	// delivery. A garrisoned unit is distance 0 (the order originates in the
	// city it sits in) and executes immediately via StartMarch below. Marching
	// units keep using recall/redirect (already courier-borne; Fas 3 unifies).
	if u, uErr := h.store.Get(ctx, unitID); uErr == nil &&
		u.OwnerID == playerID && u.WorldID == worldID &&
		u.Status == unit.StatusPositioned && u.SettlementID == nil &&
		u.Q != nil && u.R != nil {
		// R3 (megaron_plan_skeppsuppdrag_landsatt.md): a ship standing off at
		// sea — the only way a naval unit reaches this "field-positioned, no
		// settlement" branch — takes no orders; its mission already carries it
		// home on its own. Checked here, before a Runner is even dispatched,
		// not inside combat.StartMarch (see RequireShipInPort's own doc
		// comment for why).
		if rej := combat.RequireShipInPort(ctx, h.pool, worldID, playerID,
			unit.CategoryOf(u.Type), u.Status, unit.LoadDisplayName(ctx, h.pool, u.ID),
			nil, nil, nil); rej != nil {
			writeError(w, rej.Status, rej.Reason)
			return
		}
		order := combat.MarchOrder{
			WorldID: worldID, PlayerID: playerID, UnitID: unitID,
			TargetQ: req.TargetQ, TargetR: req.TargetR,
			Stance: req.Stance, Intent: req.Intent, Name: req.Name, Mode: req.Mode,
			CargoIntent: req.CargoIntent,
		}
		h.dispatchMarchCourier(w, ctx, order, province.MapPosition{Q: *u.Q, R: *u.R}, targetKnown)
		return
	}

	// Validate+execute core shared with the order-courier delivery path
	// (temenos_orderlopare_plan.md Fas 1) — internal/combat.StartMarch.
	res, err := combat.StartMarch(ctx, h.pool, h.scheduler, h.eventStore, h.clk, combat.MarchOrder{
		WorldID:     worldID,
		PlayerID:    playerID,
		UnitID:      unitID,
		TargetQ:     req.TargetQ,
		TargetR:     req.TargetR,
		Stance:      req.Stance,
		Intent:      req.Intent,
		Name:        req.Name,
		Mode:        req.Mode,
		CargoIntent: req.CargoIntent,
	}, targetKnown)
	if err != nil {
		var rej *combat.OrderReject
		if errors.As(err, &rej) {
			writeError(w, rej.Status, rej.Reason)
			return
		}
		writeError(w, http.StatusInternalServerError, "march failed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"unit_id":    res.UnitID,
		"departs_at": res.DepartsAt,
		"arrives_at": res.ArrivesAt,
		// K4 tick-contract: timing expressed in world ticks (source of truth under
		// the tick substrate), plus a derived UTC convenience. ArrivesAt is already
		// the tick-derived instant (now + travelTicks × TickSeconds), so
		// arrives_at_utc is that same instant normalised to UTC.
		"arrival_tick":   res.ArrivalTick,
		"duration_ticks": res.DurationTicks,
		"arrives_at_utc": res.ArrivesAt.UTC(),
		"origin_q":       res.OriginQ,
		"origin_r":       res.OriginR,
		"target_q":       res.TargetQ,
		"target_r":       res.TargetR,
		// The colonist purse the column left with (mig 107) and what the mother
		// city could not cover. Reported at dispatch because that is the last
		// moment the Wanax can call the expedition back and fund it properly.
		"carried_silver":  res.CarriedSilver,
		"purse_shortfall": res.PurseShortfall,
	})
}

// dispatchMarchCourier sends a march order to a field unit by physical Runner
// (Timothy 2026-07-26; the Greek day-runner, hemerodromos, was the source but
// the player-facing name is Runner); DB identifier stays
// kind='order' (temenos_orderlopare_plan.md Fas 2). Cheap pre-flights only
// (target exists, FOW) — the delivery handler re-validates authoritatively
// against the unit's state when the courier arrives; an order that can no
// longer be carried out fails with an OrderFailed notice, never silently.
// No pending-order guard: latest delivered wins (Timothy 2026-07-16).
func (h *UnitHandler) dispatchMarchCourier(w http.ResponseWriter, ctx context.Context, order combat.MarchOrder, unitPos province.MapPosition, targetKnown combat.TargetKnownFunc) {
	// Target hex must exist (map bounds are public knowledge).
	var destTerrain string
	if err := h.pool.QueryRow(ctx,
		`SELECT terrain FROM map_tiles WHERE world_id = $1 AND q = $2 AND r = $3`,
		order.WorldID, order.TargetQ, order.TargetR,
	).Scan(&destTerrain); err != nil {
		writeError(w, http.StatusNotFound, "target hex not found")
		return
	}
	// FOW march rule — checked at dispatch (the player's knowledge NOW is what
	// authorises the order). Exempt: explore and colonize-in-place (own hex).
	colonizeInPlace := order.Intent == "colonize" && order.TargetQ == unitPos.Q && order.TargetR == unitPos.R
	if !colonizeInPlace && order.Intent != "explore" &&
		!targetKnown(ctx, province.MapPosition{Q: order.TargetQ, R: order.TargetR}, destTerrain) {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("none of your men have ever seen (%d,%d) — a march cannot be ordered into unknown land; send a scout first (march with intent \"explore\")",
				order.TargetQ, order.TargetR))
		return
	}

	origin, ok := h.resolveOrderOrigin(w, ctx, order.WorldID, order.PlayerID, unitPos)
	if !ok {
		return
	}

	// Distance 0 (the commanding presence stands with the unit): execute now.
	if origin.dist == 0 {
		res, err := combat.StartMarch(ctx, h.pool, h.scheduler, h.eventStore, h.clk, order, targetKnown)
		if err != nil {
			var rej *combat.OrderReject
			if errors.As(err, &rej) {
				writeError(w, rej.Status, rej.Reason)
				return
			}
			writeError(w, http.StatusInternalServerError, "march failed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"unit_id": res.UnitID, "departs_at": res.DepartsAt, "arrives_at": res.ArrivesAt,
			"arrival_tick": res.ArrivalTick, "duration_ticks": res.DurationTicks,
			"arrives_at_utc": res.ArrivesAt.UTC(),
			"origin_q":       res.OriginQ, "origin_r": res.OriginR,
			"target_q": res.TargetQ, "target_r": res.TargetR,
		})
		return
	}

	h.sendOrderCourier(w, ctx, messenger.OrderDeliveryPayload{
		WorldID: order.WorldID, PlayerID: order.PlayerID, UnitID: order.UnitID,
		Verb: "march", March: &order,
	}, fmt.Sprintf("Runner — march order to (%d,%d).", order.TargetQ, order.TargetR),
		origin, unitPos, map[string]any{"target_q": order.TargetQ, "target_r": order.TargetR})
}

// hostCurrentPos resolves the founder host's CURRENT position: its stored hex,
// or — while the host is marching — its interpolated position along its route.
// A marching unit's stored (q,r) is its ORIGIN hex (updated only on arrival),
// so reading it raw made orders and messengers depart from where the host LAST
// stood still (Timothys fynd 2026-07-17). Mirrors loadLiveEyes' marching-unit
// eye: FindPath over the unit's own category + interpolatedEyePos.
func hostCurrentPos(ctx context.Context, pool *pgxpool.Pool, now time.Time, worldID, playerID uuid.UUID) (uuid.UUID, province.MapPosition, bool) {
	var hostID uuid.UUID
	var status, category string
	var q, r int
	var targetQ, targetR *int
	var departsAt, arrivesAt *time.Time
	var departTick, arriveTick *int
	var marchRouteRaw []byte
	if err := pool.QueryRow(ctx,
		`SELECT fp.host_unit_id, u.status, u.category, u.q, u.r, u.target_q, u.target_r, u.departs_at, u.arrives_at,
		        u.depart_tick, u.arrive_tick, u.march_route
		 FROM founder_phase fp JOIN units u ON u.id = fp.host_unit_id
		 WHERE fp.world_id = $1 AND fp.owner_id = $2 AND fp.active
		   AND u.q IS NOT NULL AND u.r IS NOT NULL`,
		worldID, playerID,
	).Scan(&hostID, &status, &category, &q, &r, &targetQ, &targetR, &departsAt, &arrivesAt,
		&departTick, &arriveTick, &marchRouteRaw); err != nil {
		return uuid.Nil, province.MapPosition{}, false
	}
	pos := province.MapPosition{Q: q, R: r}
	if status == "marching" && targetQ != nil && targetR != nil && departsAt != nil && arrivesAt != nil {
		// movement 2a, R6.e: read via the saved route when it applies
		// (invariant 2); otherwise the old FindPath + interpolatedEyePos,
		// unchanged.
		if route, ok := combat.LoadActiveRoute(marchRouteRaw, status, departTick, arriveTick); ok {
			if anchor, aErr := tick.LoadAnchor(ctx, pool, worldID); aErr == nil {
				if p, rErr := combat.RoutePositionAt(route, anchor.MilliAt(now)); rErr == nil {
					return hostID, p, true
				}
			}
		}
		path, _, ok, err := province.FindPath(ctx, pool, worldID, pos,
			province.MapPosition{Q: *targetQ, R: *targetR}, category)
		if err == nil && ok && len(path) > 0 {
			pos = interpolatedEyePos(now, *departsAt, *arrivesAt, path)
		}
		// FindPath failure: best-effort fallback to the stored origin hex.
	}
	return hostID, pos, true
}

// orderOrigin is the resolved dispatching city — the NEAREST own settlement to
// the unit (Timothy 2026-07-16) — or, in founder phase, the wandering host.
type orderOrigin struct {
	settlementID *uuid.UUID
	unitID       *uuid.UUID
	q, r, dist   int
}

// resolveOrderOrigin finds the order's origin; on failure it writes the 422
// and returns ok=false.
func (h *UnitHandler) resolveOrderOrigin(w http.ResponseWriter, ctx context.Context, worldID, playerID uuid.UUID, unitPos province.MapPosition) (orderOrigin, bool) {
	var o orderOrigin
	found := false
	rows, err := h.pool.Query(ctx,
		`SELECT s.id, p.map_q, p.map_r FROM settlements s
		 JOIN provinces p ON p.id = s.province_id
		 WHERE s.world_id = $1 AND s.owner_id = $2 AND s.state = 'active'`,
		worldID, playerID)
	if err == nil {
		for rows.Next() {
			var sid uuid.UUID
			var q, r int
			if rows.Scan(&sid, &q, &r) != nil {
				continue
			}
			d := province.HexDistance(province.MapPosition{Q: q, R: r}, unitPos)
			if !found || d < o.dist {
				found = true
				s := sid
				o = orderOrigin{settlementID: &s, q: q, r: r, dist: d}
			}
		}
		rows.Close()
	}
	if !found {
		hostID, pos, ok := hostCurrentPos(ctx, h.pool, h.clk.Now(), worldID, playerID)
		if !ok {
			writeError(w, http.StatusUnprocessableEntity,
				"you have no city (and no wandering host) to dispatch a Runner from")
			return o, false
		}
		hid := hostID
		o = orderOrigin{unitID: &hid, q: pos.Q, r: pos.R, dist: province.HexDistance(pos, unitPos)}
	}
	return o, true
}

// sendOrderCourier inserts the kind='order' runner and schedules its
// ScheduledOrderDelivery, answering 202 order_dispatched with the courier ETA.
func (h *UnitHandler) sendOrderCourier(w http.ResponseWriter, ctx context.Context, payload messenger.OrderDeliveryPayload, msgText string, origin orderOrigin, unitPos province.MapPosition, extra map[string]any) {
	now := h.clk.Now()
	var currentTick int
	_ = h.pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	// megaron_plan_budet_liftar.md R1: a runner order whose route needs sea runs
	// to the PLAYER's own port and waits for a real carrier instead of crossing
	// the abstract boat instantly.
	courierArrivesAt, dueTick, passage, passageSinceTick, rErr := messenger.ResolveDeparture(
		ctx, h.pool, payload.WorldID, payload.PlayerID,
		province.MapPosition{Q: origin.q, R: origin.r}, unitPos, now, currentTick)
	if errors.Is(rErr, messenger.ErrNoPort) {
		writeError(w, http.StatusUnprocessableEntity, rErr.Error())
		return
	}
	if rErr != nil {
		writeError(w, http.StatusInternalServerError, "could not resolve courier route")
		return
	}
	var passageSinceTickArg *int
	if passage != nil {
		passageSinceTickArg = &passageSinceTick
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not begin transaction")
		return
	}
	defer tx.Rollback(ctx)

	messengerID, err := insertOrderMessenger(ctx, tx, payload, msgText, origin,
		unitPos.Q, unitPos.R, courierArrivesAt, passage, passageSinceTickArg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not dispatch order runner")
		return
	}
	payload.MessengerID = messengerID
	if passage == nil {
		if err := h.scheduler.EnqueueTickTx(ctx, tx, payload.WorldID, events.ScheduledOrderDelivery, payload, dueTick); err != nil {
			writeError(w, http.StatusInternalServerError, "could not schedule order delivery")
			return
		}
	}
	// passage != nil: the runner just runs to its port and waits — no
	// ScheduledOrderDelivery yet. messenger.PassageScanHandler schedules it
	// (rebuilding the payload from order_payload) once it boards a carrier or
	// takes the reserve (R2/R5). order_payload already carries the up-to-date
	// payload written just above.
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "could not commit order dispatch")
		return
	}

	resp := map[string]any{
		"status":             "order_dispatched",
		"verb":               payload.Verb,
		"unit_id":            payload.UnitID,
		"messenger_id":       messengerID,
		"courier_arrives_at": courierArrivesAt,
		"courier_due_tick":   dueTick,
	}
	if passage != nil {
		resp["passage_status"] = "awaiting_passage"
	}
	for k, v := range extra {
		resp[k] = v
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(resp)
}

// mustJSON marshals the order envelope for the messenger row; the payload is
// built from typed structs and cannot fail to encode.
func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// insertOrderMessenger is the ONE INSERT shape for a kind='order' messenger
// row — extracted out of sendOrderCourier (megaron_plan_hamta_hem.md R2) so
// its two callers, sendOrderCourier's own route-resolved dispatch and the
// pickup handler's bud (which already knows its route — it is created
// directly at the ship's own port, never routed via messenger.
// ResolveDeparture, since the whole point is to ride THIS specific ship) —
// agree on what each column means. ⚠️ messengers.hex_q/hex_r/origin_q/origin_r
// had drifted into meaning different things per INSERT path before (the 3b-3
// lesson); one function now owns the mapping. hex_q/hex_r is always the
// courier's own current (starting) position; dest_q/dest_r is the unit it is
// walking to deliver the order to.
func insertOrderMessenger(
	ctx context.Context, tx pgx.Tx,
	payload messenger.OrderDeliveryPayload, msgText string, origin orderOrigin,
	destQ, destR int, arrivesAt time.Time,
	passage *messenger.PassagePort, passageSinceTick *int,
) (uuid.UUID, error) {
	var originQ, originR *int
	if origin.unitID != nil {
		originQ, originR = &origin.q, &origin.r
	}
	var passagePortID *uuid.UUID
	if passage != nil {
		passagePortID = &passage.SettlementID
	}
	var messengerID uuid.UUID
	err := tx.QueryRow(ctx,
		`INSERT INTO messengers
		     (world_id, sender_id, origin_id, origin_unit_id, origin_q, origin_r, destination_id, message_text, status, kind, hex_q, hex_r, dest_q, dest_r, arrives_at, order_payload, passage_status, passage_port_id, passage_since_tick)
		 VALUES ($1,$2,$3,$4,$5,$6,NULL,$7,'outbound','order',$8,$9,$10,$11,$12,$13,$14,$15,$16)
		 RETURNING id`,
		payload.WorldID, payload.PlayerID, origin.settlementID, origin.unitID, originQ, originR,
		msgText, origin.q, origin.r, destQ, destR, arrivesAt, mustJSON(payload),
		messenger.PassageStatusArg(passage), passagePortID, passageSinceTick,
	).Scan(&messengerID)
	return messengerID, err
}

// Recall handles POST /worlds/{worldID}/units/{unitID}/recall
//
// Sends a physical recall/redirect order to a marching unit via the order
// envelope (temenos_orderlopare_plan.md — recall/redirect→kuvert-unifiering).
// Body (optional): {"target_q":int,"target_r":int} — omitted = recall (turn home to
// the hex the unit departed from); both given = redirect (new course). The order
// travels as a visible runner; the unit keeps marching on its original course
// until the courier physically catches up with it — command is never instant.
func (h *UnitHandler) Recall(w http.ResponseWriter, r *http.Request) {
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
		TargetQ *int `json:"target_q"`
		TargetR *int `json:"target_r"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if (req.TargetQ == nil) != (req.TargetR == nil) {
		writeError(w, http.StatusBadRequest, "must provide both target_q and target_r, or neither (omit both to recall home)")
		return
	}
	mode := "recall"
	if req.TargetQ != nil {
		mode = "redirect"
	}

	ctx := r.Context()

	// Ownership + existence collapsed into one 404: don't reveal a unit's
	// existence/status to a non-owner.
	u, err := h.store.Get(ctx, unitID)
	if err != nil || u.OwnerID != playerID || u.WorldID != worldID {
		writeError(w, http.StatusNotFound, "unit not found")
		return
	}
	if u.Status != unit.StatusMarching {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("unit is not marching (status: %s) — nothing to recall", string(u.Status)))
		return
	}
	if u.Q == nil || u.R == nil || u.TargetQ == nil || u.TargetR == nil || u.DepartsAt == nil || u.ArrivesAt == nil {
		writeError(w, http.StatusInternalServerError, "marching unit missing position data")
		return
	}

	// R3 (megaron_plan_skeppsuppdrag_landsatt.md): a marching ship is always at
	// sea, on a mission with a built-in return leg — it cannot be recalled or
	// redirected. Land units are unaffected (RequireShipInPort is a no-op for
	// them).
	if rej := combat.RequireShipInPort(ctx, h.pool, worldID, playerID,
		unit.CategoryOf(u.Type), u.Status, unit.LoadDisplayName(ctx, h.pool, u.ID),
		u.TargetQ, u.TargetR, u.ArrivesAt); rej != nil {
		writeError(w, rej.Status, rej.Reason)
		return
	}

	// Guard: an earlier recall/redirect is already in flight for this unit.
	// Checks the order-envelope's ScheduledOrderDelivery queue (verb recall|redirect)
	// now that dispatch goes through sendOrderCourier instead of the frozen
	// ScheduledMarchRecall path — that old event type is no longer written by
	// fresh dispatches, so checking it here would silently stop firing.
	var pendingMessengerID uuid.UUID
	if err := h.pool.QueryRow(ctx,
		`SELECT (payload->>'messenger_id')::uuid
		 FROM scheduled_events
		 WHERE event_type = $1 AND (payload->>'unit_id')::uuid = $2
		   AND payload->>'verb' = ANY(ARRAY['recall','redirect'])
		   AND processed_at IS NULL AND failed_at IS NULL`,
		string(events.ScheduledOrderDelivery), unitID,
	).Scan(&pendingMessengerID); err == nil {
		var eta time.Time
		_ = h.pool.QueryRow(ctx, `SELECT arrives_at FROM messengers WHERE id=$1`, pendingMessengerID).Scan(&eta)
		writeError(w, http.StatusConflict,
			fmt.Sprintf("a recall/redirect order is already on its way to this unit (ETA %s)", eta.Local().Format(time.RFC3339)))
		return
	}

	origin := province.MapPosition{Q: *u.Q, R: *u.R}
	target := province.MapPosition{Q: *u.TargetQ, R: *u.TargetR}
	category := string(unit.CategoryOf(u.Type))

	var newTargetQ, newTargetR int
	if mode == "redirect" {
		newTargetQ, newTargetR = *req.TargetQ, *req.TargetR

		var destTerrain string
		if err := h.pool.QueryRow(ctx,
			`SELECT terrain FROM map_tiles WHERE world_id = $1 AND q = $2 AND r = $3`,
			worldID, newTargetQ, newTargetR,
		).Scan(&destTerrain); err != nil {
			writeError(w, http.StatusNotFound, "target hex not found")
			return
		}
		// FOW march rule — a redirect is a march order like any other; the new
		// destination must be seen or remembered (temenos_orderlopare_plan.md
		// Fas 0). Checked before the terrain responses below to avoid leaking
		// what stands on an unseen hex.
		fowTarget := province.MapPosition{Q: newTargetQ, R: newTargetR}
		eyes := loadLiveEyes(ctx, h.pool, worldID, playerID, h.clk.Now())
		if !province.AnyEyeSees(eyes, fowTarget, destTerrain) &&
			!loadRememberedTiles(ctx, h.pool, worldID, playerID)[[2]int{newTargetQ, newTargetR}] {
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("none of your men have ever seen (%d,%d) — a march cannot be redirected into unknown land",
					newTargetQ, newTargetR))
			return
		}
		if destTerrain == "mountain_limestone" || destTerrain == "mountain_red" {
			writeError(w, http.StatusUnprocessableEntity, "mountain terrain is impassable")
			return
		}
		// River is water too, a wall for land units (megaron_floden_plan.md, Timothy 2026-07-29).
		isSea := destTerrain == "coastal_sea" || destTerrain == "deep_sea" || destTerrain == "river"
		if unit.CategoryOf(u.Type) == unit.CategoryLand && isSea {
			writeError(w, http.StatusUnprocessableEntity, "land units cannot enter sea terrain")
			return
		}
	}

	// movement 2a, R6.c: read via the saved route when it applies to this
	// march (invariant 2); otherwise interpolate along the path it already
	// proved traversable at dispatch, exactly as before — never a
	// straight-line guess either way.
	var currentPos province.MapPosition
	posOK := false
	if activeRoute, ok := combat.LoadActiveRoute(u.MarchRoute, string(u.Status), u.DepartTick, u.ArriveTick); ok {
		if anchor, aErr := tick.LoadAnchor(ctx, h.pool, worldID); aErr == nil {
			if p, rErr := combat.RoutePositionAt(activeRoute, anchor.MilliAt(h.clk.Now())); rErr == nil {
				currentPos, posOK = p, true
			}
		}
	}
	if !posOK {
		var ipErr error
		currentPos, posOK, ipErr = province.InterpolatePosition(ctx, h.pool, worldID, origin, target, category,
			*u.DepartsAt, *u.ArrivesAt, h.clk.Now())
		if ipErr != nil {
			writeError(w, http.StatusInternalServerError, "could not resolve unit's current position")
			return
		}
		if !posOK {
			currentPos = origin
		}
	}

	if mode == "redirect" {
		newTarget := province.MapPosition{Q: newTargetQ, R: newTargetR}
		if _, _, pathOK, pathErr := province.FindPath(ctx, h.pool, worldID, currentPos, newTarget, category); pathErr != nil {
			writeError(w, http.StatusInternalServerError, "pathfinding error")
			return
		} else if !pathOK {
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("no passable route from the unit's current position to (%d,%d)", newTargetQ, newTargetR))
			return
		}
	}

	recallOrder := &combat.RecallOrder{WorldID: worldID, UnitID: unitID, Mode: mode}
	if mode == "redirect" {
		recallOrder.NewTargetQ = &newTargetQ
		recallOrder.NewTargetR = &newTargetR
	}

	// Wanax travels WITH the nomadic host — an order to it is an order to
	// Wanax's own body, so it needs no Runner (unit.CommandedInPerson; the
	// one exception to the messenger pillar, buggrapport 70c1bfb3
	// 2026-09-04). Every other unit type still falls through to the courier
	// path below, unchanged. Apply directly via the SAME execution core a
	// courier's delivery runs (messenger.OrderDeliveryHandler.Handle's
	// recall/redirect case) — no duplicated logic, just no runner to wait for.
	if unit.CommandedInPerson(u.Type) {
		res, err := combat.ExecuteRecall(ctx, h.pool, h.scheduler, h.eventStore, h.clk, *recallOrder)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not apply order")
			return
		}
		if res == nil {
			// The unit stopped marching between the status check above and
			// here (e.g. it arrived in the instant this request was being
			// handled) — a genuine miss, not a game-rule rejection.
			writeError(w, http.StatusConflict,
				"the unit was no longer marching by the time the order was applied — check its current status")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":     "order_applied",
			"verb":       mode,
			"unit_id":    res.UnitID,
			"q":          res.FromQ,
			"r":          res.FromR,
			"target_q":   res.NewTargetQ,
			"target_r":   res.NewTargetR,
			"arrives_at": res.ArrivesAt,
		})
		return
	}

	// Resolve who dispatches the order: the nearest own active settlement to
	// the unit's CURRENT position (beslut 4, temenos_orderlopare_plan.md) — the
	// same resolveOrderOrigin march/stance already use, not the old bespoke
	// "settlement at march origin → capital → host" chain. currentPos (not the
	// march-departure hex) is the right distance anchor: a marching unit is
	// never "in" a city, so this never short-circuits to instant delivery —
	// sendOrderCourier always dispatches a real runner here.
	courierOrigin, ok := h.resolveOrderOrigin(w, ctx, worldID, playerID, currentPos)
	if !ok {
		return
	}

	// Aim the Runner at an honest interception point along the unit's still-
	// in-progress march, not at the position it happened to occupy this
	// instant (temenos_orderlopare_plan.md interception fix, 2026-07-30): a
	// courier always takes time to travel, and the unit keeps marching while
	// it does, so a snapshot aim is stale the moment the runner sets out —
	// the exact silent tap this replaces (internal/messenger.ExecuteRecall
	// only checks the unit is still "marching" when the courier arrives; it
	// never re-validates against where the runner was actually headed). When
	// no honest intercept exists — courierOrigin is too far, or too little of
	// the march remains, for any physically real runner to catch this unit —
	// fail now, visibly, instead of queuing a courier already certain to
	// arrive too late.
	interceptPos, interceptOK, err := messenger.InterceptCourierTarget(ctx, h.pool, worldID,
		province.MapPosition{Q: courierOrigin.q, R: courierOrigin.r}, origin, target, category,
		*u.DepartsAt, *u.ArrivesAt, h.clk.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not resolve runner interception")
		return
	}
	if !interceptOK {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("no Runner can catch this unit before it completes its march (arrives %s) — wait for it to arrive, then issue a fresh order",
				u.ArrivesAt.Local().Format(time.RFC3339)))
		return
	}

	msgText := "Runner — recall order, return home."
	if mode == "redirect" {
		msgText = fmt.Sprintf("Runner — redirect order, new course to (%d,%d).", newTargetQ, newTargetR)
	}

	extra := map[string]any{"mode": mode}
	if mode == "redirect" {
		extra["target_q"] = newTargetQ
		extra["target_r"] = newTargetR
	}

	h.sendOrderCourier(w, ctx, messenger.OrderDeliveryPayload{
		WorldID: worldID, PlayerID: playerID, UnitID: unitID,
		Verb: mode, Recall: recallOrder,
	}, msgText, courierOrigin, interceptPos, extra)
}

// Pickup handles POST /worlds/{worldID}/units/{unitID}/pickup — R1/R2,
// megaron_plan_hamta_hem.md, slice 2b. unitID is the fetched unit (a land
// unit standing positioned in the field, no settlement); the body names one
// of the caller's own ships. Body: {"ship_id": "...", "wait_ticks": N?}.
//
// The ship sails to the shore nearest the fetched unit's own hex (the unit's
// own hex, if it already stands on one reachable by sea from the ship's
// port). If the unit does not already stand there, a runner is dispatched
// automatically to march it there — riding the very same ship across the
// sea, exactly like an arranged passage. See combat.StartMarch's "pickup"
// intent, combat.UnitArrivalHandler.pickupArrived/boardPickupUnit/
// HandlePickupTimeout/pickupReturned for what happens at either end of the
// voyage.
func (h *UnitHandler) Pickup(w http.ResponseWriter, r *http.Request) {
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
	fetchUnitID, err := uuid.Parse(chi.URLParam(r, "unitID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid unit ID")
		return
	}

	var req struct {
		ShipID    string `json:"ship_id"`
		WaitTicks *int   `json:"wait_ticks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	shipID, err := uuid.Parse(req.ShipID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid ship_id")
		return
	}

	ctx := r.Context()

	// Load and validate the fetched unit (R1's first condition).
	fetched, err := h.store.Get(ctx, fetchUnitID)
	if err != nil {
		writeError(w, http.StatusNotFound, "unit not found")
		return
	}
	if fetched.OwnerID != playerID || fetched.WorldID != worldID {
		writeError(w, http.StatusForbidden, "not your unit")
		return
	}
	fetchedName := unit.LoadDisplayName(ctx, h.pool, fetchUnitID)
	if unit.CategoryOf(fetched.Type) != unit.CategoryLand || !unit.CanEmbark(fetched.Type) {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("%s cannot be carried by ship", fetchedName))
		return
	}
	switch fetched.Status {
	case unit.StatusMarching:
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("%s is on the march — fetch it once it has halted", fetchedName))
		return
	case unit.StatusGarrison:
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("%s is in a city — send the ship there and load it", fetchedName))
		return
	case unit.StatusPositioned:
		// ok — a field-positioned unit is exactly what pickup is for.
	default:
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("%s cannot be fetched right now (status: %s)", fetchedName, string(fetched.Status)))
		return
	}
	if fetched.SettlementID != nil || fetched.Q == nil || fetched.R == nil {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("%s has no known position in the field", fetchedName))
		return
	}
	unitQ, unitR := *fetched.Q, *fetched.R

	// Load and validate the ship (R1's second/third/fourth conditions — same
	// "docked, garrison, no cargo" shape as ArrangePassage's own ship check).
	ship, err := h.store.Get(ctx, shipID)
	if err != nil {
		writeError(w, http.StatusNotFound, "ship not found")
		return
	}
	if ship.OwnerID != playerID || ship.WorldID != worldID {
		writeError(w, http.StatusForbidden, "not your ship")
		return
	}
	if unit.CategoryOf(ship.Type) != unit.CategoryNaval {
		writeError(w, http.StatusUnprocessableEntity, "unit is not a naval vessel")
		return
	}
	if ship.Status != unit.StatusGarrison || ship.SettlementID == nil {
		writeError(w, http.StatusUnprocessableEntity,
			"the ship must be docked in its own port, free of any mission, to be sent to fetch a unit")
		return
	}
	if ship.CargoUnitID != nil {
		writeError(w, http.StatusUnprocessableEntity, "the ship is already carrying a unit — unload it first")
		return
	}

	waitTicks := combat.PickupWaitDefaultTicks
	if req.WaitTicks != nil {
		waitTicks = *req.WaitTicks
		if waitTicks < 1 || waitTicks > combat.PickupWaitMaxTicks {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("wait_ticks must be between 1 and %d", combat.PickupWaitMaxTicks))
			return
		}
	}

	shoreQ, shoreR, err := resolvePickupShore(ctx, h.pool, worldID, *ship.SettlementID, unitQ, unitR)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("no open shore near %s that this ship can reach by sea", fetchedName))
		return
	}
	runnerNeeded := shoreQ != unitQ || shoreR != unitR
	if runnerNeeded && ship.Type == unit.TypeWarGalley {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("a war galley cannot carry a runner — %s must already stand on the shore, or send a galley or merchantman", fetchedName))
		return
	}

	var shipPortQ, shipPortR int
	if err := h.pool.QueryRow(ctx,
		`SELECT p.map_q, p.map_r FROM provinces p JOIN settlements s ON s.province_id = p.id WHERE s.id = $1`,
		*ship.SettlementID,
	).Scan(&shipPortQ, &shipPortR); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load the ship's port")
		return
	}

	// R2.1: the bud, if the unit does not already stand on the chosen shore.
	// Created BEFORE the ship's own march (R2's own ordering) — a worst-case
	// StartMarch failure right after leaves an orphaned, but self-healing,
	// waiting runner: the existing PassageStalled dispatch (3b-4) surfaces it
	// to the Wanax like any other stranded wait, rather than a silent leak.
	estimateTicks := 0
	var messengerID *uuid.UUID
	res, err := combat.StartMarch(ctx, h.pool, h.scheduler, h.eventStore, h.clk, combat.MarchOrder{
		WorldID: worldID, PlayerID: playerID, UnitID: shipID,
		TargetQ: shoreQ, TargetR: shoreR,
		Intent:          "pickup",
		PickupUnitID:    &fetchUnitID,
		PickupWaitTicks: waitTicks,
	}, nil) // no FOW check: the shore is server-computed, not player-typed
	if err != nil {
		var rej *combat.OrderReject
		if errors.As(err, &rej) {
			writeError(w, rej.Status, rej.Reason)
			return
		}
		writeError(w, http.StatusInternalServerError, "pickup failed")
		return
	}

	// R2.3: board the runner now, at dispatch — same reasoning as Arrange
	// (R0, megaron_plan_hamta_hem.md).
	// The runner is created only AFTER the ship's march is accepted: created
	// first, a StartMarch rejection would strand a runner in port carrying an
	// order the player was just told had failed (review 2026-09-28).
	if runnerNeeded {
		msgID, mErr := h.dispatchPickupRunner(ctx, worldID, playerID, fetchUnitID,
			*ship.SettlementID, shipPortQ, shipPortR, shoreQ, shoreR, unitQ, unitR)
		if mErr != nil {
			// The ship has already sailed: it will wait its ticks and come home
			// empty. Log it rather than answer 500 for a march that did start.
			slog.Error("pickup: could not dispatch the runner after the ship sailed", "ship", shipID, "err", mErr)
		} else {
			messengerID = &msgID
		}
		if t, ok := estimateTicksToShore(ctx, h.pool, worldID, shoreQ, shoreR, unitQ, unitR, fetched.Type, fetched.Crew); ok {
			estimateTicks = t
		}
	}

	// R2.2.
	if runnerNeeded {
		if err := messenger.BoardDispatchedShipRunner(ctx, h.pool, h.scheduler, h.clk, worldID, shipID); err != nil {
			slog.Error("pickup: board runner at dispatch failed", "ship", shipID, "err", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"unit_id":                  shipID,
		"pickup_unit_id":           fetchUnitID,
		"shore_q":                  shoreQ,
		"shore_r":                  shoreR,
		"arrival_tick":             res.ArrivalTick,
		"wait_ticks":               waitTicks,
		"messenger_id":             messengerID,
		"estimated_ticks_to_shore": estimateTicks,
	})
}

// dispatchPickupRunner is R2.1 (megaron_plan_hamta_hem.md): creates the order
// bud that will ride the pickup ship inland to march the fetched unit to the
// shore. Unlike sendOrderCourier's own dispatch, this bud's route is already
// known — it is created directly at the ship's own port and is ALWAYS
// awaiting passage there (the whole point is to ride this specific ship), so
// it goes straight through insertOrderMessenger rather than messenger.
// ResolveDeparture's route search. arrives_at/passage_since_tick = now/the
// current tick, same as ResolveReturnDeparture's own sea branch for a
// messenger that already stands at its port with no landward leg to run.
func (h *UnitHandler) dispatchPickupRunner(
	ctx context.Context, worldID, playerID, fetchUnitID, shipSettlementID uuid.UUID,
	portQ, portR, shoreQ, shoreR, unitQ, unitR int,
) (uuid.UUID, error) {
	now := h.clk.Now()
	var currentTick int
	_ = h.pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)

	march := combat.MarchOrder{
		WorldID: worldID, PlayerID: playerID, UnitID: fetchUnitID,
		TargetQ: shoreQ, TargetR: shoreR,
	}
	payload := messenger.OrderDeliveryPayload{
		WorldID: worldID, PlayerID: playerID, UnitID: fetchUnitID,
		Verb: "march", March: &march,
	}
	msgText := fmt.Sprintf("Runner — order to march to (%d,%d) for pickup.", shoreQ, shoreR)
	origin := orderOrigin{settlementID: &shipSettlementID, q: portQ, r: portR}
	passage := &messenger.PassagePort{SettlementID: shipSettlementID, Q: portQ, R: portR}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	messengerID, err := insertOrderMessenger(ctx, tx, payload, msgText, origin,
		unitQ, unitR, now, passage, &currentTick)
	if err != nil {
		return uuid.Nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return messengerID, nil
}

// estimateTicksToShore is R2.4's estimated_ticks_to_shore (megaron_plan_
// hamta_hem.md): the bud's own landward leg from the shore to the fetched
// unit (CategoryCourierLand) plus the fetched unit's own march from itself
// to the shore (its own category), rounded UP to a whole tick — so the Wanax
// can choose wait_ticks with a real number in hand, not a guess. ok=false
// when either leg has no route; best-effort — the caller then reports 0
// rather than refusing the whole pickup over a preview number.
func estimateTicksToShore(ctx context.Context, pool *pgxpool.Pool, worldID uuid.UUID, shoreQ, shoreR, unitQ, unitR int, unitType unit.Type, unitCrew int) (int, bool) {
	shore := province.MapPosition{Q: shoreQ, R: shoreR}
	unitPos := province.MapPosition{Q: unitQ, R: unitR}
	_, courierHours, courierOK, cErr := province.FindPath(ctx, pool, worldID, shore, unitPos, province.CategoryCourierLand)
	if cErr != nil || !courierOK {
		return 0, false
	}
	_, marchHours, marchOK, mErr := province.FindPath(ctx, pool, worldID, unitPos, shore, string(unit.CategoryOf(unitType)))
	if mErr != nil || !marchOK {
		return 0, false
	}
	marchHours *= combat.TravelFactor(unitType, unitCrew, false)
	ticks := int(math.Ceil(courierHours + marchHours))
	if ticks < 1 {
		ticks = 1
	}
	return ticks, true
}

// Load handles POST /worlds/{worldID}/units/{shipID}/load
//
// Embarks a land unit onto a naval unit (the ship). Rules (C6 plan):
//   - Caller must own both units.
//   - Both units must be in the same settlement (garrison).
//   - Ship must be naval and have no current cargo (cargo_unit_id IS NULL).
//   - Land unit must be status='garrison' (no size gate — a battle-worn
//     cohort of size < 100 is a real unit that can still embark).
//   - Origin must be a coastal settlement (adjacent to sea) or have a harbour.
//
// Outcome: ship.cargo_unit_id = land_unit_id; land unit status → 'embarked'.
// Emits ShipLoaded.
func (h *UnitHandler) Load(w http.ResponseWriter, r *http.Request) {
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
	shipID, err := uuid.Parse(chi.URLParam(r, "unitID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid ship ID")
		return
	}

	var req struct {
		UnitID string `json:"unit_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	cargoID, err := uuid.Parse(req.UnitID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid unit_id")
		return
	}

	ctx := r.Context()

	// Load ship.
	ship, err := h.store.Get(ctx, shipID)
	if err != nil {
		writeError(w, http.StatusNotFound, "ship not found")
		return
	}
	if ship.OwnerID != playerID || ship.WorldID != worldID {
		writeError(w, http.StatusForbidden, "not your ship")
		return
	}
	if unit.CategoryOf(ship.Type) != unit.CategoryNaval {
		writeError(w, http.StatusUnprocessableEntity, "unit is not a naval vessel")
		return
	}
	if ship.Status != unit.StatusGarrison {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("ship must be garrisoned to load (status: %s)", string(ship.Status)))
		return
	}
	if ship.CargoUnitID != nil {
		writeError(w, http.StatusConflict, "ship already carries a unit — unload first")
		return
	}
	if ship.SettlementID == nil {
		writeError(w, http.StatusUnprocessableEntity, "ship has no settlement; cannot load")
		return
	}

	// Load cargo unit.
	cargo, err := h.store.Get(ctx, cargoID)
	if err != nil {
		writeError(w, http.StatusNotFound, "cargo unit not found")
		return
	}
	if cargo.OwnerID != playerID || cargo.WorldID != worldID {
		writeError(w, http.StatusForbidden, "not your unit")
		return
	}
	if unit.CategoryOf(cargo.Type) != unit.CategoryLand {
		writeError(w, http.StatusUnprocessableEntity, "only land units can be loaded onto ships")
		return
	}
	if !unit.CanEmbark(cargo.Type) {
		writeError(w, http.StatusUnprocessableEntity, "a people on the move cannot go aboard")
		return
	}
	if cargo.Status != unit.StatusGarrison {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("cargo unit must be garrisoned to embark (status: %s)", string(cargo.Status)))
		return
	}
	// No size gate: 'garrison' status already excludes forming/training units, and
	// a battle-worn cohort (size < 100 after losses) is a real unit that can embark.
	// Both must be in the same settlement.
	if cargo.SettlementID == nil || *cargo.SettlementID != *ship.SettlementID {
		writeError(w, http.StatusUnprocessableEntity, "ship and cargo unit must be in the same settlement")
		return
	}

	// Embark-gating: settlement must be coastal or have a harbour.
	var settlementCoastal bool
	if err := h.pool.QueryRow(ctx,
		`SELECT COALESCE(p.coastal, false)
		 FROM settlements s
		 JOIN provinces p ON p.id = s.province_id
		 WHERE s.id = $1`,
		*ship.SettlementID,
	).Scan(&settlementCoastal); err != nil {
		writeError(w, http.StatusInternalServerError, "could not check settlement coastal")
		return
	}
	if !settlementCoastal {
		var hasHarbour bool
		_ = h.pool.QueryRow(ctx,
			`SELECT EXISTS(
			   SELECT 1 FROM buildings b
			   JOIN settlements s ON s.id = b.settlement_id
			   WHERE s.id = $1 AND b.building_type = 'harbour'
			 )`,
			*ship.SettlementID,
		).Scan(&hasHarbour)
		if !hasHarbour {
			writeError(w, http.StatusUnprocessableEntity, "units can only embark at coastal settlements or harbours")
			return
		}
	}

	// Atomic: lock both rows, apply changes.
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not begin transaction")
		return
	}
	defer tx.Rollback(ctx)

	// Re-read ship FOR UPDATE (idempotency guard).
	var shipStatus string
	var shipCargo *uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT status, cargo_unit_id FROM units WHERE id = $1 FOR UPDATE`, shipID,
	).Scan(&shipStatus, &shipCargo); err != nil {
		writeError(w, http.StatusNotFound, "ship not found in transaction")
		return
	}
	if unit.Status(shipStatus) != unit.StatusGarrison {
		writeError(w, http.StatusConflict, "ship status changed; load not applied")
		return
	}
	if shipCargo != nil {
		writeError(w, http.StatusConflict, "ship already has cargo (concurrent request)")
		return
	}

	// Re-read cargo FOR UPDATE.
	var cargoStatus string
	if err := tx.QueryRow(ctx,
		`SELECT status FROM units WHERE id = $1 FOR UPDATE`, cargoID,
	).Scan(&cargoStatus); err != nil {
		writeError(w, http.StatusNotFound, "cargo unit not found in transaction")
		return
	}
	if unit.Status(cargoStatus) != unit.StatusGarrison {
		writeError(w, http.StatusConflict, "cargo unit status changed; load not applied")
		return
	}

	// Set cargo_unit_id on ship.
	if _, err := tx.Exec(ctx,
		`UPDATE units SET cargo_unit_id = $2, updated_at = now() WHERE id = $1`,
		shipID, cargoID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update ship")
		return
	}

	// Mark cargo unit as embarked.
	if _, err := tx.Exec(ctx,
		`UPDATE units SET status = 'embarked', updated_at = now() WHERE id = $1`,
		cargoID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not embark unit")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "could not commit load")
		return
	}

	// Get ship position for event payload.
	var posQ, posR int
	if ship.Q != nil {
		posQ = *ship.Q
	}
	if ship.R != nil {
		posR = *ship.R
	}

	_, _ = h.eventStore.Append(ctx, shipID, events.StreamType(unit.StreamUnit), unit.EventShipLoaded,
		unit.ShipLoadedPayload{
			ShipUnitID:  shipID,
			CargoUnitID: cargoID,
			Q:           posQ,
			R:           posR,
		}, worldID, nil,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ship_id":       shipID,
		"cargo_unit_id": cargoID,
	})
}

// Unload handles POST /worlds/{worldID}/units/{shipID}/unload
//
// Disembarks the cargo land unit from a naval unit. Rules (C6 plan; the P7
// soak fix's field-landing fall (b) was RETIRED by
// megaron_plan_skeppsuppdrag_landsatt.md R2 — see below):
//   - Caller must own the ship.
//   - Ship must have a cargo unit (cargo_unit_id non-null).
//   - Ship must be garrisoned at a coastal (adjacent to sea) settlement or
//     harbour — cargo joins that settlement's garrison. A ship standing to sea
//     (status='positioned') can no longer unload there at all: 422.
//
// P7 (2026-07-19) added a field-landing fall (b) — a ship field-positioned at
// a sea hex could drop its cargo ashore on unclaimed land next to it,
// immediately, with no courier. That is exactly the "order that skips the
// messenger pillar" the order-runner plan closes: landing troops is now a
// mission a ship is given IN PORT (march intent=land — see
// api/handlers/unit.go's March/combat.StartMarch and
// megaron_plan_skeppsuppdrag_landsatt.md R1), with a built-in return leg,
// never an instant remote command to a ship already out at sea. Amphibious
// ASSAULT against an enemy settlement is unaffected (march intent=assault,
// unit_arrival.go resolveAmphibiousAssault).
//
// Outcome: cargo unit status → 'garrison'; ship.cargo_unit_id = NULL.
// Emits ShipUnloaded.
func (h *UnitHandler) Unload(w http.ResponseWriter, r *http.Request) {
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
	shipID, err := uuid.Parse(chi.URLParam(r, "unitID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid ship ID")
		return
	}

	ctx := r.Context()

	ship, err := h.store.Get(ctx, shipID)
	if err != nil {
		writeError(w, http.StatusNotFound, "ship not found")
		return
	}
	if ship.OwnerID != playerID || ship.WorldID != worldID {
		writeError(w, http.StatusForbidden, "not your ship")
		return
	}
	if unit.CategoryOf(ship.Type) != unit.CategoryNaval {
		writeError(w, http.StatusUnprocessableEntity, "unit is not a naval vessel")
		return
	}
	// R2 (megaron_plan_skeppsuppdrag_landsatt.md): fall (b) — landing on bare,
	// unclaimed shore from a ship standing to sea — is retired. A ship at sea
	// takes no orders (R3); putting troops ashore away from a friendly port is
	// now a MISSION given in port (march intent=land, R1), never an instant
	// command to a ship already out on the water.
	if ship.Status != unit.StatusGarrison {
		writeError(w, http.StatusUnprocessableEntity,
			"a ship at sea cannot be ordered to unload — give it a \"land\" mission from port instead")
		return
	}
	if ship.CargoUnitID == nil {
		writeError(w, http.StatusUnprocessableEntity, "ship carries no unit")
		return
	}

	// destQ/destR is where the cargo unit ends up. Resolved up front so both
	// the pre-flight and the in-tx re-check below agree on the same target.
	var destQ, destR int
	destSettlementID := ship.SettlementID

	// Ship is docked at a settlement — disembark gating: must be coastal or harbour.
	var disembarkCoastal bool
	if err := h.pool.QueryRow(ctx,
		`SELECT COALESCE(p.coastal, false)
		 FROM settlements s
		 JOIN provinces p ON p.id = s.province_id
		 WHERE s.id = $1`,
		*ship.SettlementID,
	).Scan(&disembarkCoastal); err != nil {
		writeError(w, http.StatusInternalServerError, "could not check settlement coastal")
		return
	}
	if !disembarkCoastal {
		var hasHarbour bool
		_ = h.pool.QueryRow(ctx,
			`SELECT EXISTS(
			   SELECT 1 FROM buildings b
			   JOIN settlements s ON s.id = b.settlement_id
			   WHERE s.id = $1 AND b.building_type = 'harbour'
			 )`,
			*ship.SettlementID,
		).Scan(&hasHarbour)
		if !hasHarbour {
			writeError(w, http.StatusUnprocessableEntity, "units can only disembark at coastal settlements or harbours")
			return
		}
	}
	if ship.Q != nil {
		destQ = *ship.Q
	}
	if ship.R != nil {
		destR = *ship.R
	}

	cargoID := *ship.CargoUnitID

	// Load cargo to get its current position for the event.
	cargo, err := h.store.Get(ctx, cargoID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load cargo unit")
		return
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not begin transaction")
		return
	}
	defer tx.Rollback(ctx)

	// Re-read ship FOR UPDATE (idempotency guard).
	var shipStatus string
	var shipCargo *uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT status, cargo_unit_id FROM units WHERE id = $1 FOR UPDATE`, shipID,
	).Scan(&shipStatus, &shipCargo); err != nil {
		writeError(w, http.StatusNotFound, "ship not found in transaction")
		return
	}
	if unit.Status(shipStatus) != unit.StatusGarrison && unit.Status(shipStatus) != unit.StatusPositioned {
		writeError(w, http.StatusConflict, "ship status changed; unload not applied")
		return
	}
	if shipCargo == nil {
		writeError(w, http.StatusConflict, "ship has no cargo (concurrent request)")
		return
	}

	// Clear cargo from ship.
	if _, err := tx.Exec(ctx,
		`UPDATE units SET cargo_unit_id = NULL, updated_at = now() WHERE id = $1`,
		shipID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update ship")
		return
	}

	// Place the cargo unit at its resolved destination: garrisoned at the
	// ship's settlement (a), or field-positioned on the bare land hex (b) —
	// mirrors arriveGarrison's own "positioned when settlementID is nil" rule
	// (unit_arrival.go), so the same `march --intent colonize` path that
	// handles any other field-positioned unit picks this one up too.
	newStatus := "garrison"
	if destSettlementID == nil {
		newStatus = "positioned"
	}
	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   status        = $2,
		   settlement_id = $3,
		   q             = $4,
		   r             = $5,
		   updated_at    = now()
		 WHERE id = $1`,
		cargoID, newStatus, destSettlementID, destQ, destR,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not disembark unit")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "could not commit unload")
		return
	}

	_ = cargo // silence unused warning; used above for loading
	_, _ = h.eventStore.Append(ctx, shipID, events.StreamType(unit.StreamUnit), unit.EventShipUnloaded,
		unit.ShipUnloadedPayload{
			ShipUnitID:  shipID,
			CargoUnitID: cargoID,
			Q:           destQ,
			R:           destR,
		}, worldID, nil,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ship_id":       shipID,
		"cargo_unit_id": cargoID,
		"q":             destQ,
		"r":             destR,
		"status":        newStatus,
	})
}

// SetStance handles POST /worlds/{worldID}/units/{unitID}/stance
//
// Allows a garrison or positioned unit to adopt a combat stance without moving (C5).
// Body: {"stance":"fortify"|"storm"|"sentry"|"none"}
//
// Rules:
//   - Caller must own the unit.
//   - Unit must be status='garrison', 'positioned' or 'marching' (not forming, etc.).
//     A marching unit gets it by a Runner that catches up (stanceToMarchingUnit).
//   - "none" clears the stance.
//   - "sentry": sets sentry_q/sentry_r to the unit's current hex.
//
// The march endpoint already blocks fortify units from marching (see March handler).
func (h *UnitHandler) SetStance(w http.ResponseWriter, r *http.Request) {
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
		Stance string `json:"stance"` // fortify|storm|sentry|none
		// ReactionForeign optionally picks the avsiktslagret foreign-relation verb
		// when Stance == "sentry" (default: intercept, today's unchanged
		// behaviour). See unit.ReactionPolicy — all four verbs are behaviourally
		// wired for unit-vs-unit (KR3 §7); escort/alert remain stubs for caravans.
		ReactionForeign string `json:"reaction_foreign,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	ctx := r.Context()
	order := combat.StanceOrder{
		WorldID: worldID, PlayerID: playerID, UnitID: unitID,
		Stance: req.Stance, ReactionForeign: req.ReactionForeign,
	}

	// A MARCHING unit (megaron_styrande_beslut §11): the stance goes by a
	// Runner that has to catch up with it — see stanceToMarchingUnit.
	if u, uErr := h.store.Get(ctx, unitID); uErr == nil &&
		u.OwnerID == playerID && u.WorldID == worldID && u.Status == unit.StatusMarching {
		h.stanceToMarchingUnit(w, ctx, u, order)
		return
	}

	// Order latency (temenos_orderlopare_plan.md Fas 3): a stance order to a
	// field unit travels by runner from the nearest own city and applies
	// only on delivery. Garrisoned units are distance 0 — the order originates
	// in the city the unit sits in — and apply immediately below.
	if u, uErr := h.store.Get(ctx, unitID); uErr == nil &&
		u.OwnerID == playerID && u.WorldID == worldID &&
		u.Status == unit.StatusPositioned && u.SettlementID == nil &&
		u.Q != nil && u.R != nil {
		// Cheap pre-flights so an obviously bad order fails NOW, not by notice.
		switch req.Stance {
		case "fortify", "storm", "sentry", "none":
			// valid
		default:
			writeError(w, http.StatusBadRequest, `invalid stance: must be "fortify", "storm", "sentry", or "none"`)
			return
		}
		if unit.CategoryOf(u.Type) == unit.CategoryNaval {
			writeError(w, http.StatusUnprocessableEntity, "naval units cannot take a stance")
			return
		}
		unitPos := province.MapPosition{Q: *u.Q, R: *u.R}
		origin, originOK := h.resolveOrderOrigin(w, ctx, worldID, playerID, unitPos)
		if !originOK {
			return
		}
		if origin.dist > 0 {
			h.sendOrderCourier(w, ctx, messenger.OrderDeliveryPayload{
				WorldID: worldID, PlayerID: playerID, UnitID: unitID,
				Verb: "stance", Stance: &order,
			}, fmt.Sprintf("Runner — stance order (%s).", req.Stance),
				origin, unitPos, map[string]any{"stance": req.Stance})
			return
		}
	}

	// Validate+execute core shared with the order-courier delivery path
	// (temenos_orderlopare_plan.md Fas 3) — internal/combat.SetStance.
	res, err := combat.SetStance(ctx, h.pool, h.eventStore, order)
	if err != nil {
		var rej *combat.OrderReject
		if errors.As(err, &rej) {
			writeError(w, rej.Status, rej.Reason)
			return
		}
		writeError(w, http.StatusInternalServerError, "stance change failed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"unit_id":          res.UnitID,
		"stance":           res.Stance,
		"sentry_q":         res.SentryQ,
		"sentry_r":         res.SentryR,
		"reaction_foreign": res.ReactionForeign,
	})
}

// stanceToMarchingUnit dispatches a stance order to a unit on the march
// (megaron_styrande_beslut §11, Timothy 2026-09-25: "yes, by Runner, but then
// the Runner must catch up with it"). It reuses redirect's catch-up exactly —
// no second pursuit model: the Runner leaves the nearest own city to the
// unit's CURRENT position and is aimed at messenger.InterceptCourierTarget,
// the earliest hex on the unit's path it can reach no later than the unit.
//
// Where redirect must refuse when no intercept exists (a new course is
// meaningless once the march is over), a stance is not: the Runner is then
// aimed at the march's destination and applies the stance where the unit has
// stopped. Delivery runs the "stance_pursuit" verb (combat.SetStanceInPursuit).
//
// No in-flight guard: stance orders have always been latest-delivered-wins
// (order_delivery.go), and the recall/redirect 409 guard is scoped to those
// verbs — a stance Runner and a redirect Runner may pursue the same unit.
func (h *UnitHandler) stanceToMarchingUnit(w http.ResponseWriter, ctx context.Context, u *unit.Unit, order combat.StanceOrder) {
	switch order.Stance {
	case "fortify", "storm", "sentry", "none":
	default:
		writeError(w, http.StatusBadRequest, `invalid stance: must be "fortify", "storm", "sentry", or "none"`)
		return
	}
	if order.ReactionForeign != "" && !unit.ValidReactionVerb(order.ReactionForeign) {
		writeError(w, http.StatusBadRequest,
			`invalid reaction_foreign: must be "intercept", "escort", "ignore", or "alert"`)
		return
	}
	if unit.CategoryOf(u.Type) == unit.CategoryNaval {
		writeError(w, http.StatusUnprocessableEntity, "naval units cannot take a stance")
		return
	}
	if u.Q == nil || u.R == nil || u.TargetQ == nil || u.TargetR == nil || u.DepartsAt == nil || u.ArrivesAt == nil {
		writeError(w, http.StatusInternalServerError, "marching unit missing position data")
		return
	}

	// Wanax rides with the nomadic host — no Runner (unit.CommandedInPerson,
	// same exception Recall makes). Applied through the delivery core itself.
	if unit.CommandedInPerson(u.Type) {
		res, err := combat.SetStanceInPursuit(ctx, h.pool, h.eventStore, order)
		if err != nil {
			var rej *combat.OrderReject
			if errors.As(err, &rej) {
				writeError(w, rej.Status, rej.Reason)
				return
			}
			writeError(w, http.StatusInternalServerError, "stance change failed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"unit_id":          res.UnitID,
			"stance":           res.Stance,
			"sentry_q":         res.SentryQ,
			"sentry_r":         res.SentryR,
			"reaction_foreign": res.ReactionForeign,
		})
		return
	}

	origin := province.MapPosition{Q: *u.Q, R: *u.R}
	target := province.MapPosition{Q: *u.TargetQ, R: *u.TargetR}
	category := string(unit.CategoryOf(u.Type))
	now := h.clk.Now()

	// movement 2a, R6.d: when a gällande saved route applies (invariant 2),
	// both "where is it now" and the Runner's interception aim come from the
	// core — no path search at all. Otherwise the old re-walk + interception,
	// unchanged.
	activeRoute, hasRoute := combat.LoadActiveRoute(u.MarchRoute, string(u.Status), u.DepartTick, u.ArriveTick)
	var anchor tick.Anchor
	if hasRoute {
		var aErr error
		anchor, aErr = tick.LoadAnchor(ctx, h.pool, u.WorldID)
		if aErr != nil {
			hasRoute = false
		}
	}

	var currentPos province.MapPosition
	posOK := false
	if hasRoute {
		if p, rErr := combat.RoutePositionAt(activeRoute, anchor.MilliAt(now)); rErr == nil {
			currentPos, posOK = p, true
		}
	}
	if !posOK {
		var ipErr error
		currentPos, posOK, ipErr = province.InterpolatePosition(ctx, h.pool, u.WorldID, origin, target, category,
			*u.DepartsAt, *u.ArrivesAt, now)
		if ipErr != nil {
			writeError(w, http.StatusInternalServerError, "could not resolve unit's current position")
			return
		}
		if !posOK {
			currentPos = origin
		}
	}
	courierOrigin, ok := h.resolveOrderOrigin(w, ctx, u.WorldID, order.PlayerID, currentPos)
	if !ok {
		return
	}
	var aim province.MapPosition
	var intercepted bool
	var err error
	if hasRoute {
		aim, intercepted, err = messenger.InterceptCourierTargetRoute(ctx, h.pool, u.WorldID,
			province.MapPosition{Q: courierOrigin.q, R: courierOrigin.r}, activeRoute, anchor, now)
	} else {
		aim, intercepted, err = messenger.InterceptCourierTarget(ctx, h.pool, u.WorldID,
			province.MapPosition{Q: courierOrigin.q, R: courierOrigin.r}, origin, target, category,
			*u.DepartsAt, *u.ArrivesAt, now)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not resolve runner interception")
		return
	}
	catchUp := "on_the_march"
	if !intercepted {
		aim = target
		catchUp = "at_destination"
	}

	h.sendOrderCourier(w, ctx, messenger.OrderDeliveryPayload{
		WorldID: u.WorldID, PlayerID: order.PlayerID, UnitID: u.ID,
		Verb: "stance_pursuit", Stance: &order,
	}, fmt.Sprintf("Runner — stance order (%s), catching up with a marching unit.", order.Stance),
		courierOrigin, aim, map[string]any{
			"stance":          order.Stance,
			"catch_up":        catchUp,
			"intercept_q":     aim.Q,
			"intercept_r":     aim.R,
			"unit_arrives_at": *u.ArrivesAt,
		})
}

// Reinforce handles POST /worlds/{worldID}/units/{unitID}/reinforce
//
// Manskaps-underhåll (megaron_plan_rekryteringsmodell.md, Timothy
// 2026-08-19): marks a decimated land cohort as awaiting refill from its
// origin city's population growth. This call has NO immediate effect on
// size — it only flips units.reinforcing to true. The tick worker
// (kharis/tick.go applyReinforcement) trickles men in over time, at most
// economy.ReinforceMenPerTick per day, capped by however much the city
// actually grew that day — diverted from that growth, never from the city's
// standing population, so the origin city can never shrink from a refill.
//
// Only fires while the cohort stays garrisoned in its own origin city
// (SettlementID == OriginSettlementID, Status == garrison) — march it out
// and the tick worker stops the trickle and holds the size it had reached.
// There is no distance/courier order here (unlike March/SetStance for a
// field unit): the cohort is, by definition, sitting in a city its Wanax
// already governs, so the order applies at once — command latency only
// exists for units away from a settlement.
func (h *UnitHandler) Reinforce(w http.ResponseWriter, r *http.Request) {
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

	u, err := h.store.Get(r.Context(), unitID)
	if err != nil {
		writeError(w, http.StatusNotFound, "unit not found")
		return
	}
	if u.OwnerID != playerID || u.WorldID != worldID {
		writeError(w, http.StatusForbidden, "not your unit")
		return
	}
	if unit.CategoryOf(u.Type) != unit.CategoryLand {
		writeError(w, http.StatusUnprocessableEntity, "only land cohorts can be reinforced")
		return
	}
	if u.Status != unit.StatusGarrison || u.SettlementID == nil ||
		u.OriginSettlementID == nil || *u.SettlementID != *u.OriginSettlementID {
		writeError(w, http.StatusUnprocessableEntity,
			"reinforce only works in the cohort's home city — march it back to its origin settlement first")
		return
	}
	if u.Size >= economy.MaxUnitSize {
		writeError(w, http.StatusUnprocessableEntity, "already at full strength")
		return
	}

	if _, err := h.pool.Exec(r.Context(),
		`UPDATE units SET reinforcing = true, updated_at = now() WHERE id = $1`, unitID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not start reinforcement")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"unit_id":     unitID,
		"reinforcing": true,
		"size":        u.Size,
	})
}

// SetStandingOrders handles POST /worlds/{worldID}/units/{unitID}/standing-orders
//
// KR3 §5: change a unit's mid-battle rout threshold. Body:
// {"retreat_at_loss": 0.5} and/or {"hold_to_last_man": true} — either field
// may be omitted to leave it unchanged.
//
// The unit must currently be a participant in an active battle —
// standing_orders lives on battle_participants (migration 114), not on the
// unit; there is no pre-battle preset surface (out of scope, megaron_todo.md
// KR3 loose end (c) names only this mid-battle change as the remaining gap).
//
// Order latency, same rule as SetStance: a field unit's commander only hears
// the new order when a Runner physically arrives from the nearest own
// settlement (command is never instant). A besieged garrison unit is
// distance 0 — the Wanax is already in that city — and applies immediately.
func (h *UnitHandler) SetStandingOrders(w http.ResponseWriter, r *http.Request) {
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
		RetreatAtLoss *float64 `json:"retreat_at_loss,omitempty"`
		HoldToLastMan *bool    `json:"hold_to_last_man,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	ctx := r.Context()
	order := combat.StandingOrdersOrder{
		WorldID: worldID, PlayerID: playerID, UnitID: unitID,
		RetreatAtLoss: req.RetreatAtLoss, HoldToLastMan: req.HoldToLastMan,
	}

	// Same distance-0-vs-courier split as SetStance: a field unit (no
	// settlement, has a map position) dispatches a Runner; a garrison unit
	// skips straight to the direct-apply path below.
	if u, uErr := h.store.Get(ctx, unitID); uErr == nil &&
		u.OwnerID == playerID && u.WorldID == worldID &&
		u.SettlementID == nil && u.Q != nil && u.R != nil {
		// R3 (megaron_plan_skeppsuppdrag_landsatt.md): a ship not docked at its
		// own port takes no orders, standing orders included.
		if rej := combat.RequireShipInPort(ctx, h.pool, worldID, playerID,
			unit.CategoryOf(u.Type), u.Status, unit.LoadDisplayName(ctx, h.pool, u.ID),
			u.TargetQ, u.TargetR, u.ArrivesAt); rej != nil {
			writeError(w, rej.Status, rej.Reason)
			return
		}
		unitPos := province.MapPosition{Q: *u.Q, R: *u.R}
		origin, originOK := h.resolveOrderOrigin(w, ctx, worldID, playerID, unitPos)
		if !originOK {
			return
		}
		if origin.dist > 0 {
			h.sendOrderCourier(w, ctx, messenger.OrderDeliveryPayload{
				WorldID: worldID, PlayerID: playerID, UnitID: unitID,
				Verb: "standing_orders", StandingOrders: &order,
			}, "Runner — retreat order.", origin, unitPos, map[string]any{})
			return
		}
	}

	res, err := combat.SetStandingOrders(ctx, h.pool, h.eventStore, order)
	if err != nil {
		var rej *combat.OrderReject
		if errors.As(err, &rej) {
			writeError(w, rej.Status, rej.Reason)
			return
		}
		writeError(w, http.StatusInternalServerError, "standing orders change failed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"unit_id":          res.UnitID,
		"battle_id":        res.BattleID,
		"retreat_at_loss":  res.RetreatAtLoss,
		"hold_to_last_man": res.HoldToLastMan,
	})
}

// OccupationOrder handles POST /worlds/{worldID}/settlements/{settlementID}/occupation-order
// — the S3 erövring choice (megaron_plan_erovring.md): sack, sack-and-burn, or
// annex a city the player currently holds under occupation. Doing nothing
// leaves the city occupied (the default) — this endpoint is only reached when
// the Wanax actively chooses one of the other three. Always dispatched as a
// Runner order (megaron_plan_erovring.md's "command is never instant" — no
// distance-0 fast path here, unlike SetStance's garrisoned-unit case: the
// occupied city is never among the player's OWN active settlements, so
// resolveOrderOrigin always finds a real travel distance to it).
func (h *UnitHandler) OccupationOrder(w http.ResponseWriter, r *http.Request) {
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
	settlementID, err := uuid.Parse(chi.URLParam(r, "settlementID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid settlement ID")
		return
	}

	var req struct {
		Action string   `json:"action"` // "sack" | "burn" | "annex"
		Goods  []string `json:"goods,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	switch req.Action {
	case "sack", "burn", "annex":
	default:
		writeError(w, http.StatusBadRequest, `invalid action: must be "sack", "burn", or "annex"`)
		return
	}

	ctx := r.Context()

	// Pre-flight: settlement must exist, be in this world, and currently held
	// by this player under occupation. The runner-delivered execution
	// (combat.ExecuteOccupyAction) re-validates authoritatively at delivery
	// time — state can change while the courier travels — this is only so an
	// obviously bad order fails now, not by notice.
	var settleWorldID uuid.UUID
	var state string
	var occupantID *uuid.UUID
	var settleQ, settleR int
	if err := h.pool.QueryRow(ctx,
		`SELECT s.world_id, s.state, s.occupant_id, p.map_q, p.map_r
		 FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE s.id = $1`, settlementID,
	).Scan(&settleWorldID, &state, &occupantID, &settleQ, &settleR); err != nil {
		writeError(w, http.StatusNotFound, "settlement not found")
		return
	}
	if settleWorldID != worldID {
		writeError(w, http.StatusForbidden, "settlement not in this world")
		return
	}
	if state != "occupied" || occupantID == nil || *occupantID != playerID {
		writeError(w, http.StatusUnprocessableEntity, "you do not currently hold this city under occupation")
		return
	}

	unitPos := province.MapPosition{Q: settleQ, R: settleR}
	origin, originOK := h.resolveOrderOrigin(w, ctx, worldID, playerID, unitPos)
	if !originOK {
		return
	}
	order := combat.OccupyActionOrder{
		WorldID: worldID, PlayerID: playerID, SettlementID: settlementID,
		Action: req.Action, Goods: req.Goods,
	}
	h.sendOrderCourier(w, ctx, messenger.OrderDeliveryPayload{
		WorldID: worldID, PlayerID: playerID, Verb: "occupy_action", Occupy: &order,
	}, fmt.Sprintf("Runner — occupation order (%s).", req.Action),
		origin, unitPos, map[string]any{"action": req.Action})
}

// ListUnits handles GET /worlds/{worldID}/units — returns all non-disbanded units
// owned by the authenticated player in this world.
func (h *UnitHandler) ListUnits(w http.ResponseWriter, r *http.Request) {
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

	units, err := h.store.ListByOwner(r.Context(), playerID, worldID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load units")
		return
	}

	var currentTick int
	_ = h.pool.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&currentTick)
	// Wanaxens namn behövs för Nomadic Host — den hör till en PERSON, inte till
	// en stad, eftersom den existerar innan någon stad gör det.
	var wanax string
	_ = h.pool.QueryRow(r.Context(), `SELECT COALESCE(wanax_name, username) FROM players WHERE id = $1`, playerID).Scan(&wanax)
	// movement 2a, R7: current_q/r for a marching unit with a gällande saved
	// route comes from the core, at this exact Milli. A failed anchor load
	// (should not happen for a real world) just means no unit gets current_q/r
	// this call — never a guessed position.
	var nowMilli int64
	hasAnchor := false
	if anchor, aErr := tick.LoadAnchor(r.Context(), h.pool, worldID); aErr == nil {
		nowMilli = anchor.MilliAt(h.clk.Now())
		hasAnchor = true
	}
	summaries := unitSummaries(units, currentTick, h.clk,
		settlementNames(r.Context(), h.pool, worldID, playerID), wanax, nowMilli, hasAnchor)
	attachUnitPaths(r.Context(), h.pool, worldID, units, summaries)
	attachBattleFlags(r.Context(), h.pool, worldID, playerID, summaries)
	attachFreightingNotes(r.Context(), h.pool, worldID, playerID, summaries)
	attachPassageNotes(r.Context(), h.pool, worldID, units, summaries)
	attachPickupNotes(r.Context(), h.pool, worldID, playerID, units, summaries)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"units": summaries})
}

// settlementNames ger id → namn för spelarens städer, så namnstandarden kan
// formateras serversidan. En stad som fallit finns inte i kartan — och då faller
// "of <stad>"-ledet bort av sig självt, vilket är exakt rätt: förbandet har
// ingen försörjande stad längre.
func settlementNames(ctx context.Context, db province.Queryer, worldID, ownerID uuid.UUID) map[uuid.UUID]string {
	names := make(map[uuid.UUID]string)
	rows, err := db.Query(ctx,
		`SELECT id, name FROM settlements WHERE world_id = $1 AND owner_id = $2`,
		worldID, ownerID)
	if err != nil {
		return names
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var name string
		if rows.Scan(&id, &name) == nil {
			names[id] = name
		}
	}
	return names
}

// attachUnitPaths fills Path (the real A* route) for every marching unit, loading
// the world's tile graph once. Non-marching units are left with an empty path —
// they are not animated. Reuses marchPathWaypoints (world.go).
func attachUnitPaths(ctx context.Context, db province.Queryer, worldID uuid.UUID, units []*unit.Unit, summaries []unitSummary) {
	// movement 2a, R7: a gällande saved route (combat.LoadActiveRoute) answers
	// Path directly — route.Hexes, no FindPath at all. Only units WITHOUT one
	// (pre-153 marches, or the straight-line fallback) need the old re-search
	// below, so the tile graph is loaded lazily, only if at least one remains.
	unitByID := make(map[uuid.UUID]*unit.Unit, len(units))
	for _, u := range units {
		unitByID[u.ID] = u
	}
	needsSearch := false
	for i := range summaries {
		s := &summaries[i]
		if s.Status != "marching" || s.Q == nil || s.R == nil || s.TargetQ == nil || s.TargetR == nil {
			continue
		}
		u := unitByID[s.ID]
		if u == nil {
			continue
		}
		if route, ok := combat.LoadActiveRoute(u.MarchRoute, string(u.Status), u.DepartTick, u.ArriveTick); ok {
			s.Path = route.Hexes
			continue
		}
		needsSearch = true
	}
	if !needsSearch {
		return
	}
	g, err := province.LoadTileGraph(ctx, db, worldID)
	if err != nil {
		return
	}
	for i := range summaries {
		s := &summaries[i]
		if s.Status != "marching" || s.Q == nil || s.R == nil || s.TargetQ == nil || s.TargetR == nil || s.Path != nil {
			continue
		}
		cat := "land"
		if s.Category == "naval" {
			cat = "naval"
		}
		s.Path = marchPathWaypoints(g, *s.Q, *s.R, *s.TargetQ, *s.TargetR, cat)
	}
}

// unitSummary is the JSON shape returned to clients.
type unitSummary struct {
	ID       uuid.UUID `json:"id"`
	Type     string    `json:"type"`
	Category string    `json:"category"`
	Size     int       `json:"size"`
	Crew     int       `json:"crew,omitempty"`
	// Hull is the graded damage track (megaron_plan_skeppsreparation.md §B2,
	// 5=untouched/0=sunk) — omitted for land units (always the DB default 5,
	// meaningless for them; see unit.Unit.Hull's own doc comment).
	Hull int `json:"hull,omitempty"`
	// ProvisionDays är matmätaren (Timothy 2026-08-26): hur många DYGN skeppets
	// lager räcker vid nuvarande ranson. Dygn, inte råa korn — världen mäts i
	// speldygn och "14 dygn" är ett tal en spelare kan handla på. Utelämnas för
	// landenheter, som aldrig provianteras.
	ProvisionDays int    `json:"provision_days,omitempty"`
	Status        string `json:"status"`
	// Deployable is false while a land unit is still "forming": it cannot march,
	// colonize, or otherwise leave its settlement until it reaches 100 men. JSON
	// consumers (LLM agents, iOS) must see this in the data — the human `unit list`
	// already spells it out, but `--json` callers were misreading `complete_at`
	// (per-batch training time) as a deploy time and getting stuck.
	Deployable  bool `json:"deployable"`
	MenToDeploy int  `json:"men_to_deploy,omitempty"`
	// Name is the ship's name (Wanax-chosen or suggested at recruit time);
	// nil for land units (ship-build overhaul 2026-07-09).
	Name *string `json:"name,omitempty"`
	// DisplayName är namnstandardens färdigformaterade namn ("2nd Spearmen of
	// Knossos"). SERVERN formaterar — annars hamnar grammatiken i webben, keryx
	// och iOS var för sig och glider isär. Komponenterna följer med så en drawer
	// kan visa dem separat utan att bygga om grammatiken.
	DisplayName           string     `json:"display_name"`
	Ordinal               int        `json:"ordinal,omitempty"`
	SupportSettlementID   *uuid.UUID `json:"support_settlement_id,omitempty"`
	SupportSettlementName string     `json:"support_settlement_name,omitempty"`
	// BuildCompleteAt is the ETA for a still-forming naval unit; nil once
	// garrisoned, and always nil for land units (whose "forming" is
	// size-based, not time-based).
	BuildCompleteAt *time.Time `json:"build_complete_at,omitempty"`
	Stance          *string    `json:"stance,omitempty"`
	SettlementID    *uuid.UUID `json:"settlement_id,omitempty"`
	Q               *int       `json:"q,omitempty"`
	R               *int       `json:"r,omitempty"`
	TargetQ         *int       `json:"target_q,omitempty"`
	TargetR         *int       `json:"target_r,omitempty"`
	// DepartsAt + ArrivesAt let the map interpolate a marching unit's position
	// along its route (same as marches/messengers/trades); without departs_at a
	// per-unit march could not be animated and stayed invisible on the canvas.
	DepartsAt *time.Time `json:"departs_at,omitempty"`
	ArrivesAt *time.Time `json:"arrives_at,omitempty"`
	// K4 tick-contract: a marching unit's timing in world ticks (the source of
	// truth under the tick substrate, mig 067), plus a derived UTC convenience.
	// ArrivalTick/DurationTicks come straight off the unit row (arrive_tick,
	// arrive_tick − depart_tick); ArrivesAtUTC is derived via tick.EtaAt from the
	// world's current tick. All nil for a non-marching unit (tick columns cleared
	// on arrival, exactly like arrives_at).
	ArrivalTick   *int       `json:"arrival_tick,omitempty"`
	DurationTicks *int       `json:"duration_ticks,omitempty"`
	ArrivesAtUTC  *time.Time `json:"arrives_at_utc,omitempty"`
	// Path is the A* route [[q,r],...] a marching unit follows (via sea / around
	// mountains). The map animates the walker along it instead of a straight line,
	// so it is drawn where the unit truly is. Empty for non-marching units and when
	// no route exists (client falls back to the straight line). See marchPathWaypoints.
	Path [][2]int `json:"path,omitempty"`
	// CurrentQ/CurrentR (movement 2a, R7) is a marching unit's live position
	// read through its saved route (combat.RoutePositionAt) — never re-searched,
	// never guessed. Omitted unless a gällande route exists AND the world's
	// tick anchor loaded; a march with no saved route (pre-153, or the
	// straight-line fallback) omits it too. Client falls back to interpolating
	// Path/DepartsAt/ArrivesAt itself when these are absent, exactly as today.
	CurrentQ    *int       `json:"current_q,omitempty"`
	CurrentR    *int       `json:"current_r,omitempty"`
	CargoUnitID *uuid.UUID `json:"cargo_unit_id,omitempty"`
	// CarrierShipID/Name identify the ship an embarked land unit is aboard (the
	// ship whose cargo_unit_id points back at this unit). Without them a `unit
	// list` row for embarked cargo could not say which vessel carried it, so a
	// cargo unit stranded at sea (assault target vanished) looked orphaned.
	// Nil unless the unit is embarked.
	CarrierShipID   *uuid.UUID `json:"carrier_ship_id,omitempty"`
	CarrierShipName *string    `json:"carrier_ship_name,omitempty"`
	// MarchIntent/ColonyName surface a pending colony before it exists (Fas
	// 2i): a colonize march has no settlement row until it arrives, so this
	// was the only place its chosen name was visible at all.
	MarchIntent *string `json:"march_intent,omitempty"`
	ColonyName  *string `json:"colony_name,omitempty"`
	// OriginSettlementID/Reinforcing/CanReinforce (mig 126,
	// megaron_plan_rekryteringsmodell.md) surface the manskaps-underhåll
	// state: the cohort's permanent home city, whether the tick worker is
	// currently trickling men into it, and whether the reinforce button
	// should even show — true only while status=garrison AND
	// settlement_id == origin_settlement_id AND size < 100. Nil/false for
	// naval units and for land units recruited before mig 126 with no
	// settlement_id at backfill time.
	OriginSettlementID *uuid.UUID `json:"origin_settlement_id,omitempty"`
	Reinforcing        bool       `json:"reinforcing,omitempty"`
	CanReinforce       bool       `json:"can_reinforce,omitempty"`
	// InBattle is true while the unit is an active participant in an active
	// battle — the only time a per-unit retreat order (SetStandingOrders)
	// can take. Clients show that control only then; outside battle the
	// realm-wide retreat default (GET/PUT …/retreat-default) is what applies.
	InBattle bool `json:"in_battle"`
	// FreightingNote (megaron_plan_sjohandel_kraver_skepp.md R2/R4) explains
	// what a status='freighting' ship is doing — a single naval transfer's
	// destination, or which standing sea route it's locked to — nil for every
	// other status. Server-formatted for the same reason DisplayName is: a
	// client should never have to reconstruct this from transport/route rows
	// itself.
	FreightingNote *string `json:"freighting_note,omitempty"`
	// PassageFor/WaitingForReturn (megaron_plan_ordna_passage.md 3b-3): a ship
	// on a "passage" mission names the runner's destination; once it actually
	// holds for that runner's return leg (march_intent="passage_wait"),
	// WaitingForReturn says so plainly. Server-formatted for the same reason
	// FreightingNote is — the client never re-derives march_intent's meaning.
	PassageFor       *string `json:"passage_for,omitempty"`
	WaitingForReturn bool    `json:"waiting_for_return,omitempty"`
	// PickupFor/ShoreQ,ShoreR/WaitingUntilTick (megaron_plan_hamta_hem.md,
	// slice 2b): a ship on a "pickup"/"pickup_wait" mission names the fetched
	// unit and the shore it is sailing to (or waiting off); WaitingUntilTick
	// is set only while parked pickup_wait — the tick ScheduledPickupTimeout
	// will fire at if the unit never makes it. Server-formatted for the same
	// reason PassageFor is.
	PickupFor        *string `json:"pickup_for,omitempty"`
	ShoreQ           *int    `json:"shore_q,omitempty"`
	ShoreR           *int    `json:"shore_r,omitempty"`
	WaitingUntilTick *int    `json:"waiting_until_tick,omitempty"`
	// CanFetchByShip/PickupShips (R1, megaron_plan_hamta_hem.md): a
	// field-positioned OWN land unit (no settlement) that can be embarked
	// carries the caller's own idle ships that satisfy pickup's ship
	// condition — computed on the server so the client never re-derives R1's
	// rule itself (megaron_arbetssatt: the client must never promise an
	// action the server cannot perform). No sea route is checked per row —
	// that is only ever proven at POST .../pickup.
	CanFetchByShip bool               `json:"can_fetch_by_ship,omitempty"`
	PickupShips    []pickupShipOption `json:"pickup_ships,omitempty"`
}

// pickupShipOption is one of the caller's own ships offered as a pickup
// choice for a fetchable field unit (R1, megaron_plan_hamta_hem.md).
type pickupShipOption struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Type           string    `json:"type"`
	SettlementID   uuid.UUID `json:"settlement_id"`
	SettlementName string    `json:"settlement_name"`
	// CanCarryRunner is false for a war galley — it can only fetch a unit
	// that already stands on the shore, never one needing a runner (R1).
	CanCarryRunner bool `json:"can_carry_runner"`
}

// attachBattleFlags sets InBattle for every unit that is currently an active
// participant (left_tick IS NULL) in an active battle — the exact condition
// SetStandingOrders checks before it accepts a per-unit retreat order.
func attachBattleFlags(ctx context.Context, db province.Queryer, worldID, ownerID uuid.UUID, summaries []unitSummary) {
	rows, err := db.Query(ctx,
		`SELECT bp.unit_id FROM battle_participants bp JOIN battles b ON b.id = bp.battle_id
		 WHERE b.world_id = $1 AND b.status = 'active' AND bp.left_tick IS NULL AND bp.owner_id = $2`,
		worldID, ownerID)
	if err != nil {
		return
	}
	defer rows.Close()
	fighting := make(map[uuid.UUID]bool)
	for rows.Next() {
		var id uuid.UUID
		if rows.Scan(&id) == nil {
			fighting[id] = true
		}
	}
	for i := range summaries {
		summaries[i].InBattle = fighting[summaries[i].ID]
	}
}

// attachFreightingNotes (megaron_plan_sjohandel_kraver_skepp.md R2/R4) fills
// FreightingNote for every status='freighting' ship. A currently in-transit
// leg (the ship is actually moving right now) wins over a standing route's
// own note (the ship might be docked between legs, still locked to the
// route) — the more specific, currently-true fact is always preferred.
func attachFreightingNotes(ctx context.Context, db province.Queryer, worldID, ownerID uuid.UUID, summaries []unitSummary) {
	freighting := map[uuid.UUID]int{} // unit id -> index into summaries
	for i, s := range summaries {
		if s.Status == "freighting" {
			freighting[s.ID] = i
		}
	}
	if len(freighting) == 0 {
		return
	}

	notes := make(map[uuid.UUID]string, len(freighting))

	if rows, err := db.Query(ctx,
		`SELECT t.ship_unit_id, s.name FROM transports t JOIN settlements s ON s.id = t.dest_id
		 WHERE t.world_id = $1 AND t.owner_id = $2 AND t.status = 'in_transit' AND t.ship_unit_id IS NOT NULL`,
		worldID, ownerID,
	); err == nil {
		for rows.Next() {
			var shipID uuid.UUID
			var destName string
			if rows.Scan(&shipID, &destName) == nil {
				notes[shipID] = "carrying goods to " + destName
			}
		}
		rows.Close()
	}

	if rows, err := db.Query(ctx,
		`SELECT so.ship_unit_id, sf.name, st.name FROM standing_orders so
		 JOIN settlements sf ON sf.id = so.from_settlement_id
		 JOIN settlements st ON st.id = so.to_settlement_id
		 WHERE so.world_id = $1 AND so.owner_id = $2 AND so.ship_unit_id IS NOT NULL`,
		worldID, ownerID,
	); err == nil {
		for rows.Next() {
			var shipID uuid.UUID
			var fromName, toName string
			if rows.Scan(&shipID, &fromName, &toName) == nil {
				if _, alreadyNoted := notes[shipID]; !alreadyNoted {
					notes[shipID] = "on standing route " + fromName + " → " + toName
				}
			}
		}
		rows.Close()
	}

	for shipID, note := range notes {
		if i, ok := freighting[shipID]; ok {
			n := note
			summaries[i].FreightingNote = &n
		}
	}
}

// attachPassageNotes fills PassageFor/WaitingForReturn (megaron_plan_ordna_
// passage.md 3b-3) for every ship on a "passage"/"passage_wait" mission —
// units carries the same rows ListUnits already loaded (march_intent isn't
// itself enough to name a DESTINATION, which lives on the arranged runner).
func attachPassageNotes(ctx context.Context, db province.Queryer, worldID uuid.UUID, units []*unit.Unit, summaries []unitSummary) {
	index := make(map[uuid.UUID]int, len(summaries))
	for i, s := range summaries {
		index[s.ID] = i
	}
	var ids []uuid.UUID
	waiting := map[uuid.UUID]bool{}
	for _, u := range units {
		if u.MarchIntent == nil {
			continue
		}
		switch *u.MarchIntent {
		case "passage":
			ids = append(ids, u.ID)
		case "passage_wait":
			ids = append(ids, u.ID)
			waiting[u.ID] = true
		}
	}
	if len(ids) == 0 {
		return
	}

	rows, err := db.Query(ctx,
		`SELECT u.id, COALESCE(ds.name, 'its destination')
		   FROM units u
		   LEFT JOIN messengers m ON m.id = u.passage_messenger_id
		   LEFT JOIN settlements ds ON ds.id = m.destination_id
		  WHERE u.world_id = $1 AND u.id = ANY($2)`,
		worldID, ids,
	)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var shipID uuid.UUID
		var destName string
		if rows.Scan(&shipID, &destName) != nil {
			continue
		}
		i, ok := index[shipID]
		if !ok {
			continue
		}
		n := destName
		summaries[i].PassageFor = &n
		summaries[i].WaitingForReturn = waiting[shipID]
	}
}

// attachPickupNotes fills PickupFor/ShoreQ/ShoreR/WaitingUntilTick (R1,
// megaron_plan_hamta_hem.md) for every ship on a "pickup"/"pickup_wait"
// mission, and CanFetchByShip/PickupShips for every field-positioned (no
// settlement) OWN land unit that can be embarked — the surface a "Fetch by
// ship" button reads. units carries the same rows ListUnits already loaded.
func attachPickupNotes(ctx context.Context, db province.Queryer, worldID, ownerID uuid.UUID, units []*unit.Unit, summaries []unitSummary) {
	index := make(map[uuid.UUID]int, len(summaries))
	for i, s := range summaries {
		index[s.ID] = i
	}

	var shipIDs []uuid.UUID
	waiting := map[uuid.UUID]bool{}
	var fetchableIDs []uuid.UUID
	for _, u := range units {
		if u.MarchIntent != nil {
			switch *u.MarchIntent {
			case "pickup":
				shipIDs = append(shipIDs, u.ID)
			case "pickup_wait":
				shipIDs = append(shipIDs, u.ID)
				waiting[u.ID] = true
			}
		}
		if u.Status == "positioned" && u.SettlementID == nil &&
			unit.CategoryOf(u.Type) == unit.CategoryLand && unit.CanEmbark(u.Type) {
			fetchableIDs = append(fetchableIDs, u.ID)
		}
	}

	if len(shipIDs) > 0 {
		rows, err := db.Query(ctx,
			`SELECT u.id, u.land_target_q, u.land_target_r, COALESCE(pu.name, pu.type, 'the unit')
			   FROM units u
			   LEFT JOIN units pu ON pu.id = u.pickup_unit_id
			  WHERE u.world_id = $1 AND u.id = ANY($2)`,
			worldID, shipIDs,
		)
		if err == nil {
			for rows.Next() {
				var shipID uuid.UUID
				var shoreQ, shoreR *int
				var pickupName string
				if rows.Scan(&shipID, &shoreQ, &shoreR, &pickupName) != nil {
					continue
				}
				i, ok := index[shipID]
				if !ok {
					continue
				}
				n := pickupName
				summaries[i].PickupFor = &n
				summaries[i].ShoreQ = shoreQ
				summaries[i].ShoreR = shoreR
			}
			rows.Close()
		}
		if len(waiting) > 0 {
			var waitingIDs []uuid.UUID
			for id := range waiting {
				waitingIDs = append(waitingIDs, id)
			}
			if rows, err := db.Query(ctx,
				`SELECT (payload->>'unit_id')::uuid, due_tick FROM scheduled_events
				  WHERE event_type = 'PickupTimeout' AND processed_at IS NULL
				    AND (payload->>'unit_id')::uuid = ANY($1)`,
				waitingIDs,
			); err == nil {
				for rows.Next() {
					var shipID uuid.UUID
					var dueTick int
					if rows.Scan(&shipID, &dueTick) != nil {
						continue
					}
					if i, ok := index[shipID]; ok {
						t := dueTick
						summaries[i].WaitingUntilTick = &t
					}
				}
				rows.Close()
			}
		}
	}

	if len(fetchableIDs) == 0 {
		return
	}
	ships, err := eligiblePickupShips(ctx, db, worldID, ownerID)
	if err != nil || len(ships) == 0 {
		return
	}
	for _, id := range fetchableIDs {
		i, ok := index[id]
		if !ok {
			continue
		}
		summaries[i].CanFetchByShip = true
		summaries[i].PickupShips = ships
	}
}

// eligiblePickupShips lists ownerID's own idle ships that satisfy R1's ship
// condition for a pickup mission: naval, garrisoned in their own port, no
// cargo. Whether each can actually carry a runner (not a war galley) is
// surfaced per-row so the client never re-derives R1's rule itself.
func eligiblePickupShips(ctx context.Context, db province.Queryer, worldID, ownerID uuid.UUID) ([]pickupShipOption, error) {
	rows, err := db.Query(ctx,
		`SELECT u.id, u.name, u.type, u.settlement_id, s.name
		   FROM units u JOIN settlements s ON s.id = u.settlement_id
		  WHERE u.world_id = $1 AND u.owner_id = $2 AND u.status = 'garrison'
		    AND u.category = 'naval' AND u.cargo_unit_id IS NULL`,
		worldID, ownerID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []pickupShipOption
	for rows.Next() {
		var s pickupShipOption
		var name *string
		if err := rows.Scan(&s.ID, &name, &s.Type, &s.SettlementID, &s.SettlementName); err != nil {
			return nil, err
		}
		s.Name = unit.DisplayName(s.Type)
		if name != nil && *name != "" {
			s.Name = *name
		}
		s.CanCarryRunner = s.Type != string(unit.TypeWarGalley)
		out = append(out, s)
	}
	if out == nil {
		out = []pickupShipOption{}
	}
	return out, rows.Err()
}

// townNames är id → namn för de städer enheterna hänvisar till. Utan den kan
// servern inte formatera namnstandarden, och då hamnar grammatiken i klienterna.
func unitSummaries(us []*unit.Unit, currentTick int, clk clock.Clock, townNames map[uuid.UUID]string, wanax string, nowMilli int64, hasAnchor bool) []unitSummary {
	// Reverse map: cargo unit id → the ship carrying it, so an embarked unit can
	// name its carrier. A ship and its cargo are owned by the same Wanax, so both
	// rows are in us — no extra query needed.
	carrierByCargo := make(map[uuid.UUID]*unit.Unit)
	// Framåtuppslag id → enhet, så ett skepp kan läsa sin lasts typ och storlek
	// när matmätarens ranson räknas (ett lastat skepp äter mer).
	unitByID := make(map[uuid.UUID]*unit.Unit, len(us))
	for _, u := range us {
		unitByID[u.ID] = u
		if u.CargoUnitID != nil {
			carrierByCargo[*u.CargoUnitID] = u
		}
	}
	out := make([]unitSummary, 0, len(us))
	for _, u := range us {
		var stance *string
		if u.Stance != nil {
			s := string(*u.Stance)
			stance = &s
		}
		// K4 tick-contract: surface the march's arrival in ticks (+ a derived UTC).
		// arrive_tick is the tick-native mirror of arrives_at and is nil unless the
		// unit is marching, so these three all appear together or not at all.
		var arrivalTick, durationTicks *int
		var arrivesAtUTC *time.Time
		if u.ArriveTick != nil {
			at := *u.ArriveTick
			arrivalTick = &at
			utc := tick.EtaAt(clk, at, currentTick).UTC()
			arrivesAtUTC = &utc
			if u.DepartTick != nil {
				d := at - *u.DepartTick
				durationTicks = &d
			}
		}
		// A land unit is deployable once it reaches garrison: it gathers men while
		// "forming" (< 100), then matures for one training duration as "training"
		// (100/100, build_complete_at = ready ETA), then flips to garrison. A naval
		// unit is deployable once its build completes ("forming" until then, ship-
		// build overhaul 2026-07-09). men_to_deploy only makes sense for a gathering
		// land unit; training/naval forming show build_complete_at instead.
		// Namnstandarden (megaron_aktorer_plan.md §7). Skepp bär egennamn efter
		// kommat, landförband ordinal före typen. Saknas den försörjande staden
		// faller ledet bort — namnet blir kortare, aldrig trasigt.
		town := ""
		if u.SupportSettlementID != nil {
			town = townNames[*u.SupportSettlementID]
		}
		ordinal := 0
		if u.Ordinal != nil {
			ordinal = *u.Ordinal
		}
		var nm unit.Name
		switch {
		case u.Type == unit.TypeNomadicHost:
			nm = unit.HostName(wanax)
		case u.Category == unit.CategoryNaval:
			shipName := ""
			if u.Name != nil {
				shipName = *u.Name
			}
			nm = unit.ShipDisplayName(string(u.Type), shipName, town)
		default:
			nm = unit.LandUnitName(string(u.Type), ordinal, town, wanax)
		}

		// 'repairing' (megaron_plan_skeppsreparation.md Slice C): a ship
		// mid-repair cannot march any more than a still-forming one can.
		// 'freighting' (megaron_plan_sjohandel_kraver_skepp.md R2): a ship bound
		// to a naval transport leg or standing sea route — same non-deployable
		// gate, it belongs to the transport/route until that releases it.
		deployable := u.Status != "forming" && u.Status != "training" && u.Status != "repairing" && u.Status != "freighting"
		menToDeploy := 0
		if u.Status == "forming" && u.Category == unit.CategoryLand {
			menToDeploy = 100 - u.Size
		}
		// canReinforce mirrors the Reinforce handler's own gate exactly (mig 126):
		// garrison, in origin, under full strength. Computed here so the client
		// never has to re-derive it from settlement_id/origin_settlement_id
		// itself (see war.js's stale "batches of 10" hint this replaces).
		canReinforce := u.Status == unit.StatusGarrison && u.SettlementID != nil &&
			u.OriginSettlementID != nil && *u.SettlementID == *u.OriginSettlementID &&
			u.Size < economy.MaxUnitSize
		var carrierShipID *uuid.UUID
		var carrierShipName *string
		if u.Status == "embarked" {
			if ship, ok := carrierByCargo[u.ID]; ok {
				cid := ship.ID
				carrierShipID = &cid
				carrierShipName = ship.Name
			}
		}
		hull := 0
		provisionDays := 0
		if u.Category == unit.CategoryNaval {
			hull = u.Hull
			// Ransonen måste räknas med lasten ombord — ett lastat skepp äter
			// mer, så samma proviant räcker färre dygn. Räknas ur combat, samma
			// funktion som provianteringen och dragningen använder, annars
			// visar mätaren ett annat tal än det som faktiskt äts.
			cargoType, cargoSize := "", 0
			if u.CargoUnitID != nil {
				if c, ok := unitByID[*u.CargoUnitID]; ok {
					cargoType, cargoSize = string(c.Type), c.Size
				}
			}
			provisionDays = combat.ProvisionDaysLeft(u.Provisions,
				combat.VoyageRation(string(u.Type), u.Size, cargoType, cargoSize))
		}
		// movement 2a, R7: current_q/r is read through the saved route — never
		// re-searched, never guessed. Omitted (nil) unless a gällande route
		// exists AND the world's tick anchor loaded successfully.
		var currentQ, currentR *int
		if hasAnchor {
			if route, ok := combat.LoadActiveRoute(u.MarchRoute, string(u.Status), u.DepartTick, u.ArriveTick); ok {
				if pos, err := combat.RoutePositionAt(route, nowMilli); err == nil {
					q, r := pos.Q, pos.R
					currentQ, currentR = &q, &r
				}
			}
		}
		out = append(out, unitSummary{
			ID:                    u.ID,
			Type:                  string(u.Type),
			Category:              string(u.Category),
			Size:                  u.Size,
			Crew:                  u.Crew,
			Hull:                  hull,
			ProvisionDays:         provisionDays,
			Status:                string(u.Status),
			Deployable:            deployable,
			MenToDeploy:           menToDeploy,
			DisplayName:           nm.DisplayName,
			Ordinal:               ordinal,
			SupportSettlementID:   u.SupportSettlementID,
			SupportSettlementName: town,
			Name:                  u.Name,
			BuildCompleteAt:       u.BuildCompleteAt,
			Stance:                stance,
			SettlementID:          u.SettlementID,
			Q:                     u.Q,
			R:                     u.R,
			TargetQ:               u.TargetQ,
			TargetR:               u.TargetR,
			DepartsAt:             u.DepartsAt,
			ArrivesAt:             u.ArrivesAt,
			ArrivalTick:           arrivalTick,
			DurationTicks:         durationTicks,
			ArrivesAtUTC:          arrivesAtUTC,
			CargoUnitID:           u.CargoUnitID,
			CarrierShipID:         carrierShipID,
			CarrierShipName:       carrierShipName,
			MarchIntent:           u.MarchIntent,
			ColonyName:            u.ColonyName,
			OriginSettlementID:    u.OriginSettlementID,
			Reinforcing:           u.Reinforcing,
			CanReinforce:          canReinforce,
			CurrentQ:              currentQ,
			CurrentR:              currentR,
		})
	}
	return out
}
