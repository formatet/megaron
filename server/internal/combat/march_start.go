package combat

// StartMarch: the march order's validate+execute core, extracted from
// api/handlers.UnitHandler.March (temenos_orderlopare_plan.md Fas 1) so both
// dispatch paths share one implementation:
//   - the HTTP handler (distance-0 orders execute immediately), and
//   - the order-courier delivery handler in internal/messenger (Fas 2), which
//     runs it when the runner physically reaches the unit.
// No behaviour change: the checks and the TX are the handler's, verbatim.
//
// The FOW march rule (target must be seen/remembered) stays in the API layer —
// it is knowledge-at-dispatch, owned by the handlers package (loadLiveEyes /
// loadRememberedTiles) and injected here as a TargetKnownFunc. Delivery-time
// callers pass nil: knowledge was already checked when the order was given.

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/capabilities"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/province"
	"github.com/poleia/server/internal/tick"
	"github.com/poleia/server/internal/unit"
)

// MarchOrder is one march command against one unit.
type MarchOrder struct {
	WorldID  uuid.UUID
	PlayerID uuid.UUID
	UnitID   uuid.UUID
	TargetQ  int
	TargetR  int
	Stance   string // optional; fortify|storm|sentry — persisted for C5
	Intent   string // optional; "" = plain march, "colonize"/"explore"
	Name     string // optional colony name (only used with intent=colonize)
	Mode     string // optional; "" = sack (default) | "annex"
}

// MarchReject is a validation failure with the HTTP status the API layer
// should answer with; the delivery handler maps it to an OrderFailed notice.
type MarchReject struct {
	Status int
	Reason string
}

func (e *MarchReject) Error() string { return e.Reason }

func reject(status int, format string, args ...any) *MarchReject {
	return &MarchReject{Status: status, Reason: fmt.Sprintf(format, args...)}
}

// MarchStarted describes the accepted march (the handler's 202 body fields).
type MarchStarted struct {
	UnitID        uuid.UUID
	DepartsAt     time.Time
	ArrivesAt     time.Time
	ArrivalTick   int
	DurationTicks int
	OriginQ       int
	OriginR       int
	TargetQ       int
	TargetR       int
}

// TargetKnownFunc reports whether the ordering player has seen the target hex
// (tier-1 live or tier-2 remembered). nil = skip the check.
type TargetKnownFunc func(ctx context.Context, target province.MapPosition, terrain string) bool

// NavalSpeedFactor scales a unit's travel time by ship type (Timothy
// 2026-07-09): war galley fastest, merchantman (emporos) slowest, galley in
// between. Lower = faster. Non-naval and unknown types get 1.0. Tunable.
func NavalSpeedFactor(t unit.Type) float64 {
	switch t {
	case unit.TypeWarGalley:
		return 0.6
	case unit.TypeMerchantman:
		return 1.4
	default:
		return 1.0
	}
}

