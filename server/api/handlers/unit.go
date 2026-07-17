package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/combat"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/messenger"
	"github.com/poleia/server/internal/province"
	"github.com/poleia/server/internal/tick"
	"github.com/poleia/server/internal/unit"
)

// UnitHandler handles HTTP requests for the unit endpoints (C3).
type UnitHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	eventStore *events.Store
	clk       clock.Clock
	store     *unit.Store
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
//   - Priests: may never march (stationary)
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
		Stance  string `json:"stance"`  // optional; fortify|storm|sentry — persisted for C5
		Intent  string `json:"intent"`  // optional; "" = plain march, "colonize" = found a colony on arrival
		Name    string `json:"name"`    // optional colony name (only used with intent=colonize)
		Mode    string `json:"mode"`    // optional; "" = sack (default) | "annex" — conquest choice on arrival (Del 2b)
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
		order := combat.MarchOrder{
			WorldID: worldID, PlayerID: playerID, UnitID: unitID,
			TargetQ: req.TargetQ, TargetR: req.TargetR,
			Stance: req.Stance, Intent: req.Intent, Name: req.Name, Mode: req.Mode,
		}
		h.dispatchMarchCourier(w, ctx, order, province.MapPosition{Q: *u.Q, R: *u.R}, targetKnown)
		return
	}

	// Validate+execute core shared with the order-courier delivery path
	// (temenos_orderlopare_plan.md Fas 1) — internal/combat.StartMarch.
	res, err := combat.StartMarch(ctx, h.pool, h.scheduler, h.eventStore, h.clk, combat.MarchOrder{
		WorldID:  worldID,
		PlayerID: playerID,
		UnitID:   unitID,
		TargetQ:  req.TargetQ,
		TargetR:  req.TargetR,
		Stance:   req.Stance,
		Intent:   req.Intent,
		Name:     req.Name,
		Mode:     req.Mode,
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
	})
}

// dispatchMarchCourier sends a march order to a field unit by physical runner —
// a hemerodromos, the Greek day-runner (Timothy 2026-07-17); DB identifier stays
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
	}, fmt.Sprintf("Hemerodromos — march order to (%d,%d).", order.TargetQ, order.TargetR),
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
	if err := pool.QueryRow(ctx,
		`SELECT fp.host_unit_id, u.status, u.category, u.q, u.r, u.target_q, u.target_r, u.departs_at, u.arrives_at
		 FROM founder_phase fp JOIN units u ON u.id = fp.host_unit_id
		 WHERE fp.world_id = $1 AND fp.owner_id = $2 AND fp.active
		   AND u.q IS NOT NULL AND u.r IS NOT NULL`,
		worldID, playerID,
	).Scan(&hostID, &status, &category, &q, &r, &targetQ, &targetR, &departsAt, &arrivesAt); err != nil {
		return uuid.Nil, province.MapPosition{}, false
	}
	pos := province.MapPosition{Q: q, R: r}
	if status == "marching" && targetQ != nil && targetR != nil && departsAt != nil && arrivesAt != nil {
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
				"you have no city (and no wandering host) to dispatch a hemerodromos from")
			return o, false
		}
		hid := hostID
		o = orderOrigin{unitID: &hid, q: pos.Q, r: pos.R, dist: province.HexDistance(pos, unitPos)}
	}
	return o, true
}

