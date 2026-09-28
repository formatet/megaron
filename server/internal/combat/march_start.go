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
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"

	"formatet/megaron/server/internal/capabilities"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MarchOrder is one march command against one unit.
type MarchOrder struct {
	WorldID  uuid.UUID
	PlayerID uuid.UUID
	UnitID   uuid.UUID
	TargetQ  int
	TargetR  int
	Stance   string // optional; fortify|storm|sentry — persisted for C5
	Intent   string // optional; "" = plain march, "colonize"/"explore"/"patrol"/"land"/"passage"/"pickup"
	Name     string // optional colony name (used with intent=colonize, or intent=land + CargoIntent=colonize)
	Mode     string // optional; "" = sack (default) | "annex"
	// CargoIntent is R1's (megaron_plan_skeppsuppdrag_landsatt.md) optional
	// grounding order for the cargo a "land" mission puts ashore: "" (just
	// land) or "colonize" (found a colony on arrival, no further order
	// needed). Only meaningful when Intent == "land".
	CargoIntent string
	// PassageMessengerID is R1's (megaron_plan_ordna_passage.md, 3b-3) required
	// payload for Intent == "passage": the messenger this ship is being sent to
	// carry (outbound) or fetch (pickup). TargetQ/TargetR for this intent is
	// the disembark hex api/handlers.ArrangePassage already resolved (R2) —
	// this field only carries which runner the ship is doing it for.
	PassageMessengerID *uuid.UUID
	// PickupUnitID/PickupWaitTicks are R1's (megaron_plan_hamta_hem.md, slice
	// 2b) required payload for Intent == "pickup": the unit this ship is
	// being sent to fetch, and how many ticks it waits off the shore for that
	// unit before turning for home regardless. TargetQ/TargetR for this
	// intent is the shore hex api/handlers' pickup handler already resolved
	// (R1) — the unit's own hex if it already stands on a reachable shore,
	// otherwise the nearest open shore found the same way
	// resolveOutboundDisembark finds one for passage.
	PickupUnitID    *uuid.UUID
	PickupWaitTicks int
}

// OrderReject is a game-rule validation failure with the HTTP status the API
// layer should answer with; the delivery handler maps it to an OrderFailed
// notice. Shared by every order verb (march, stance, …).
type OrderReject struct {
	Status int
	Reason string
}

func (e *OrderReject) Error() string { return e.Reason }