// StartMarch validates and executes one march order atomically. On success the
// unit is marching and its UnitArrival is scheduled. Any *MarchReject return
// carries the HTTP status + reason exactly as the March handler answered.
func StartMarch(ctx context.Context, pool *pgxpool.Pool, scheduler *events.Scheduler, eventStore *events.Store, clk clock.Clock, o MarchOrder, targetKnown TargetKnownFunc) (*MarchStarted, error) {
	store := unit.NewStore(pool)

	// Load unit.
	u, err := store.Get(ctx, o.UnitID)
	if err != nil {
		return nil, reject(http.StatusNotFound, "unit not found")
	}

	// Ownership check.
	if u.OwnerID != o.PlayerID {
		return nil, reject(http.StatusForbidden, "not your unit")
	}
	if u.WorldID != o.WorldID {
		return nil, reject(http.StatusForbidden, "unit not in this world")
	}

	// Priests are stationary by rule.
	if u.Type == unit.TypePriest {
		return nil, reject(http.StatusUnprocessableEntity, "priests are stationary and cannot march")
	}

	// Must be garrisoned or positioned (positioned = on map without a settlement,
	// e.g. landed on empty hex; it must be able to march back or onward).
	if u.Status != unit.StatusGarrison && u.Status != unit.StatusPositioned {
		return nil, reject(http.StatusUnprocessableEntity,
			"unit cannot march: status is '%s' (must be 'garrison' or 'positioned')", string(u.Status))
	}

	// C5: a unit in fortify stance is locked in place until stance is changed.
	if u.Stance != nil && *u.Stance == unit.StanceFortify {
		return nil, reject(http.StatusUnprocessableEntity,
			"unit is in fortify stance and cannot march; change stance to 'none' first via POST /stance")
	}

	// Validate optional stance value.
	if o.Stance != "" {
		switch unit.Stance(o.Stance) {
		case unit.StanceFortify, unit.StanceStorm, unit.StanceSentry:
			// valid
		default:
			return nil, reject(http.StatusBadRequest, "invalid stance: must be fortify, storm, or sentry")
		}
	}

	// Resolve origin position. Garrisoned units use their settlement's province
	// hex; positioned units (on the map without a settlement) use their stored q/r.
	var originQ, originR int
	if u.SettlementID != nil {
		var originTerrain string
		if err := pool.QueryRow(ctx,
			`SELECT p.map_q, p.map_r, p.terrain_type
			 FROM settlements s
			 JOIN provinces p ON p.id = s.province_id
			 WHERE s.id = $1`,
			*u.SettlementID,
		).Scan(&originQ, &originR, &originTerrain); err != nil {
			return nil, reject(http.StatusInternalServerError, "could not load origin province")
		}
		// Naval units cannot occupy land: a galley garrisoned at a coastal
		// settlement actually departs from the settlement's harbour — the nearest
		// adjacent sea hex — not the settlement's own (land) province hex. Without
		// this, FindPath always rejected the unit's own origin as impassable and
		// the ship could never leave port, no matter what it was told to do.
		if unit.CategoryOf(u.Type) == unit.CategoryNaval {
			seaQ, seaR, foundSea, seaErr := province.NearestSeaNeighbor(ctx, pool, o.WorldID, originQ, originR)
			if seaErr != nil {
				return nil, reject(http.StatusInternalServerError, "could not resolve naval departure hex")
			}
			if !foundSea {
				return nil, reject(http.StatusUnprocessableEntity,
					"settlement has no adjacent sea hex — naval units cannot depart an inland settlement")
			}
			originQ, originR = seaQ, seaR
		}
	} else if u.Q != nil && u.R != nil {
		// positioned unit: origin is its current hex.
		originQ, originR = *u.Q, *u.R
	} else {
		return nil, reject(http.StatusUnprocessableEntity, "unit has no known position; cannot determine departure hex")
	}

	// Amphibious assault: a laden galley ordered against a coastal settlement it
	// does not own cannot enter the land hex, so it sails to the adjacent sea hex
	// and lands its cargo on the beach. The arrival handler resolves the storming
	// with the cargo's strength (combat.resolveAmphibiousAssault). Detected here so
	// the ship is routed to the offshore hex and tagged intent=assault.
	targetQ, targetR := o.TargetQ, o.TargetR
	assaultLanding := false
	if unit.CategoryOf(u.Type) == unit.CategoryNaval && u.CargoUnitID != nil {
		var settOwner uuid.UUID
		var settCoastal bool
		if sErr := pool.QueryRow(ctx,
			`SELECT s.owner_id, COALESCE(p.coastal, false)
			 FROM provinces p JOIN settlements s ON s.province_id = p.id
			 WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3 AND s.state = 'active'`,
			o.WorldID, targetQ, targetR,
		).Scan(&settOwner, &settCoastal); sErr == nil && settOwner != o.PlayerID && settCoastal {
			seaQ, seaR, foundSea, seaErr := province.NearestSeaNeighbor(ctx, pool, o.WorldID, targetQ, targetR)
			if seaErr != nil {
				return nil, reject(http.StatusInternalServerError, "could not resolve landing hex")
			}
			if !foundSea {
				return nil, reject(http.StatusUnprocessableEntity,
					"that settlement has no adjacent sea hex to land on")
			}
			assaultLanding = true
			targetQ, targetR = seaQ, seaR
		}
	}

	// Target hex must exist on this world's map.
	var destTerrain string
	if err := pool.QueryRow(ctx,
		`SELECT terrain FROM map_tiles WHERE world_id = $1 AND q = $2 AND r = $3`,
		o.WorldID, targetQ, targetR,
	).Scan(&destTerrain); err != nil {
		return nil, reject(http.StatusNotFound, "target hex not found")
	}

	// Fas 2f: colonize-in-place. A field-positioned land unit (already on the
	// map, no settlement) may found a colony on the exact empty hex it occupies,
	// without marching one hex out and back. Target == its own hex. This
	// deliberately bypasses FindPath (a zero-distance query it was never built
	// to answer) and settles on the next tick via the normal arrival→foundColony
	// path. The colonize preconditions below (land unit, empty target,
	// settlement cap) still apply.
	colonizeInPlace := o.Intent == "colonize" && u.SettlementID == nil &&
		targetQ == originQ && targetR == originR

	// Sanity: cannot march to own hex (colonize-in-place, above, is the exception).
	if !colonizeInPlace && targetQ == originQ && targetR == originR {
		return nil, reject(http.StatusBadRequest, "cannot march to own hex")
	}

	// Intent validation: colonize and explore are the only supported intents.
	// Validate up front so the agent gets an actionable error instead of a
	// silent return-home at arrival.
	if o.Intent != "" && o.Intent != "colonize" && o.Intent != "explore" {
		return nil, reject(http.StatusBadRequest,
			"unknown march intent %q (must be \"colonize\" or \"explore\")", o.Intent)
	}
	// Del 2b: conquest choice. Empty defaults to "sack" (loot + raze); "annex" keeps
	// the settlement (capital→colony takeover). Validated up front, same reasoning
	// as intent above — actionable error instead of a silent default at arrival.
	if o.Mode != "" && o.Mode != "sack" && o.Mode != "annex" {
		return nil, reject(http.StatusBadRequest,
			"unknown capture mode %q (must be \"sack\" or \"annex\")", o.Mode)
	}
	captureMode := o.Mode
	if captureMode == "" {
		captureMode = "sack"
	}

	// FOW march rule (Timothy 2026-07-15/16, temenos_orderlopare_plan.md Fas 0):
	// a march can only be ordered onto land the Wanax has actually seen — the
	// API layer injects the knowledge check. It runs BEFORE the terrain and
	// settlement responses below so their error strings cannot leak what stands
	// on an unseen hex. Exempt: explore-intent (the sanctioned order INTO the
	// unknown) and colonize-in-place (own hex).
	if targetKnown != nil && !colonizeInPlace && o.Intent != "explore" {
		if !targetKnown(ctx, province.MapPosition{Q: targetQ, R: targetR}, destTerrain) {
			return nil, reject(http.StatusUnprocessableEntity,
				"none of your men have ever seen (%d,%d) — a march cannot be ordered into unknown land; send a scout first (march with intent \"explore\")",
				targetQ, targetR)
		}
	}

	// Explore: the unit marches to the target and returns home automatically —
	// it needs a home to return to, so it must currently be garrisoned at a
	// settlement (not already field-positioned).
	if o.Intent == "explore" && u.SettlementID == nil {
		return nil, reject(http.StatusUnprocessableEntity,
			"explore requires a unit currently garrisoned at a settlement (it needs a home to return to)")
	}
	if o.Intent == "colonize" {
		// Only land units found colonies — they become the new colony's garrison.
		if unit.CategoryOf(u.Type) != unit.CategoryLand {
			return nil, reject(http.StatusUnprocessableEntity, "only land units can found a colony")
		}
		// The target province must be unclaimed (no settlement). Best-effort pre-flight;
		// the arrival handler re-checks under lock and returns the unit home on a race.
		var existing int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM settlements s
			 JOIN provinces p ON p.id = s.province_id
			 WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3`,
			o.WorldID, targetQ, targetR,
		).Scan(&existing); err == nil && existing > 0 {
			return nil, reject(http.StatusUnprocessableEntity,
				"target hex already has a settlement — colonize requires an empty hex")
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
			capabilities.NewContext(ctx, pool, clk, o.WorldID, uuid.Nil, o.PlayerID, uuid.Nil))
		if !capReq.Satisfied {
			return nil, reject(http.StatusUnprocessableEntity, "%s", capReq.Hint)
		}
	}

	// Mountains are impassable.
	if destTerrain == "mountain_limestone" || destTerrain == "mountain_red" {
		return nil, reject(http.StatusUnprocessableEntity, "mountain terrain is impassable")
	}

	// Land units cannot enter sea hexes.
	isSea := destTerrain == "coastal_sea" || destTerrain == "deep_sea"
	if unit.CategoryOf(u.Type) == unit.CategoryLand && isSea {
		return nil, reject(http.StatusUnprocessableEntity, "land units cannot enter sea terrain")
	}

	// A* pathfinding: verify that a traversable route exists and derive the real
	// path cost for ETA. This catches the land-over-sea bug (target on land, but
	// the only route crosses water) and routes around mountains correctly.
	// Skipped for colonize-in-place: origin == target, so there is no route to
	// find and no distance to travel — the colony settles on the next tick.
	var moveHours float64
	if !colonizeInPlace {
		_, pathCost, pathOK, pathErr := province.FindPath(ctx, pool, o.WorldID,
			province.MapPosition{Q: originQ, R: originR},
			province.MapPosition{Q: targetQ, R: targetR},
			string(unit.CategoryOf(u.Type)),
		)
		if pathErr != nil {
			return nil, reject(http.StatusInternalServerError, "pathfinding error")
		}
		if !pathOK {
			hint := "a sea crossing needs a ship (embark), and mountains must be routed around"
			if unit.CategoryOf(u.Type) == unit.CategoryNaval {
				hint = "no sea route connects your harbour to that hex — land blocks the way"
			}
			return nil, reject(http.StatusUnprocessableEntity,
				"no passable route to (%d,%d) for this unit — %s", targetQ, targetR, hint)
		}

		// Calculate movement time.
		dist := province.HexDistance(
			province.MapPosition{Q: originQ, R: originR},
			province.MapPosition{Q: targetQ, R: targetR},
		)
		if dist == 0 {
			return nil, reject(http.StatusBadRequest, "target is the same hex as origin")
		}
		moveHours = pathCost
	}
	// Ship types vary in speed (Timothy 2026-07-09): war galley fastest,
	// merchantman slowest, galley between. Factor scales travel time (lower =
	// faster); tunable, lives in NavalSpeedFactor.
	moveHours *= NavalSpeedFactor(u.Type)
	// The nomadic host is the slowest thing on the map: half a spearman's speed,
	// i.e. DOUBLE its hours. Every other type is unaffected (factor 1.0).
	moveHours *= unit.MarchHoursFactorFor(u.Type)
	// Loaded ships move 1.5× slower.
	if u.CargoUnitID != nil {
		moveHours *= 1.5
	}

	now := clk.Now()
	var currentTick int
	_ = pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	travelTicks := max(1, int(math.Round(moveHours)))
	// arrives_at must mirror the real tick-scheduled arrival (travelTicks
	// ticks × real seconds/tick), NOT moveHours-as-hours: the map interpolates
	// the marching unit's position against this window, so a wall-clock value
	// (~24 min for a short hop) leaves the unit frozen at its origin until the
	// real tick arrival (6 s at TICK_SECONDS=6) teleports it home.
	arrivesAt := now.Add(time.Duration(travelTicks*tick.TickSeconds) * time.Second)

	// Atomic DB update: set unit to marching and schedule arrival event.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, reject(http.StatusInternalServerError, "could not begin transaction")
	}
	defer tx.Rollback(ctx)

	// Idempotency guard: re-read status inside the transaction with FOR UPDATE.
	var currentStatus string
	if err := tx.QueryRow(ctx,
		`SELECT status FROM units WHERE id = $1 FOR UPDATE`, o.UnitID,
	).Scan(&currentStatus); err != nil {
		return nil, reject(http.StatusNotFound, "unit not found in transaction")
	}
	if unit.Status(currentStatus) != unit.StatusGarrison && unit.Status(currentStatus) != unit.StatusPositioned {
		return nil, reject(http.StatusConflict, "unit status changed; march not sent")
	}

	// Build stance SET clause only when provided.
	var stanceArg *string
	if o.Stance != "" {
		s := o.Stance
		stanceArg = &s
	}

	// Colonize intent + optional name ride along on the unit so the arrival
	// handler can found a colony. NULL intent = plain march (cleared each dispatch).
	// Explore intent captures the unit's home settlement (about to be nulled
	// below) so the arrival handler can dispatch the return leg — see
	// combat.UnitArrivalHandler.exploreArrived.
	var intentArg, nameArg *string
	var homeSettlementArg *uuid.UUID
	if o.Intent != "" {
		intent := o.Intent
		intentArg = &intent
		if o.Intent == "colonize" && o.Name != "" {
			name := o.Name
			nameArg = &name
		}
		if o.Intent == "explore" {
			homeSettlementArg = u.SettlementID
		}
	}
	// Amphibious assault: the ship carries intent=assault to its offshore hex so
	// the arrival handler storms the adjacent coastal settlement with the cargo.
	if assaultLanding {
		a := "assault"
		intentArg = &a
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
		   home_settlement_id = $11,
		   capture_mode = $12,
		   depart_tick  = $13,
		   arrive_tick  = $14,
		   updated_at   = now()
		 WHERE id = $1`,
		o.UnitID, originQ, originR, targetQ, targetR, now, arrivesAt, stanceArg, intentArg, nameArg, homeSettlementArg, captureMode,
		currentTick, currentTick+travelTicks,
	); err != nil {
		return nil, reject(http.StatusInternalServerError, "could not update unit")
	}

	// Schedule the UnitArrival event.
	arrPayload := unit.ScheduledUnitArrivalPayload{
		UnitID:  o.UnitID,
		WorldID: o.WorldID,
	}
	if err := scheduler.EnqueueTickTx(ctx, tx, o.WorldID, events.ScheduledUnitArrival, arrPayload, currentTick+travelTicks); err != nil {
		return nil, reject(http.StatusInternalServerError, "could not schedule unit arrival")
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, reject(http.StatusInternalServerError, "could not commit march")
	}

	// Append domain event (outcome, not intention).
	_, _ = eventStore.Append(ctx, o.UnitID, events.StreamType(unit.StreamUnit), unit.EventUnitMarchOrdered,
		unit.UnitMarchOrderedPayload{
			UnitID:    o.UnitID,
			OriginQ:   originQ,
			OriginR:   originR,
			TargetQ:   targetQ,
			TargetR:   targetR,
			Stance:    o.Stance,
			DepartsAt: now.Format(time.RFC3339),
			ArrivesAt: arrivesAt.Format(time.RFC3339),
		},
		o.WorldID, nil,
	)

	return &MarchStarted{
		UnitID:        o.UnitID,
		DepartsAt:     now,
		ArrivesAt:     arrivesAt,
		ArrivalTick:   currentTick + travelTicks,
		DurationTicks: travelTicks,
		OriginQ:       originQ,
		OriginR:       originR,
		TargetQ:       targetQ,
		TargetR:       targetR,
	}, nil
}