// sendOrderCourier inserts the kind='order' hemerodromos and schedules its
// ScheduledOrderDelivery, answering 202 order_dispatched with the courier ETA.
func (h *UnitHandler) sendOrderCourier(w http.ResponseWriter, ctx context.Context, payload messenger.OrderDeliveryPayload, msgText string, origin orderOrigin, unitPos province.MapPosition, extra map[string]any) {
	now := h.clk.Now()
	courierTravelTicks, courierTravelDur := messenger.CourierTravel(ctx, h.pool, payload.WorldID,
		province.MapPosition{Q: origin.q, R: origin.r}, unitPos)
	courierArrivesAt := now.Add(courierTravelDur)
	var currentTick int
	_ = h.pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	dueTick := currentTick + courierTravelTicks

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not begin transaction")
		return
	}
	defer tx.Rollback(ctx)

	var originQ, originR *int
	if origin.unitID != nil {
		originQ, originR = &origin.q, &origin.r
	}
	var messengerID uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO messengers
		     (world_id, sender_id, origin_id, origin_unit_id, origin_q, origin_r, destination_id, message_text, status, kind, hex_q, hex_r, dest_q, dest_r, arrives_at, order_payload)
		 VALUES ($1,$2,$3,$4,$5,$6,NULL,$7,'outbound','order',$8,$9,$10,$11,$12,$13)
		 RETURNING id`,
		payload.WorldID, payload.PlayerID, origin.settlementID, origin.unitID, originQ, originR,
		msgText, origin.q, origin.r, unitPos.Q, unitPos.R, courierArrivesAt, mustJSON(payload),
	).Scan(&messengerID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not dispatch order runner")
		return
	}
	payload.MessengerID = messengerID
	if err := h.scheduler.EnqueueTickTx(ctx, tx, payload.WorldID, events.ScheduledOrderDelivery, payload, dueTick); err != nil {
		writeError(w, http.StatusInternalServerError, "could not schedule order delivery")
		return
	}
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

// Recall handles POST /worlds/{worldID}/units/{unitID}/recall
//
// Sends a physical recall/redirect order to a marching unit (temenos_march_recall.md).
// Body (optional): {"target_q":int,"target_r":int} — omitted = recall (turn home to
// the hex the unit departed from); both given = redirect (new course). The order
// travels as a visible messenger; the unit keeps marching on its original course
// until the messenger physically catches up with it — command is never instant.
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

	// Guard: an earlier recall/redirect is already in flight for this unit.
	var pendingMessengerID uuid.UUID
	if err := h.pool.QueryRow(ctx,
		`SELECT (payload->>'messenger_id')::uuid
		 FROM scheduled_events
		 WHERE event_type = $1 AND (payload->>'unit_id')::uuid = $2
		   AND processed_at IS NULL AND failed_at IS NULL`,
		string(events.ScheduledMarchRecall), unitID,
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
		isSea := destTerrain == "coastal_sea" || destTerrain == "deep_sea"
		if unit.CategoryOf(u.Type) == unit.CategoryLand && isSea {
			writeError(w, http.StatusUnprocessableEntity, "land units cannot enter sea terrain")
			return
		}
	}

	// Interpolate the unit's actual current position along the path it already
	// proved traversable at dispatch — never a straight-line guess.
	currentPos, posOK, err := province.InterpolatePosition(ctx, h.pool, worldID, origin, target, category,
		*u.DepartsAt, *u.ArrivesAt, h.clk.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not resolve unit's current position")
		return
	}
	if !posOK {
		currentPos = origin
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

	// Resolve who dispatches the order messenger: the settlement at the unit's
	// march origin if one still stands there, else the owner's capital, else —
	// founder phase — the wandering host itself (mig 087 lets a messenger have a
	// unit origin; before founding there is no settlement anywhere to send from,
	// and reaching/recalling the escort is one of the host's designed uses).
	var originSettlementID, originUnitID *uuid.UUID
	var homeQ, homeR int
	var homeSettlementID uuid.UUID
	if err := h.pool.QueryRow(ctx,
		`SELECT s.id, p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3`,
		worldID, origin.Q, origin.R,
	).Scan(&homeSettlementID, &homeQ, &homeR); err == nil {
		originSettlementID = &homeSettlementID
	} else if err := h.pool.QueryRow(ctx,
		`SELECT s.id, p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE s.world_id = $1 AND s.owner_id = $2 AND s.is_capital = true`,
		worldID, playerID,
	).Scan(&homeSettlementID, &homeQ, &homeR); err == nil {
		originSettlementID = &homeSettlementID
	} else {
		hostID, pos, ok := hostCurrentPos(ctx, h.pool, h.clk.Now(), worldID, playerID)
		if !ok {
			writeError(w, http.StatusInternalServerError, "could not resolve a settlement to dispatch the order from")
			return
		}
		hid := hostID
		originUnitID, homeQ, homeR = &hid, pos.Q, pos.R
	}

	now := h.clk.Now()
	recallTravelTicks, recallTravelDur := messenger.CourierTravel(ctx, h.pool, worldID,
		province.MapPosition{Q: homeQ, R: homeR}, currentPos)
	messengerArrivesAt := now.Add(recallTravelDur)
	var currentTick int
	_ = h.pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	dueTick := currentTick + recallTravelTicks

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not begin transaction")
		return
	}
	defer tx.Rollback(ctx)

	// Idempotency / race guard: re-check status FOR UPDATE inside the tx.
	var lockedStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM units WHERE id = $1 FOR UPDATE`, unitID).Scan(&lockedStatus); err != nil {
		writeError(w, http.StatusNotFound, "unit not found in transaction")
		return
	}
	if unit.Status(lockedStatus) != unit.StatusMarching {
		writeError(w, http.StatusConflict, "unit status changed; order not sent")
		return
	}

	msgText := "Recall order — return home."
	if mode == "redirect" {
		msgText = fmt.Sprintf("Redirect order — new course to (%d,%d).", newTargetQ, newTargetR)
	}

	// origin_q/origin_r only ride along on a unit origin (mig 087): the host's
	// departure point is frozen here; a settlement origin keeps its coords in
	// the settlement row as before.
	var originQ, originR *int
	if originUnitID != nil {
		originQ, originR = &homeQ, &homeR
	}
	var messengerID uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO messengers
		     (world_id, sender_id, origin_id, origin_unit_id, origin_q, origin_r, destination_id, message_text, status, kind, hex_q, hex_r, dest_q, dest_r, arrives_at)
		 VALUES ($1,$2,$3,$4,$5,$6,NULL,$7,'outbound','recall',$8,$9,$10,$11,$12)
		 RETURNING id`,
		worldID, playerID, originSettlementID, originUnitID, originQ, originR,
		msgText, homeQ, homeR, currentPos.Q, currentPos.R, messengerArrivesAt,
	).Scan(&messengerID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not dispatch order messenger")
		return
	}

	payload := messenger.MarchRecallPayload{
		WorldID:     worldID,
		UnitID:      unitID,
		MessengerID: messengerID,
		Mode:        mode,
	}
	if mode == "redirect" {
		payload.NewTargetQ = &newTargetQ
		payload.NewTargetR = &newTargetR
	}
	if err := h.scheduler.EnqueueTickTx(ctx, tx, worldID, events.ScheduledMarchRecall, payload, dueTick); err != nil {
		writeError(w, http.StatusInternalServerError, "could not schedule order arrival")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "could not commit order")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":               "recall_order_sent",
		"unit_id":              unitID,
		"messenger_id":         messengerID,
		"messenger_arrives_at": messengerArrivesAt,
		"due_tick":             dueTick,
		"mode":                 mode,
	})
}

// Load handles POST /worlds/{worldID}/units/{shipID}/load
//
// Embarks a land unit onto a naval unit (the ship). Rules (C6 plan):
//   - Caller must own both units.
//   - Both units must be in the same settlement (garrison).
//   - Ship must be naval and have no current cargo (cargo_unit_id IS NULL).
//   - Land unit must be status='garrison', size=100, and not a priest.
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
	if cargo.Type == unit.TypePriest {
		writeError(w, http.StatusUnprocessableEntity, "priests cannot embark")
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
	if cargo.Size < 100 {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("cargo unit is still forming (%d/100); must be at full strength to embark", cargo.Size))
		return
	}
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
// Disembarks the cargo land unit from a naval unit. Rules (C6 plan):
//   - Caller must own the ship.
//   - Ship must have a cargo unit (cargo_unit_id non-null).
//   - Ship must be garrisoned at a coastal (adjacent to sea) settlement or harbour.
//
// Outcome: cargo unit status → 'garrison', q/r = ship's position, settlement_id =
// ship's settlement; ship.cargo_unit_id = NULL.
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
	if ship.Status != unit.StatusGarrison {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("ship must be garrisoned to unload (status: %s)", string(ship.Status)))
		return
	}
	if ship.CargoUnitID == nil {
		writeError(w, http.StatusUnprocessableEntity, "ship carries no unit")
		return
	}
	if ship.SettlementID == nil {
		writeError(w, http.StatusUnprocessableEntity, "ship has no settlement; cannot unload")
		return
	}

	// Disembark gating: must be coastal or harbour.
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

	cargoID := *ship.CargoUnitID

	// Load cargo to get its current position for the event.
	cargo, err := h.store.Get(ctx, cargoID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load cargo unit")
		return
	}

	// Get destination position from ship.
	var destQ, destR int
	if ship.Q != nil {
		destQ = *ship.Q
	}
	if ship.R != nil {
		destR = *ship.R
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
	if unit.Status(shipStatus) != unit.StatusGarrison {
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

	// Place cargo unit at ship's settlement.
	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   status        = 'garrison',
		   settlement_id = $2,
		   q             = $3,
		   r             = $4,
		   updated_at    = now()
		 WHERE id = $1`,
		cargoID, *ship.SettlementID, destQ, destR,
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
	})
}

