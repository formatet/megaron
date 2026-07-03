package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"

	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/capabilities"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/messenger"
	"github.com/poleia/server/internal/province"
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
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	ctx := r.Context()

	// Load unit.
	u, err := h.store.Get(ctx, unitID)
	if err != nil {
		writeError(w, http.StatusNotFound, "unit not found")
		return
	}

	// Ownership check.
	if u.OwnerID != playerID {
		writeError(w, http.StatusForbidden, "not your unit")
		return
	}
	if u.WorldID != worldID {
		writeError(w, http.StatusForbidden, "unit not in this world")
		return
	}

	// Priests are stationary by rule.
	if u.Type == unit.TypePriest {
		writeError(w, http.StatusUnprocessableEntity, "priests are stationary and cannot march")
		return
	}

	// Must be garrisoned or positioned (positioned = on map without a settlement,
	// e.g. landed on empty hex; it must be able to march back or onward).
	if u.Status != unit.StatusGarrison && u.Status != unit.StatusPositioned {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("unit cannot march: status is '%s' (must be 'garrison' or 'positioned')", string(u.Status)))
		return
	}

	// Land units must be at full strength.
	if unit.CategoryOf(u.Type) == unit.CategoryLand && u.Size < 100 {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("unit is still forming (%d/100 men); it cannot march until fully recruited", u.Size))
		return
	}

	// C5: a unit in fortify stance is locked in place until stance is changed.
	if u.Stance != nil && *u.Stance == unit.StanceFortify {
		writeError(w, http.StatusUnprocessableEntity,
			"unit is in fortify stance and cannot march; change stance to 'none' first via POST /stance")
		return
	}

	// Validate optional stance value.
	if req.Stance != "" {
		switch unit.Stance(req.Stance) {
		case unit.StanceFortify, unit.StanceStorm, unit.StanceSentry:
			// valid
		default:
			writeError(w, http.StatusBadRequest, "invalid stance: must be fortify, storm, or sentry")
			return
		}
	}

	// Resolve origin position. Garrisoned units use their settlement's province
	// hex; positioned units (on the map without a settlement) use their stored q/r.
	var originQ, originR int
	if u.SettlementID != nil {
		var originTerrain string
		if err := h.pool.QueryRow(ctx,
			`SELECT p.map_q, p.map_r, p.terrain_type
			 FROM settlements s
			 JOIN provinces p ON p.id = s.province_id
			 WHERE s.id = $1`,
			*u.SettlementID,
		).Scan(&originQ, &originR, &originTerrain); err != nil {
			writeError(w, http.StatusInternalServerError, "could not load origin province")
			return
		}
		// Naval units cannot occupy land: a galley garrisoned at a coastal
		// settlement actually departs from the settlement's harbour — the nearest
		// adjacent sea hex — not the settlement's own (land) province hex. Without
		// this, FindPath always rejected the unit's own origin as impassable and
		// the ship could never leave port, no matter what it was told to do.
		if unit.CategoryOf(u.Type) == unit.CategoryNaval {
			seaQ, seaR, foundSea, seaErr := province.NearestSeaNeighbor(ctx, h.pool, worldID, originQ, originR)
			if seaErr != nil {
				writeError(w, http.StatusInternalServerError, "could not resolve naval departure hex")
				return
			}
			if !foundSea {
				writeError(w, http.StatusUnprocessableEntity,
					"settlement has no adjacent sea hex — naval units cannot depart an inland settlement")
				return
			}
			originQ, originR = seaQ, seaR
		}
	} else if u.Q != nil && u.R != nil {
		// positioned unit: origin is its current hex.
		originQ, originR = *u.Q, *u.R
	} else {
		writeError(w, http.StatusUnprocessableEntity, "unit has no known position; cannot determine departure hex")
		return
	}

	// Target hex must exist on this world's map.
	var destTerrain string
	if err := h.pool.QueryRow(ctx,
		`SELECT terrain FROM map_tiles WHERE world_id = $1 AND q = $2 AND r = $3`,
		worldID, req.TargetQ, req.TargetR,
	).Scan(&destTerrain); err != nil {
		writeError(w, http.StatusNotFound, "target hex not found")
		return
	}

	// Sanity: cannot march to own hex.
	if req.TargetQ == originQ && req.TargetR == originR {
		writeError(w, http.StatusBadRequest, "cannot march to own hex")
		return
	}

	// Colonize intent: validate up front so the agent gets an actionable error
	// instead of a silent return-home at arrival. Only "colonize" is supported.
	if req.Intent != "" && req.Intent != "colonize" {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("unknown march intent %q (only \"colonize\" is supported)", req.Intent))
		return
	}
	if req.Intent == "colonize" {
		// Only land units found colonies — they become the new colony's garrison.
		if unit.CategoryOf(u.Type) != unit.CategoryLand {
			writeError(w, http.StatusUnprocessableEntity, "only land units can found a colony")
			return
		}
		// The target province must be unclaimed (no settlement). Best-effort pre-flight;
		// the arrival handler re-checks under lock and returns the unit home on a race.
		var existing int
		if err := h.pool.QueryRow(ctx,
			`SELECT count(*) FROM settlements s
			 JOIN provinces p ON p.id = s.province_id
			 WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3`,
			worldID, req.TargetQ, req.TargetR,
		).Scan(&existing); err == nil && existing > 0 {
			writeError(w, http.StatusUnprocessableEntity,
				"target hex already has a settlement — colonize requires an empty hex")
			return
		}
		// Settlement cap: a Wanax may hold at most maxSettlementsPerWanax active
		// settlements. Enforced at dispatch so the harness gets immediate feedback
		// and the colonising army never wastes the march. The arrival handler is the
		// authoritative fallback if the count changes mid-transit.
		//
		// Reuses capabilities' colonize checker's settlement-cap requirement
		// directly (temenos_capabilities.md Fas 3 anti-drift) — not the whole
		// CanColonize verb, because its OTHER requirement ("a deployable land
		// unit garrisoned here") is aggregate-per-settlement and would wrongly
		// reject a "positioned" unit (already off any settlement, mid-journey)
		// that this handler has already validated is deployable by other means
		// (status + size checks above).
		capReq := capabilities.SettlementCapRequirement(
			capabilities.NewContext(ctx, h.pool, h.clk, worldID, uuid.Nil, playerID, uuid.Nil))
		if !capReq.Satisfied {
			writeError(w, http.StatusUnprocessableEntity, capReq.Hint)
			return
		}
	}

	// Mountains are impassable.
	if destTerrain == "mountain_limestone" || destTerrain == "mountain_red" {
		writeError(w, http.StatusUnprocessableEntity, "mountain terrain is impassable")
		return
	}

	// Land units cannot enter sea hexes.
	isSea := destTerrain == "coastal_sea" || destTerrain == "deep_sea"
	if unit.CategoryOf(u.Type) == unit.CategoryLand && isSea {
		writeError(w, http.StatusUnprocessableEntity, "land units cannot enter sea terrain")
		return
	}

	// A* pathfinding: verify that a traversable route exists and derive the real
	// path cost for ETA. This catches the land-over-sea bug (target on land, but
	// the only route crosses water) and routes around mountains correctly.
	_, pathCost, pathOK, pathErr := province.FindPath(ctx, h.pool, worldID,
		province.MapPosition{Q: originQ, R: originR},
		province.MapPosition{Q: req.TargetQ, R: req.TargetR},
		string(unit.CategoryOf(u.Type)),
	)
	if pathErr != nil {
		writeError(w, http.StatusInternalServerError, "pathfinding error")
		return
	}
	if !pathOK {
		hint := "a sea crossing needs a ship (embark), and mountains must be routed around"
		if unit.CategoryOf(u.Type) == unit.CategoryNaval {
			hint = "no sea route connects your harbour to that hex — land blocks the way"
		}
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("no passable route to (%d,%d) for this unit — %s", req.TargetQ, req.TargetR, hint))
		return
	}

	// Calculate movement time.
	dist := province.HexDistance(
		province.MapPosition{Q: originQ, R: originR},
		province.MapPosition{Q: req.TargetQ, R: req.TargetR},
	)
	if dist == 0 {
		writeError(w, http.StatusBadRequest, "target is the same hex as origin")
		return
	}

	moveHours := pathCost
	// Loaded ships move 1.5× slower.
	if u.CargoUnitID != nil {
		moveHours *= 1.5
	}

	now := h.clk.Now()
	arrivesAt := now.Add(time.Duration(moveHours * float64(time.Hour)))
	var unitMarchCurrentTick int
	_ = h.pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&unitMarchCurrentTick)
	unitTravelTicks := max(1, int(math.Round(moveHours)))

	// Atomic DB update: set unit to marching and schedule arrival event.
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not begin transaction")
		return
	}
	defer tx.Rollback(ctx)

	// Idempotency guard: re-read status inside the transaction with FOR UPDATE.
	var currentStatus string
	if err := tx.QueryRow(ctx,
		`SELECT status FROM units WHERE id = $1 FOR UPDATE`, unitID,
	).Scan(&currentStatus); err != nil {
		writeError(w, http.StatusNotFound, "unit not found in transaction")
		return
	}
	if unit.Status(currentStatus) != unit.StatusGarrison && unit.Status(currentStatus) != unit.StatusPositioned {
		writeError(w, http.StatusConflict, "unit status changed; march not sent")
		return
	}

	// Build stance SET clause only when provided.
	var stanceArg *string
	if req.Stance != "" {
		s := req.Stance
		stanceArg = &s
	}

	// Colonize intent + optional name ride along on the unit so the arrival
	// handler can found a colony. NULL intent = plain march (cleared each dispatch).
	var intentArg, nameArg *string
	if req.Intent == "colonize" {
		intentArg = &req.Intent
		if req.Name != "" {
			nameArg = &req.Name
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   status       = 'marching',
		   q            = $2,
		   r            = $3,
		   target_q     = $4,
		   target_r     = $5,
		   departs_at   = $6,
		   arrives_at   = $7,
		   settlement_id = NULL,
		   stance       = COALESCE($8, stance),
		   march_intent = $9,
		   colony_name  = $10,
		   updated_at   = now()
		 WHERE id = $1`,
		unitID, originQ, originR, req.TargetQ, req.TargetR, now, arrivesAt, stanceArg, intentArg, nameArg,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update unit")
		return
	}

	// Schedule the UnitArrival event.
	arrPayload := unit.ScheduledUnitArrivalPayload{
		UnitID:  unitID,
		WorldID: worldID,
	}
	if err := h.scheduler.EnqueueTickTx(ctx, tx, worldID, events.ScheduledUnitArrival, arrPayload, unitMarchCurrentTick+unitTravelTicks); err != nil {
		writeError(w, http.StatusInternalServerError, "could not schedule unit arrival")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "could not commit march")
		return
	}

	// Append domain event (outcome, not intention).
	stanceStr := req.Stance
	_, _ = h.eventStore.Append(ctx, unitID, events.StreamType(unit.StreamUnit), unit.EventUnitMarchOrdered,
		unit.UnitMarchOrderedPayload{
			UnitID:    unitID,
			OriginQ:   originQ,
			OriginR:   originR,
			TargetQ:   req.TargetQ,
			TargetR:   req.TargetR,
			Stance:    stanceStr,
			DepartsAt: now.Format(time.RFC3339),
			ArrivesAt: arrivesAt.Format(time.RFC3339),
		},
		worldID, nil,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"unit_id":    unitID,
		"departs_at": now,
		"arrives_at": arrivesAt,
		"origin_q":   originQ,
		"origin_r":   originR,
		"target_q":   req.TargetQ,
		"target_r":   req.TargetR,
	})
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

	// Resolve the settlement that dispatches the order messenger: the settlement
	// at the unit's march origin if one still stands there, else the owner's
	// capital (a positioned unit's origin hex may hold no settlement, but the
	// messengers row's origin_id is a hard settlement FK).
	var homeSettlementID uuid.UUID
	var homeQ, homeR int
	if err := h.pool.QueryRow(ctx,
		`SELECT s.id, p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3`,
		worldID, origin.Q, origin.R,
	).Scan(&homeSettlementID, &homeQ, &homeR); err != nil {
		if err := h.pool.QueryRow(ctx,
			`SELECT s.id, p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id
			 WHERE s.world_id = $1 AND s.owner_id = $2 AND s.is_capital = true`,
			worldID, playerID,
		).Scan(&homeSettlementID, &homeQ, &homeR); err != nil {
			writeError(w, http.StatusInternalServerError, "could not resolve a settlement to dispatch the order from")
			return
		}
	}

	dist := province.HexDistance(province.MapPosition{Q: homeQ, R: homeR}, currentPos)
	now := h.clk.Now()
	messengerArrivesAt := now.Add(messenger.MessengerTravelDuration(dist))
	var currentTick int
	_ = h.pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	dueTick := currentTick + messenger.MessengerTravelTicks(dist)

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

	var messengerID uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO messengers
		     (world_id, sender_id, origin_id, destination_id, message_text, status, kind, hex_q, hex_r, dest_q, dest_r, arrives_at)
		 VALUES ($1,$2,$3,NULL,$4,'outbound','recall',$5,$6,$7,$8,$9)
		 RETURNING id`,
		worldID, playerID, homeSettlementID, msgText, homeQ, homeR, currentPos.Q, currentPos.R, messengerArrivesAt,
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
	shipID, err := uuid.Parse(chi.URLParam(r, "shipID"))
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
	shipID, err := uuid.Parse(chi.URLParam(r, "shipID"))
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

	// Validate stance value.
	switch req.Stance {
	case "fortify", "storm", "sentry", "none":
		// valid
	default:
		writeError(w, http.StatusBadRequest, `invalid stance: must be "fortify", "storm", "sentry", or "none"`)
		return
	}

	ctx := r.Context()

	u, err := h.store.Get(ctx, unitID)
	if err != nil {
		writeError(w, http.StatusNotFound, "unit not found")
		return
	}
	if u.OwnerID != playerID {
		writeError(w, http.StatusForbidden, "not your unit")
		return
	}
	if u.WorldID != worldID {
		writeError(w, http.StatusForbidden, "unit not in this world")
		return
	}
	if u.Type == unit.TypePriest {
		writeError(w, http.StatusUnprocessableEntity, "priests cannot take a stance")
		return
	}
	if u.Status != unit.StatusGarrison && u.Status != unit.StatusPositioned {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("unit cannot change stance while %s (must be garrison or positioned)", string(u.Status)))
		return
	}

	// Determine new stance value and sentry coords.
	var newStance *string
	var newSentryQ, newSentryR *int
	if req.Stance != "none" {
		s := req.Stance
		newStance = &s
	}
	if req.Stance == "sentry" {
		// sentry_q/r = unit's current hex position.
		// For garrisoned units, resolve via settlement province.
		var hexQ, hexR int
		if u.Q != nil && u.R != nil {
			hexQ, hexR = *u.Q, *u.R
		} else if u.SettlementID != nil {
			if err := h.pool.QueryRow(ctx,
				`SELECT p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id WHERE s.id = $1`,
				*u.SettlementID,
			).Scan(&hexQ, &hexR); err != nil {
				writeError(w, http.StatusInternalServerError, "could not resolve unit hex for sentry")
				return
			}
		}
		newSentryQ = &hexQ
		newSentryR = &hexR
	}

	// Atomic update inside transaction with FOR UPDATE idempotency guard.
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not begin transaction")
		return
	}
	defer tx.Rollback(ctx)

	var currentStatus string
	var currentStance *string
	if err := tx.QueryRow(ctx,
		`SELECT status, stance FROM units WHERE id = $1 FOR UPDATE`, unitID,
	).Scan(&currentStatus, &currentStance); err != nil {
		writeError(w, http.StatusNotFound, "unit not found in transaction")
		return
	}
	if unit.Status(currentStatus) != unit.StatusGarrison && unit.Status(currentStatus) != unit.StatusPositioned {
		writeError(w, http.StatusConflict, "unit status changed; stance not applied")
		return
	}

	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   stance     = $2,
		   sentry_q   = $3,
		   sentry_r   = $4,
		   updated_at = now()
		 WHERE id = $1`,
		unitID, newStance, newSentryQ, newSentryR,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update stance")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "could not commit stance change")
		return
	}

	// Record stance-before for event payload.
	stanceBefore := ""
	if currentStance != nil {
		stanceBefore = *currentStance
	}
	stanceAfter := req.Stance
	if req.Stance == "none" {
		stanceAfter = ""
	}
	_, _ = h.eventStore.Append(ctx, unitID, events.StreamType(unit.StreamUnit), unit.EventUnitStanceChanged,
		unit.UnitStanceChangedPayload{
			UnitID:       unitID,
			WorldID:      worldID,
			StanceBefore: stanceBefore,
			StanceAfter:  stanceAfter,
			SentryQ:      newSentryQ,
			SentryR:      newSentryR,
		}, worldID, nil,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"unit_id":  unitID,
		"stance":   stanceAfter,
		"sentry_q": newSentryQ,
		"sentry_r": newSentryR,
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"units": unitSummaries(units)})
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
	Stance       *string    `json:"stance,omitempty"`
	SettlementID *uuid.UUID `json:"settlement_id,omitempty"`
	Q            *int       `json:"q,omitempty"`
	R            *int       `json:"r,omitempty"`
	TargetQ      *int       `json:"target_q,omitempty"`
	TargetR      *int       `json:"target_r,omitempty"`
	ArrivesAt    *time.Time `json:"arrives_at,omitempty"`
	CargoUnitID  *uuid.UUID `json:"cargo_unit_id,omitempty"`
}

func unitSummaries(us []*unit.Unit) []unitSummary {
	out := make([]unitSummary, 0, len(us))
	for _, u := range us {
		var stance *string
		if u.Stance != nil {
			s := string(*u.Stance)
			stance = &s
		}
		// A land unit is deployable once it is no longer "forming" (it auto-flips to
		// garrison at 100 men). Naval units are deployable from creation. men_to_deploy
		// tells a forming land unit exactly how many more men to recruit.
		deployable := u.Status != "forming"
		menToDeploy := 0
		if u.Status == "forming" {
			menToDeploy = 100 - u.Size
		}
		out = append(out, unitSummary{
			ID:           u.ID,
			Type:         string(u.Type),
			Category:     string(u.Category),
			Size:         u.Size,
			Crew:         u.Crew,
			Status:       string(u.Status),
			Deployable:   deployable,
			MenToDeploy:  menToDeploy,
			Stance:       stance,
			SettlementID: u.SettlementID,
			Q:            u.Q,
			R:            u.R,
			TargetQ:      u.TargetQ,
			TargetR:      u.TargetR,
			ArrivesAt:    u.ArrivesAt,
			CargoUnitID:  u.CargoUnitID,
		})
	}
	return out
}