func reject(status int, format string, args ...any) *OrderReject {
	return &OrderReject{Status: status, Reason: fmt.Sprintf(format, args...)}
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
	// CarriedSilver is the colonist purse this column left with, and
	// PurseShortfall is how much of it the mother city could not afford. Both
	// belong in the dispatch response rather than only in the colony's balance
	// later: a Wanax has to learn that the expedition set out broke BEFORE it
	// arrives, not when the new city cannot pay its garrison.
	CarriedSilver  float64
	PurseShortfall float64
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

// crewSpeedMax is CrewSpeedFactor's ceiling: an empty-benches hull sails at
// this multiple of a fully crewed one's time. STRAWMAN tal, same status as
// NavalSpeedFactor's 0.6/1.4 — kalibreras i soak, inte här
// (megaron_plan_skeppsfart_besattning.md §4).
const crewSpeedMax = 2.0

// CrewSpeedFactor: a shorthanded hull rows slower. 1.0 at full crew, rising
// continuously toward crewSpeedMax as the benches empty — no threshold, no
// minimum-crew hard stop. A ship with crew 1 still sails, just slowly
// (Timothy's decision, §4: a hard stranding gate would lock an absent
// Wanax's ship on the map forever, which is exactly what the asynchronicity
// gate forbids). Land units (CrewFor == 0) are exempt regardless of the crew
// column's value.
func CrewSpeedFactor(t unit.Type, crew int) float64 {
	full := unit.CrewFor(t)
	if full <= 0 {
		return 1.0 // land unit
	}
	if crew >= full {
		return 1.0
	}
	if crew < 0 {
		crew = 0
	}
	short := 1 - float64(crew)/float64(full)
	return 1 + short*(crewSpeedMax-1)
}

// TravelFactor is the complete multiplier applied to a path's raw tick cost:
// hull speed by ship type, the host's halved march hours, the crew's
// manning level, and the laden penalty. ONE home for all five legs (dispatch,
// arrival, recall, redirect, damaged return) — they diverged once already
// (unit_arrival.go's missing mirror, see its doc comment) — so a future term
// (e.g. hull) is added here once instead of in five places that will drift
// again.
func TravelFactor(t unit.Type, crew int, laden bool) float64 {
	f := NavalSpeedFactor(t) * unit.MarchHoursFactorFor(t) * CrewSpeedFactor(t, crew)
	if laden {
		f *= 1.5
	}
	return f
}

// StartMarch validates and executes one march order atomically. On success the
// unit is marching and its UnitArrival is scheduled. Any *OrderReject return
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

	// A disbanded unit no longer exists — most often a stale Nomadic Host id used
	// after founding (the host dissolves into its metropolis), but also a unit lost
	// in battle or to starvation. Say so plainly instead of the bare "status is
	// 'disbanded'" that read as a system error to players marching an old host id.
	if u.Status == unit.StatusDisbanded {
		return nil, reject(http.StatusUnprocessableEntity,
			"that unit no longer exists — it has been disbanded (a Nomadic Host becomes its metropolis once founded; a defeated or starved unit dissolves into the populace). Order your capital's garrison instead.")
	}

	// Must be garrisoned or positioned (positioned = on map without a settlement,
	// e.g. landed on empty hex; it must be able to march back or onward).
	if u.Status != unit.StatusGarrison && u.Status != unit.StatusPositioned {
		// A unit already marching can't start a second march — but it isn't
		// stuck: redirect sends a courier that turns it onto a new course
		// without waiting for it to arrive first. Name that verb instead of
		// leaving the player at "it says no" (megaron_plan_fyra_smaslices §4b).
		// Surface-neutral wording (Timothy 2026-09-25): this message reaches
		// web players too, who have no keryx — the old text named only the
		// CLI's flag syntax. Web's redirect surfaces are the march-menu
		// right-click and War → Army's Redirect button.
		if u.Status == unit.StatusMarching {
			return nil, reject(http.StatusUnprocessableEntity,
				"unit is already marching and cannot start a new march; redirect it to a new destination instead (right-click the map, or keryx redirect)")
		}
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
	if o.Intent != "land" && o.Intent != "passage" && unit.CategoryOf(u.Type) == unit.CategoryNaval && u.CargoUnitID != nil {
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

	// R1 (megaron_plan_skeppsuppdrag_landsatt.md): mission "land". o.TargetQ/R
	// here IS the chosen land hex (not yet the ship's real sailing target) —
	// resolved and validated before the generic destTerrain lookup below,
	// which from this point on tracks the ship's actual path, same as
	// assaultLanding's redirect above.
	landMission := o.Intent == "land"
	var landTargetQ, landTargetR int
	if landMission {
		if unit.CategoryOf(u.Type) != unit.CategoryNaval {
			return nil, reject(http.StatusUnprocessableEntity, "only a ship can be given a land mission")
		}
		// Distance 0 only (temenos_orderlopare_plan.md beslut 10): a land
		// mission is given from port, never carried by a Runner to a ship
		// already at sea — R3's RequireShipInPort already refuses any order to
		// a non-garrisoned ship at the two intake points; this repeats the
		// check so a direct StartMarch caller (tests, the delivery handler's
		// defensive re-validation) gets the same honest rejection.
		if u.Status != unit.StatusGarrison {
			return nil, reject(http.StatusUnprocessableEntity,
				"a land mission can only be given from a ship docked in its own port")
		}
		if u.CargoUnitID == nil {
			return nil, reject(http.StatusUnprocessableEntity,
				"a land mission needs cargo aboard — load a unit first")
		}
		if o.CargoIntent != "" && o.CargoIntent != "colonize" {
			return nil, reject(http.StatusBadRequest,
				"unknown cargo intent %q (must be \"colonize\" or omitted)", o.CargoIntent)
		}
		landTargetQ, landTargetR = o.TargetQ, o.TargetR
		var landTerrain string
		if err := pool.QueryRow(ctx,
			`SELECT terrain FROM map_tiles WHERE world_id = $1 AND q = $2 AND r = $3`,
			o.WorldID, landTargetQ, landTargetR,
		).Scan(&landTerrain); err != nil {
			return nil, reject(http.StatusNotFound, "target hex not found")
		}
		// Same "ofri mark" criteria as province.NearestUnclaimedLandNeighbor —
		// applied to the chosen hex directly rather than a neighbour of it.
		isSea := landTerrain == "coastal_sea" || landTerrain == "deep_sea" || landTerrain == "river" || landTerrain == "river_ford"
		isMountain := landTerrain == "mountain_limestone" || landTerrain == "mountain_red"
		var settledCount int
		_ = pool.QueryRow(ctx,
			`SELECT count(*) FROM provinces p JOIN settlements s ON s.province_id = p.id
			 WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3 AND s.state = 'active'`,
			o.WorldID, landTargetQ, landTargetR,
		).Scan(&settledCount)
		if isSea || isMountain || settledCount > 0 {
			return nil, reject(http.StatusUnprocessableEntity,
				"(%d,%d) is not open, unclaimed land — a land mission needs bare ground to put troops ashore on",
				landTargetQ, landTargetR)
		}
		seaQ, seaR, foundSea, seaErr := province.NearestSeaNeighbor(ctx, pool, o.WorldID, landTargetQ, landTargetR)
		if seaErr != nil {
			return nil, reject(http.StatusInternalServerError, "could not resolve a sea approach to the landing hex")
		}
		if !foundSea {
			return nil, reject(http.StatusUnprocessableEntity,
				"no open sea reaches (%d,%d) — pick a different landing site", landTargetQ, landTargetR)
		}
		// cargo_intent=colonize: run the exact same pre-flights a colonize
		// march runs at dispatch (settlement cap, catchment overlap, karens,
		// name-taken) against the TRUE land target — before the ship even
		// sails, not just at arrival.
		if o.CargoIntent == "colonize" {
			if rej := colonizeDispatchPreflight(ctx, pool, clk, o.WorldID, o.PlayerID, landTargetQ, landTargetR, o.Name); rej != nil {
				return nil, rej
			}
		}
		targetQ, targetR = seaQ, seaR
	}

	// R1 (megaron_plan_ordna_passage.md, 3b-3): mission "passage". Same shape
	// as "land" above — o.TargetQ/R is the disembark hex api/handlers.
	// ArrangePassage already chose (R2), not yet the ship's real sailing
	// target — but unlike a land mission, a passage disembark hex is very
	// often a SETTLED hex (the whole point is usually a city), so none of
	// land mission's "bare, unclaimed ground" checks apply here; only that it
	// is dry land with a sea approach.
	passageMission := o.Intent == "passage"
	var passageDisembarkQ, passageDisembarkR int
	if passageMission {
		if unit.CategoryOf(u.Type) != unit.CategoryNaval {
			return nil, reject(http.StatusUnprocessableEntity, "only a ship can be arranged for passage")
		}
		if u.Type == unit.TypeWarGalley {
			return nil, reject(http.StatusUnprocessableEntity,
				"a war galley cannot carry a runner — passage needs a galley or merchantman")
		}
		// Distance 0 only, same reasoning as the land mission above: passage is
		// arranged from a ship already docked in its own port, never carried to
		// a ship already at sea.
		if u.Status != unit.StatusGarrison {
			return nil, reject(http.StatusUnprocessableEntity,
				"passage can only be arranged for a ship docked in its own port")
		}
		if o.PassageMessengerID == nil {
			return nil, reject(http.StatusBadRequest, "passage needs a messenger to carry")
		}
		passageDisembarkQ, passageDisembarkR = o.TargetQ, o.TargetR
		var passageTerrain string
		if err := pool.QueryRow(ctx,
			`SELECT terrain FROM map_tiles WHERE world_id = $1 AND q = $2 AND r = $3`,
			o.WorldID, passageDisembarkQ, passageDisembarkR,
		).Scan(&passageTerrain); err != nil {
			return nil, reject(http.StatusNotFound, "disembark hex not found")
		}
		isSea := passageTerrain == "coastal_sea" || passageTerrain == "deep_sea" || passageTerrain == "river" || passageTerrain == "river_ford"
		isMountain := passageTerrain == "mountain_limestone" || passageTerrain == "mountain_red"
		if isSea || isMountain {
			return nil, reject(http.StatusUnprocessableEntity,
				"(%d,%d) is not dry land — passage needs a shore to put the runner ashore on",
				passageDisembarkQ, passageDisembarkR)
		}
		seaQ, seaR, foundSea, seaErr := province.NearestSeaNeighbor(ctx, pool, o.WorldID, passageDisembarkQ, passageDisembarkR)
		if seaErr != nil {
			return nil, reject(http.StatusInternalServerError, "could not resolve a sea approach to the disembark hex")
		}
		if !foundSea {
			return nil, reject(http.StatusUnprocessableEntity,
				"no open sea reaches (%d,%d) — passage is not possible there", passageDisembarkQ, passageDisembarkR)
		}
		targetQ, targetR = seaQ, seaR
	}

	// R1 (megaron_plan_hamta_hem.md, slice 2b): mission "pickup". Same shape
	// as "passage" above — o.TargetQ/R is the shore hex api/handlers' pickup
	// handler already resolved (R1), not yet the ship's real sailing target —
	// but unlike passage there is no war-galley restriction here: whether a
	// runner rides along at all (and therefore whether the galley restriction
	// applies) was already decided by the handler before this order was ever
	// built. A galley may be sent when the unit already stands on the shore.
	pickupMission := o.Intent == "pickup"
	var pickupShoreQ, pickupShoreR int
	if pickupMission {
		if unit.CategoryOf(u.Type) != unit.CategoryNaval {
			return nil, reject(http.StatusUnprocessableEntity, "only a ship can be sent to fetch a unit")
		}
		// Distance 0 only, same reasoning as land/passage above.
		if u.Status != unit.StatusGarrison {
			return nil, reject(http.StatusUnprocessableEntity,
				"a pickup mission can only be given from a ship docked in its own port")
		}
		if o.PickupUnitID == nil {
			return nil, reject(http.StatusBadRequest, "pickup needs the unit to fetch")
		}
		pickupShoreQ, pickupShoreR = o.TargetQ, o.TargetR
		var pickupTerrain string
		if err := pool.QueryRow(ctx,
			`SELECT terrain FROM map_tiles WHERE world_id = $1 AND q = $2 AND r = $3`,
			o.WorldID, pickupShoreQ, pickupShoreR,
		).Scan(&pickupTerrain); err != nil {
			return nil, reject(http.StatusNotFound, "shore hex not found")
		}
		isSea := pickupTerrain == "coastal_sea" || pickupTerrain == "deep_sea" || pickupTerrain == "river" || pickupTerrain == "river_ford"
		isMountain := pickupTerrain == "mountain_limestone" || pickupTerrain == "mountain_red"
		if isSea || isMountain {
			return nil, reject(http.StatusUnprocessableEntity,
				"(%d,%d) is not dry land — a pickup needs a shore to fetch from",
				pickupShoreQ, pickupShoreR)
		}
		seaQ, seaR, foundSea, seaErr := province.NearestSeaNeighbor(ctx, pool, o.WorldID, pickupShoreQ, pickupShoreR)
		if seaErr != nil {
			return nil, reject(http.StatusInternalServerError, "could not resolve a sea approach to the shore")
		}
		if !foundSea {
			return nil, reject(http.StatusUnprocessableEntity,
				"no open sea reaches (%d,%d) — pickup is not possible there", pickupShoreQ, pickupShoreR)
		}
		targetQ, targetR = seaQ, seaR
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

	// "sentry" is a deprecated alias for "patrol" (megaron_plan_cli_sanning):
	// the same word used to name two different orders — a land STANCE you
	// hold (unit.StanceSentry, untouched by this rename) and this naval march
	// INTENT, which patrols and then returns home on its own. Normalise the
	// alias here, once, before anything below reads o.Intent, so old
	// scripts/agents sending "sentry" keep working and every march dispatched
	// from here on is persisted (march_intent) under the one, unambiguous
	// name. unit_arrival.go still accepts the literal "sentry" too, for
	// in-flight units whose march_intent was written before this change.
	if o.Intent == "sentry" {
		o.Intent = "patrol"
	}

	// Intent validation: colonize, explore, patrol, land, passage and pickup
	// are the only supported intents. Validate up front so the agent gets an
	// actionable error instead of a silent return-home at arrival.
	if o.Intent != "" && o.Intent != "colonize" && o.Intent != "explore" && o.Intent != "patrol" && o.Intent != "land" && o.Intent != "passage" && o.Intent != "pickup" {
		return nil, reject(http.StatusBadRequest,
			"unknown march intent %q (must be \"colonize\", \"explore\", \"patrol\", \"land\", \"passage\" or \"pickup\")", o.Intent)
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
	// it needs a home to return to. A garrisoned unit's home is simply the
	// settlement it stands in. P7 soak fix (2026-07-19, "explore kräver
	// garrison — moment 22 för skepp till havs"): a unit that is ALREADY
	// field-positioned (no settlement — e.g. a ship that plain-marched out, or
	// one that just landed cargo via Unload) used to be flatly rejected here
	// with no way forward except sailing all the way home first just to issue
	// the same order. It still needs a real settlement to return to, so
	// instead of requiring it to be standing IN one right now, resolve the
	// player's nearest OWNED settlement as home — the same "nearest own city"
	// notion resolveOrderOrigin already uses for field-unit order dispatch
	// (api/handlers/unit.go). Only reject when the player owns none at all.
	var exploreHomeID *uuid.UUID
	if o.Intent == "explore" {
		if u.SettlementID != nil {
			exploreHomeID = u.SettlementID
		} else {
			nearest, found, hErr := nearestOwnedSettlement(ctx, pool, o.WorldID, o.PlayerID, originQ, originR)
			if hErr != nil {
				return nil, reject(http.StatusInternalServerError, "could not resolve a home settlement for explore")
			}
			if !found {
				return nil, reject(http.StatusUnprocessableEntity,
					"explore requires at least one settlement of yours to return to, and you currently hold none")
			}
			exploreHomeID = &nearest
		}
	}
	// Patrol: a naval sea order — a ship posts on a coastal_sea hex, holds there
	// (projecting FOW + intercepting passing enemy caravans via the sentry stance,
	// the physical posture — unchanged) and auto-returns home when its patrol
	// timer fires. Like explore it needs a home port to return to; unlike
	// explore the target must be a SEEN shallow-water hex (the FOW rule above
	// already applies — patrol is not in the explore exemption). Land units
	// hold position with the stance verb in place instead.
	if o.Intent == "patrol" {
		if unit.CategoryOf(u.Type) != unit.CategoryNaval {
			return nil, reject(http.StatusUnprocessableEntity,
				"only naval units can patrol a sea hex (land units hold position with the fortify/storm/sentry stance in place)")
		}
		if u.SettlementID == nil {
			return nil, reject(http.StatusUnprocessableEntity,
				"patrol requires a ship currently in port at a settlement (it needs a home to return to)")
		}
		// Deliberately NOT "|| destTerrain == river": patrol is a sea patrol. A
		// patrol standing in a 1-hex-wide river is not a patrol — there's no
		// water to project over (megaron_floden_plan.md §3, Timothy 2026-07-29).
		if destTerrain != "coastal_sea" {
			return nil, reject(http.StatusUnprocessableEntity,
				"patrol can only cover shallow coastal water (coastal_sea)")
		}
	}
	if o.Intent == "colonize" {
		// Only land units found colonies — they become the new colony's garrison.
		if unit.CategoryOf(u.Type) != unit.CategoryLand {
			return nil, reject(http.StatusUnprocessableEntity, "only land units can found a colony")
		}
		if rej := colonizeDispatchPreflight(ctx, pool, clk, o.WorldID, o.PlayerID, targetQ, targetR, o.Name); rej != nil {
			return nil, rej
		}
	}

	// R4 (megaron_plan_skeppsuppdrag_landsatt.md): a naval unit's plain march
	// (no intent) must end at a hex next to a settlement of its own —
	// otherwise it would drift to open sea and become a "positioned" ship no
	// order can ever reach again (R3), short of the one-time R6 sweep. Every
	// mission that already carries its own return leg (explore/patrol/land)
	// and assault (already redirected to the enemy's offshore hex above) is
	// exempt — each validates its own destination on its own terms.
	if o.Intent == "" && !assaultLanding && unit.CategoryOf(u.Type) == unit.CategoryNaval {
		if _, found, fErr := friendlySettlementAdjacent(ctx, pool, o.WorldID, o.PlayerID, targetQ, targetR); fErr != nil {
			return nil, reject(http.StatusInternalServerError, "could not check for a port at the destination")
		} else if !found {
			return nil, reject(http.StatusUnprocessableEntity,
				"ships need a mission when not sailing to a port: patrol, explore or land")
		}
	}

	// Mountains are impassable.
	if destTerrain == "mountain_limestone" || destTerrain == "mountain_red" {
		return nil, reject(http.StatusUnprocessableEntity, "mountain terrain is impassable")
	}

	// Land units cannot enter sea hexes — river is water too, a wall for land units
	// (megaron_floden_plan.md, Timothy 2026-07-29).
	isSea := destTerrain == "coastal_sea" || destTerrain == "deep_sea" || destTerrain == "river"
	if unit.CategoryOf(u.Type) == unit.CategoryLand && isSea {
		return nil, reject(http.StatusUnprocessableEntity, "land units cannot enter sea terrain")
	}

	// A* pathfinding: verify that a traversable route exists and derive the real
	// path cost for ETA. This catches the land-over-sea bug (target on land, but
	// the only route crosses water) and routes around mountains correctly.
	// Skipped for colonize-in-place: origin == target, so there is no route to
	// find and no distance to travel — the colony settles on the next tick.
	category := string(unit.CategoryOf(u.Type))
	var moveTicks float64
	var path []province.MapPosition
	if !colonizeInPlace {
		p, pathCost, pathOK, pathErr := province.FindPath(ctx, pool, o.WorldID,
			province.MapPosition{Q: originQ, R: originR},
			province.MapPosition{Q: targetQ, R: targetR},
			category,
		)
		path = p
		if pathErr != nil {
			return nil, reject(http.StatusInternalServerError, "pathfinding error")
		}
		if !pathOK {
			hint := "a sea crossing needs a ship — load the land unit aboard first: keryx unit load --ship <id> --unit <id> (ship and unit in the same settlement, both garrisoned, settlement coastal or with a harbour); mountains must be routed around"
			if unit.CategoryOf(u.Type) == unit.CategoryNaval {
				hint = "the sea is blocked by land — no open-water route reaches it from this settlement's shore (building a harbour will not change this)"
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
		moveTicks = pathCost
	}
	// Ship types vary in speed (Timothy 2026-07-09): war galley fastest,
	// merchantman slowest, galley between. The nomadic host is the slowest
	// thing on the map: half a spearman's speed. A shorthanded crew rows
	// slower still (Slice B). Loaded ships move 1.5× slower. All four live
	// in TravelFactor — the one home for every leg.
	moveTicks *= TravelFactor(u.Type, u.Crew, u.CargoUnitID != nil)

	now := clk.Now()
	var currentTick int
	_ = pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	travelTicks := max(1, int(math.Round(moveTicks)))
	// arrives_at must mirror the real tick-scheduled arrival (travelTicks
	// ticks × real seconds/tick), NOT moveTicks-as-hours: the map interpolates
	// the marching unit's position against this window, so a wall-clock value
	// (~24 min for a short hop) leaves the unit frozen at its origin until the
	// real tick arrival (6 s at TICK_SECONDS=6) teleports it home.
	arrivesAt := now.Add(time.Duration(travelTicks*tick.TickSeconds) * time.Second)

	// movement 2a, R1/R5: save the path FindPath already found (never a second
	// search) so a later read (recall/redirect, courier interception, the
	// owner's own map and keryx) never has to re-walk it. NULL for colonize-
	// in-place (no real path) and for a FindPath result too short to build a
	// route from (defensive — should not happen given pathOK was already
	// checked above).
	var marchRoute []byte
	if len(path) >= 2 {
		stepHours, shErr := province.StepHoursDB(ctx, pool, o.WorldID, path, category)
		if shErr == nil {
			if route, ok := BuildRoute(path, stepHours, currentTick, currentTick+travelTicks); ok {
				if raw, mErr := json.Marshal(route); mErr == nil {
					marchRoute = raw
				}
			}
		}
	}

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

	// The colonist purse (B3, mig 107). A founding used to MINT the colony's
	// starting silver; now the expedition carries it from the city that sent it.
	// Debited here, inside the same transaction that starts the march, so the
	// silver is never in two places at once.
	//
	// Not a hard gate: a city that cannot afford the full purse sends what it
	// has, and a colony can be founded with nothing. That is a real and legible
	// choice — a colony with no silver cannot pay upkeep — and the response
	// carries the figure so the Wanax learns it BEFORE the column leaves, not
	// when the city starves.
	var purse, purseShortfall float64
	if o.Intent == "colonize" && u.SupportSettlementID != nil {
		want := colonistPurse(ctx, tx, u.Size)
		if want > 0 {
			var have float64
			if err := tx.QueryRow(ctx,
				`SELECT GREATEST(0, settled(amount, rate, calc_tick))
				 FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver' FOR UPDATE`,
				*u.SupportSettlementID,
			).Scan(&have); err == nil {
				purse = math.Min(want, have)
				purseShortfall = want - purse
			}
		}
		if purse > 0 {
			if _, err := tx.Exec(ctx,
				`UPDATE settlement_goods
				    SET amount = GREATEST(0, settled(amount, rate, calc_tick) - $1),
				        calc_tick = current_world_tick()
				  WHERE settlement_id = $2 AND good_key = 'silver'`,
				purse, *u.SupportSettlementID,
			); err != nil {
				return nil, reject(http.StatusInternalServerError, "could not withdraw the colonist purse")
			}
			if _, err := tx.Exec(ctx,
				`UPDATE units SET carried_silver = carried_silver + $2 WHERE id = $1`,
				o.UnitID, purse,
			); err != nil {
				return nil, reject(http.StatusInternalServerError, "could not load the colonist purse")
			}
		}
	}

	// Skeppets proviant (megaron_plan_skeppsproviant.md §4, Timothy 2026-08-26).
	// Samma form som kolonistbörsen ovan — dras ur staden inne i den transaktion
	// som startar resan, så maten aldrig finns på två ställen samtidigt — men med
	// EN avgörande skillnad: börsen är mjuk (skicka det du har), provianten är
	// HÅRD. En koloni utan silver är ett legitimt, magert val; ett skepp utan mat
	// är ett skepp som dör till sjöss utan att spelaren kan svara, vilket är just
	// det asynkronitetsbrott mekaniken finns för att stänga.
	//
	// Landenheter rörs inte: proviantering är sjö, furagering är land.
	// ⚠️ Grinden är "ligger i hamn", INTE "har en hemstad". Två skäl, båda
	// funna av testsviten:
	//
	//  1. Ett skepp som redan står till sjöss har ofta ingen support_settlement
	//     (TestStartMarch_ExploreFromFieldPositionResolvesNearestOwnedHome). Ett
	//     hårt krav där STRANDAR skeppet permanent — det kan aldrig få en ny
	//     order. Det vore ett värre fel än det provianteringen lagar.
	//  2. Ett skepp till sjöss som HAR en support_settlement får inte proviantera
	//     ur den heller — då tar man ombord last från en hamn tjugo hexar bort,
	//     vilket är exakt den teleporterande logistik mekaniken finns för att ta
	//     bort, tillbaka genom en bakdörr.
	//
	// Man provianterar i hamn. Ett skepp som får en ny order till sjöss seglar på
	// det det har; tar det slut gäller undantagsgrenen i upkeep.
	var provisions float64
	if unit.CategoryOf(u.Type) == unit.CategoryNaval && !colonizeInPlace &&
		unit.Status(currentStatus) == unit.StatusGarrison && u.SupportSettlementID != nil {
		var cargoType string
		var cargoSize int
		if u.CargoUnitID != nil {
			// Kohorten ombord äter ur skeppets lager — därför provianteras den
			// med. En tom last ger nollvärden och VoyageRation hoppar över den.
			_ = tx.QueryRow(ctx,
				`SELECT type, size FROM units WHERE id = $1`, *u.CargoUnitID,
			).Scan(&cargoType, &cargoSize)
		}
		stationTicks := 0
		if o.Intent == "patrol" {
			stationTicks = SentryPatrolTicks
		}
		ration := VoyageRation(string(u.Type), u.Size, cargoType, cargoSize)
		provisions = VoyageProvisions(ration, travelTicks, stationTicks)

		if provisions > 0 {
			var have float64
			if err := tx.QueryRow(ctx,
				`SELECT GREATEST(0, settled(amount, rate, calc_tick))
				 FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain' FOR UPDATE`,
				*u.SupportSettlementID,
			).Scan(&have); err != nil {
				return nil, reject(http.StatusInternalServerError, "could not read the home port's granary")
			}
			if have < provisions {
				// Båda talen med flit: en ärlig brist säger hur stor den är, så
				// spelaren vet om hon ska vänta en dag eller bygga en åker.
				return nil, reject(http.StatusUnprocessableEntity,
					"not enough grain to provision the voyage — the home port holds %.0f, "+
						"the voyage needs %.0f (%.1f/day for %d days out, on station and home again)",
					have, provisions, ration, 2*travelTicks+stationTicks)
			}
			if _, err := tx.Exec(ctx,
				`UPDATE settlement_goods
				    SET amount = GREATEST(0, settled(amount, rate, calc_tick) - $1),
				        calc_tick = current_world_tick()
				  WHERE settlement_id = $2 AND good_key = 'grain'`,
				provisions, *u.SupportSettlementID,
			); err != nil {
				return nil, reject(http.StatusInternalServerError, "could not load the ship's provisions")
			}
			if _, err := tx.Exec(ctx,
				`UPDATE units SET provisions = provisions + $2 WHERE id = $1`,
				o.UnitID, provisions,
			); err != nil {
				return nil, reject(http.StatusInternalServerError, "could not stow the ship's provisions")
			}
		}
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
	// R1 (megaron_plan_skeppsuppdrag_landsatt.md): the chosen land hex and
	// optional cargo grounding order ride along on the SHIP (its own
	// target_q/target_r is overwritten below to the sea waypoint it actually
	// sails to) — see land_target_q/r + land_cargo_intent, mig 147.
	var landTargetQArg, landTargetRArg *int
	var landCargoIntentArg *string
	// R1 (megaron_plan_ordna_passage.md, 3b-3): the messenger this passage
	// mission was arranged for — see mig 149.
	var passageMessengerArg *uuid.UUID
	if o.Intent != "" {
		intent := o.Intent
		intentArg = &intent
		if o.Intent == "colonize" && o.Name != "" {
			name := o.Name
			nameArg = &name
		}
		if o.Intent == "explore" {
			// Resolved above — either the settlement the unit is standing in, or
			// (P7 fix) the nearest owned settlement when it was already field-positioned.
			homeSettlementArg = exploreHomeID
		}
		if o.Intent == "patrol" {
			// Capture the home port (about to be nulled below) so
			// sentryArrived's patrol timer can turn the ship for home.
			homeSettlementArg = u.SettlementID
		}
		if landMission {
			// Same reasoning as patrol above: capture the home port now, before
			// settlement_id is nulled, so the arrival handler's dispatchReturnHome
			// (reusing the explore_return machinery, R1) knows where to sail home to.
			homeSettlementArg = u.SettlementID
			lq, lr := landTargetQ, landTargetR
			landTargetQArg, landTargetRArg = &lq, &lr
			if o.CargoIntent != "" {
				ci := o.CargoIntent
				landCargoIntentArg = &ci
				if o.CargoIntent == "colonize" && o.Name != "" {
					name := o.Name
					nameArg = &name
				}
			}
		}
		if passageMission {
			// Same reasoning as patrol/land above: capture the home port now,
			// before settlement_id is nulled, so the release phase and
			// dispatchReturnHome know where to sail home to. The chosen
			// disembark hex reuses land_target_q/r (boardShipMissions already
			// reads those generically for any ship mission) — no new columns
			// for that half, only passage_messenger_id (mig 149) for R3/R4.
			homeSettlementArg = u.SettlementID
			lq, lr := passageDisembarkQ, passageDisembarkR
			landTargetQArg, landTargetRArg = &lq, &lr
			passageMessengerArg = o.PassageMessengerID
		}
		if pickupMission {
			// Same reasoning as passage above: capture the home port now,
			// before settlement_id is nulled, and reuse land_target_q/r for
			// the shore hex — boardShipMissions/BoardDispatchedShipRunner
			// already read those generically for any ship mission, so a
			// pickup order's bud (if one rides along) boards exactly like a
			// passage order's does.
			homeSettlementArg = u.SettlementID
			lq, lr := pickupShoreQ, pickupShoreR
			landTargetQArg, landTargetRArg = &lq, &lr
		}
	}
	// Amphibious assault: the ship carries intent=assault to its offshore hex so
	// the arrival handler storms the adjacent coastal settlement with the cargo.
	// homeSettlementArg captures the home port now (about to be nulled below),
	// same as patrol/land above — R5 (megaron_plan_skeppsuppdrag_landsatt.md):
	// the ship sails home on its own once its cargo is ashore, it no longer
	// simply sits at the beach.
	if assaultLanding {
		a := "assault"
		intentArg = &a
		homeSettlementArg = u.SettlementID
	}

	// R1 (megaron_plan_hamta_hem.md): the fetched unit's id and the chosen
	// wait, ridden on the ship for pickupArrived/HandlePickupTimeout to read.
	var pickupUnitArg *uuid.UUID
	var pickupWaitArg *int
	if pickupMission {
		pickupUnitArg = o.PickupUnitID
		w := o.PickupWaitTicks
		pickupWaitArg = &w
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
		   land_target_q = $15,
		   land_target_r = $16,
		   land_cargo_intent = $17,
		   passage_messenger_id = $18,
		   pickup_unit_id = $19,
		   pickup_wait_ticks = $20,
		   march_route  = $21,
		   updated_at   = now()
		 WHERE id = $1`,
		o.UnitID, originQ, originR, targetQ, targetR, now, arrivesAt, stanceArg, intentArg, nameArg, homeSettlementArg, captureMode,
		currentTick, currentTick+travelTicks, landTargetQArg, landTargetRArg, landCargoIntentArg, passageMessengerArg,
		pickupUnitArg, pickupWaitArg, marchRoute,
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

		CarriedSilver:  purse,
		PurseShortfall: purseShortfall,
	}, nil
}

// colonizeDispatchPreflight runs the four colonize dispatch checks — target
// unclaimed, catchment doesn't overlap a neighbour, settlement cap, chosen
// name free — against (targetQ,targetR) for playerID. Shared by a plain
// colonize march and R1's land mission (megaron_plan_skeppsuppdrag_landsatt.md)
// when its cargo_intent is "colonize": the ship's own category/cargo checks
// differ between the two callers, but once a land hex and a Wanax are known,
// "can a colony be founded here" is the exact same question both times.
// Best-effort pre-flight in every case; unit_arrival.go's foundColony gate
// (or, for the land mission, its own arrival-time re-check) is the
// authoritative fallback if the world changes mid-transit.
func colonizeDispatchPreflight(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, worldID, playerID uuid.UUID, targetQ, targetR int, name string) *OrderReject {
	// The target province must be unclaimed (no settlement). Exception
	// (megaron_plan_erovring.md S5): a settlement razed by sack-and-burn is
	// EXCLUDED here once its burnRecolonizeKaren has elapsed
	// (recolonizable_after_tick <= current tick) — the karens replaces what
	// used to be a PERMANENT block for every razed row. Every other dead
	// state (old sackSettlement's razed rows with no karens set, collapsed
	// rows) still blocks forever, unchanged.
	var existing int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM settlements s
		 JOIN provinces p ON p.id = s.province_id
		 WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3
		   AND NOT (s.state = 'razed' AND s.recolonizable_after_tick IS NOT NULL
		            AND s.recolonizable_after_tick <= current_world_tick())`,
		worldID, targetQ, targetR,
	).Scan(&existing); err == nil && existing > 0 {
		return reject(http.StatusUnprocessableEntity,
			"target hex already has a settlement — colonize requires an empty hex")
	}
	// Catchment overlap: no settlement, of ANY owner, may found where its
	// 7-hex catchment would overlap an existing, alive settlement's — the
	// delad-catchment-grind invariant (Timothy 2026-07-27/28: "finns delat
	// catchment kan staden inte grundas"). Never names the blocking
	// settlement here — FOW-aware naming needs the api/handlers
	// knownToPlayer model, which this package cannot import (G1 package
	// order); the generic phrasing is trivially FOW-safe.
	if conflict, cErr := province.SettlementCatchmentOverlap(ctx, pool, worldID, targetQ, targetR); cErr == nil && conflict != nil {
		needed := province.CatchmentClearanceHexes(province.HexDistance(
			province.MapPosition{Q: targetQ, R: targetR}, province.MapPosition{Q: conflict.Q, R: conflict.R}))
		return reject(http.StatusUnprocessableEntity,
			"this ground is already farmed by another settlement — its catchment overlaps yours here; move at least %d hex(es) farther away to found a settlement",
			needed)
	}
	// Settlement cap: a Wanax may hold at most maxSettlementsPerWanax active
	// settlements. Enforced at dispatch so the harness gets immediate
	// feedback and the colonising expedition never wastes the trip.
	//
	// Reuses capabilities' colonize checker's settlement-cap requirement
	// directly (temenos_capabilities.md Fas 3 anti-drift) — not the whole
	// canColonize verb, because its OTHER requirement ("a deployable land
	// unit garrisoned here") is aggregate-per-settlement and would wrongly
	// reject a unit already off any settlement mid-journey.
	capReq := capabilities.SettlementCapRequirement(
		capabilities.NewContext(ctx, pool, clk, worldID, uuid.Nil, playerID, uuid.Nil))
	if !capReq.Satisfied {
		return reject(http.StatusUnprocessableEntity, "%s", capReq.Hint)
	}
	// A chosen colony name must be free: names are how Wanaxes address each
	// other's cities. Caught at dispatch so the Wanax can pick another before
	// anything sets out; the arrival handler falls back to a generated name
	// if the name is claimed while the colonists are still on the road.
	if name != "" {
		if taken, nErr := province.SettlementNameIsTaken(ctx, pool, worldID, name); nErr == nil && taken {
			return reject(http.StatusConflict,
				"a settlement named %q already stands in this world — choose another name", name)
		}
	}
	return nil
}

// colonistPurse is what a colonising expedition tries to take with it: exactly
// the liquid silver the colony used to be seeded with out of thin air, so the
// colony's balance sheet is unchanged and only its SOURCE moves — from the
// world's faucet to the mother city's treasury. Priced against the population
// the colony will actually have (base + the column's own men), the same figure
// foundColony computes on arrival.
//
// Returns 0 rather than an error if grain's base value can't be read: an
// expedition that leaves without a purse is a worse outcome than a failed
// dispatch would be honest about, but silently minting is worse than both, and
// a colony can legitimately be founded penniless.
func colonistPurse(ctx context.Context, tx pgx.Tx, unitSize int) float64 {
	grainBaseValue, err := economy.GoodBaseValue(ctx, tx, "grain")
	if err != nil {
		slog.Error("colonist purse: load grain base value", "err", err)
		return 0
	}
	seed, _ := economy.GenesisSilverLiquid(economy.ColonyBaseFoundingPopulation+unitSize,
		grainBaseValue, economy.LoadSitosConfig())
	return seed
}

// nearestOwnedSettlement finds the player's active settlement closest to
// (q,r) — used by the P7 explore fix above to give a field-positioned unit a
// home to return to without requiring it to already be standing in one.
// found=false when the player owns no active settlement at all.
func nearestOwnedSettlement(ctx context.Context, pool province.Queryer, worldID, playerID uuid.UUID, q, r int) (uuid.UUID, bool, error) {
	rows, err := pool.Query(ctx,
		`SELECT s.id, p.map_q, p.map_r FROM settlements s
		 JOIN provinces p ON p.id = s.province_id
		 WHERE s.world_id = $1 AND s.owner_id = $2 AND s.state = 'active'`,
		worldID, playerID,
	)
	if err != nil {
		return uuid.Nil, false, err
	}
	defer rows.Close()
	var best uuid.UUID
	found := false
	bestDist := 0
	for rows.Next() {
		var sid uuid.UUID
		var sq, sr int
		if rows.Scan(&sid, &sq, &sr) != nil {
			continue
		}
		d := province.HexDistance(province.MapPosition{Q: sq, R: sr}, province.MapPosition{Q: q, R: r})
		if !found || d < bestDist {
			found = true
			bestDist = d
			best = sid
		}
	}
	return best, found, rows.Err()
}