// SetStance handles POST /worlds/{worldID}/units/{unitID}/stance
//
// Allows a garrison or positioned unit to adopt a combat stance without moving (C5).
// Body: {"stance":"fortify"|"storm"|"sentry"|"none"}
//
// Rules:
//   - Caller must own the unit.
//   - Unit must be status='garrison' or status='positioned' (not marching, forming, etc.).
//   - Priests may not take a stance.
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
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	ctx := r.Context()
	order := combat.StanceOrder{WorldID: worldID, PlayerID: playerID, UnitID: unitID, Stance: req.Stance}

	// Order latency (temenos_orderlopare_plan.md Fas 3): a stance order to a
	// field unit travels by hemerodromos from the nearest own city and applies
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
			}, fmt.Sprintf("Hemerodromos — stance order (%s).", req.Stance),
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
		"unit_id":  res.UnitID,
		"stance":   res.Stance,
		"sentry_q": res.SentryQ,
		"sentry_r": res.SentryR,
	})
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
	summaries := unitSummaries(units, currentTick, h.clk)
	attachUnitPaths(r.Context(), h.pool, worldID, summaries)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"units": summaries})
}

// attachUnitPaths fills Path (the real A* route) for every marching unit, loading
// the world's tile graph once. Non-marching units are left with an empty path —
// they are not animated. Reuses marchPathWaypoints (world.go).
func attachUnitPaths(ctx context.Context, db province.Queryer, worldID uuid.UUID, summaries []unitSummary) {
	marching := false
	for i := range summaries {
		s := &summaries[i]
		if s.Status == "marching" && s.Q != nil && s.R != nil && s.TargetQ != nil && s.TargetR != nil {
			marching = true
			break
		}
	}
	if !marching {
		return
	}
	g, err := province.LoadTileGraph(ctx, db, worldID)
	if err != nil {
		return
	}
	for i := range summaries {
		s := &summaries[i]
		if s.Status != "marching" || s.Q == nil || s.R == nil || s.TargetQ == nil || s.TargetR == nil {
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
	ID           uuid.UUID  `json:"id"`
	Type         string     `json:"type"`
	Category     string     `json:"category"`
	Size         int        `json:"size"`
	Crew         int        `json:"crew,omitempty"`
	Status       string     `json:"status"`
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
	// BuildCompleteAt is the ETA for a still-forming naval unit; nil once
	// garrisoned, and always nil for land units (whose "forming" is
	// size-based, not time-based).
	BuildCompleteAt *time.Time `json:"build_complete_at,omitempty"`
	Stance       *string    `json:"stance,omitempty"`
	SettlementID *uuid.UUID `json:"settlement_id,omitempty"`
	Q            *int       `json:"q,omitempty"`
	R            *int       `json:"r,omitempty"`
	TargetQ      *int       `json:"target_q,omitempty"`
	TargetR      *int       `json:"target_r,omitempty"`
	// DepartsAt + ArrivesAt let the map interpolate a marching unit's position
	// along its route (same as marches/messengers/trades); without departs_at a
	// per-unit march could not be animated and stayed invisible on the canvas.
	DepartsAt    *time.Time `json:"departs_at,omitempty"`
	ArrivesAt    *time.Time `json:"arrives_at,omitempty"`
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
	Path         [][2]int   `json:"path,omitempty"`
	CargoUnitID  *uuid.UUID `json:"cargo_unit_id,omitempty"`
	// MarchIntent/ColonyName surface a pending colony before it exists (Fas
	// 2i): a colonize march has no settlement row until it arrives, so this
	// was the only place its chosen name was visible at all.
	MarchIntent *string `json:"march_intent,omitempty"`
	ColonyName  *string `json:"colony_name,omitempty"`
}

func unitSummaries(us []*unit.Unit, currentTick int, clk clock.Clock) []unitSummary {
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
		deployable := u.Status != "forming" && u.Status != "training"
		menToDeploy := 0
		if u.Status == "forming" && u.Category == unit.CategoryLand {
			menToDeploy = 100 - u.Size
		}
		out = append(out, unitSummary{
			ID:              u.ID,
			Type:            string(u.Type),
			Category:        string(u.Category),
			Size:            u.Size,
			Crew:            u.Crew,
			Status:          string(u.Status),
			Deployable:      deployable,
			MenToDeploy:     menToDeploy,
			Name:            u.Name,
			BuildCompleteAt: u.BuildCompleteAt,
			Stance:          stance,
			SettlementID: u.SettlementID,
			Q:            u.Q,
			R:            u.R,
			TargetQ:      u.TargetQ,
			TargetR:      u.TargetR,
			DepartsAt:    u.DepartsAt,
			ArrivesAt:    u.ArrivesAt,
			ArrivalTick:   arrivalTick,
			DurationTicks: durationTicks,
			ArrivesAtUTC:  arrivesAtUTC,
			CargoUnitID:  u.CargoUnitID,
			MarchIntent:  u.MarchIntent,
			ColonyName:   u.ColonyName,
		})
	}
	return out
}

