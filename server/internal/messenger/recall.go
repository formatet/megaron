package messenger

// Shared courier/messenger travel and interception helpers. The RecallArrival
// handler and its army-recall chain (RecallMarch → ScheduledRecallArrival →
// ScheduledArmyArrival, which had no consumer) were removed 2026-10-07; recall
// now travels as an order messenger (api/handlers.UnitHandler.Recall).

import (
	"context"
	"math"
	"time"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
)

// TicksPerHex is the travel speed of a messenger (ticks per hex). Shared by diplomatic
// messengers (api/handlers messenger send) and recall messengers so the rate lives in one place.
// A future "blessed messengers" rite would turn this into a multiplier-driven value (see thalassa_todo).
const TicksPerHex = 0.5

// MessengerTravelDuration returns the wall-clock travel time for a messenger over dist hexes,
// for the messengers.arrives_at DISPLAY column. Scheduling uses MessengerTravelTicks (1 tick =
// 1 game hour); converted through tick.RealUntil for the same tick/wall-clock reason —
// a fixed "1 game hour = 1 real hour" assumption drifts from the actual tick-driven delivery on
// any world not running the default 60 min/tick cadence, and TickMinutes' 1-minute floor drifts
// again on a sub-minute TICK_SECONDS dev cadence.
func MessengerTravelDuration(dist int) time.Duration {
	return tick.RealUntil(MessengerTravelTicks(dist), 0)
}

// MessengerTravelTicks returns the world-tick travel time for a messenger over dist hexes.
func MessengerTravelTicks(dist int) int {
	t := int(math.Round(float64(dist) * TicksPerHex))
	if t < 1 {
		return 1
	}
	return t
}

// CourierTravel returns the world-tick and wall-clock travel time for a
// runner from 'from' to 'to' (temenos_orderlopare_plan.md Fas 4): A*
// over the courier graph — land at half a land unit's terrain ticks (2×
// spearman speed), mountains routed around, rivers by boat
// (province.CourierSeaTicks, megaron_floden_plan.md — untouched by 3b-4).
// ok=false when no such route exists — most often because it needs the sea,
// which a courier may no longer cross on its own (megaron_plan_ordna_
// passage.md, slice 3b-4, R3: the old straight-line raklinjefallback is
// GONE). Every caller must handle !ok visibly (a 422 or a genuine error),
// never by guessing a travel time. One speed model for ALL messengers:
// diplomatic, recall/redirect and order runners alike (trade CARAVANS keep
// their own terrain-based journey planner).
func CourierTravel(ctx context.Context, db province.Queryer, worldID uuid.UUID, from, to province.MapPosition) (ticks int, dur time.Duration, ok bool, err error) {
	g, err := province.LoadTileGraph(ctx, db, worldID)
	if err != nil {
		return 0, 0, false, err
	}
	ticks, dur, ok = CourierTravelOnGraph(g, from, to)
	return ticks, dur, ok, nil
}

// CourierTravelOnGraph is CourierTravel over an already-loaded TileGraph —
// avoids re-querying every map_tile per call. Used by InterceptAlongPath,
// which evaluates many candidate hexes against the same courier origin within
// one request; loading the graph once instead of once per candidate is the
// difference between one DB round-trip and dozens. ok=false — see
// CourierTravel's own doc comment; no raklinjefallback here either.
func CourierTravelOnGraph(g province.TileGraph, from, to province.MapPosition) (ticks int, dur time.Duration, ok bool) {
	_, hours, pathOK := g.FindPath(from, to, province.CategoryCourier)
	if !pathOK {
		return 0, 0, false
	}
	t := int(math.Round(hours))
	if t < 1 {
		t = 1
	}
	return t, tick.RealUntil(t, 0), true
}

// InterceptAlongPath finds the earliest hex along a marching unit's
// already-validated outbound path (as returned by province.FindPath) where a
// courier departing courierOrigin at now can reach the unit BEFORE it does —
// an honest moving-target intercept, rather than aiming at the unit's
// position at the dispatch instant (temenos_orderlopare_plan.md interception
// fix, 2026-07-30). Aiming at that instant's snapshot is always stale: the
// courier takes time to reach even that point, and the unit keeps moving
// while it travels — precisely the silent "runner arrived, unit long gone"
// miss this replaces.
//
// Scans path indices from the start (skipping every hex the unit has already
// passed) to the last (the march's own destination). CategoryCourier runs at
// 2× a land unit's speed (sea legs at the flat CourierSeaTicks rate), so an
// intercept normally exists somewhere along the remaining path — the earlier
// on the path it is found, the sooner the order is delivered.
//
// ok=false means no hex on the remaining path — including the destination
// itself — is reachable before the unit passes it: courierOrigin is too far
// (or the remaining march too short) for any physically-honest courier to
// catch this unit. Callers must treat that as a genuine, visible dispatch
// failure; queuing a courier anyway would only reproduce the silent miss this
// function exists to prevent.
func InterceptAlongPath(
	g province.TileGraph, courierOrigin province.MapPosition, path []province.MapPosition,
	departsAt, arrivesAt, now time.Time,
) (target province.MapPosition, ok bool) {
	n := len(path)
	if n < 2 {
		return province.MapPosition{}, false
	}
	total := arrivesAt.Sub(departsAt)
	unitTimeAt := func(i int) time.Time {
		// unitTime is the instant the unit reaches path[i], on the same
		// index-fraction-of-total model province.InterpolatePosition uses for
		// "where is the unit right now" — the last hex is pinned to the exact
		// stored arrivesAt to avoid float round-trip drift.
		if i == n-1 {
			return arrivesAt
		}
		frac := float64(i) / float64(n-1)
		return departsAt.Add(time.Duration(frac * float64(total)))
	}
	return interceptScan(g, courierOrigin, path, unitTimeAt, now)
}

// InterceptAlongPathRoute is InterceptAlongPath for a marching unit with a
// gällande saved route (movement 2a, R6.d, megaron_plan_rorelse_sparad_vag.md):
// unitTime for path[i] comes from the route's own hex-entry times
// (combat.RouteEnterMilli, converted to wall clock via tick.Anchor.WallAt)
// instead of the even frac=i/(n-1) split — terrain-weighted, not index-weighted.
// path must be route.Hexes and enterAt[i] must be that same route's entry time
// for Hexes[i] (see InterceptCourierTargetRoute, which pairs them correctly);
// len(enterAt) must equal len(path). The rest of the logic — courier travel
// time per candidate hex, ties let through, no raklinje guess over open sea —
// is untouched, via the shared interceptScan.
func InterceptAlongPathRoute(
	g province.TileGraph, courierOrigin province.MapPosition, path []province.MapPosition,
	enterAt []time.Time, now time.Time,
) (target province.MapPosition, ok bool) {
	if len(path) < 2 || len(enterAt) != len(path) {
		return province.MapPosition{}, false
	}
	return interceptScan(g, courierOrigin, path, func(i int) time.Time { return enterAt[i] }, now)
}

// interceptScan is InterceptAlongPath/InterceptAlongPathRoute's shared scan:
// the earliest path[i] a courier from courierOrigin at now can reach no later
// than the unit itself does (unitTimeAt(i)), skipping any hex the unit has
// already passed and any hex the courier can only reach by crossing open sea
// on its own (CourierTravelOnGraph's ok=false).
func interceptScan(
	g province.TileGraph, courierOrigin province.MapPosition, path []province.MapPosition,
	unitTimeAt func(i int) time.Time, now time.Time,
) (target province.MapPosition, ok bool) {
	for i := 0; i < len(path); i++ {
		unitTime := unitTimeAt(i)
		if !unitTime.After(now) {
			continue // the unit has already passed (or is exactly at) this hex
		}
		// <= (not strict <): an exact tie — the courier's rounded-to-the-tick
		// ETA lands on the same instant the unit reaches this hex — is a real
		// 50/50 race at that tick (whichever scheduled event a poll happens to
		// process first), not a provable miss. Rejecting ties outright would
		// wrongly fail the ordinary "recall from the very city the unit
		// marched out of, right at the march's halfway point" case: a courier
		// at 2× a land unit's speed departing from the SAME origin always
		// needs exactly half the march's total duration to reach the
		// destination, which ties the remaining time exactly at the halfway
		// mark. Ties are let through; a genuine loss of that race still ends
		// in a visible OrderFailed (order_delivery.go), never a silent one.
		//
		// 3b-4 R4: a hex the courier can only reach by crossing open sea on
		// its own is no longer a candidate at all — CourierTravelOnGraph's
		// ok=false skips it, never approximates it with a raklinje guess.
		// Stance-pursuit's own aim (combat's stanceToMarchingUnit) already
		// targets the unit's destination and takes the real passage route
		// when a land intercept misses, exactly as before this slice.
		_, courierDur, courierOK := CourierTravelOnGraph(g, courierOrigin, path[i])
		if !courierOK {
			continue
		}
		if !now.Add(courierDur).After(unitTime) {
			return path[i], true
		}
	}
	return province.MapPosition{}, false
}

// InterceptCourierTarget loads the marching unit's outbound path and the
// world's terrain graph, then delegates to InterceptAlongPath. err is
// non-nil only for DB/scan failures; ok=false (err==nil) covers both "no
// route exists" (mirrors province.InterpolatePosition's own ok=false) and
// "a route exists but no courier can catch the unit on it" — see
// InterceptAlongPath's doc comment for the latter.
func InterceptCourierTarget(
	ctx context.Context, db province.Queryer, worldID uuid.UUID,
	courierOrigin, marchOrigin, marchTarget province.MapPosition, category string,
	departsAt, arrivesAt, now time.Time,
) (target province.MapPosition, ok bool, err error) {
	path, _, pathOK, err := province.FindPath(ctx, db, worldID, marchOrigin, marchTarget, category)
	if err != nil {
		return province.MapPosition{}, false, err
	}
	if !pathOK {
		return province.MapPosition{}, false, nil
	}
	g, err := province.LoadTileGraph(ctx, db, worldID)
	if err != nil {
		return province.MapPosition{}, false, err
	}
	t, ok := InterceptAlongPath(g, courierOrigin, path, departsAt, arrivesAt, now)
	return t, ok, nil
}

// InterceptCourierTargetRoute is InterceptCourierTarget for a marching unit
// with a gällande saved route (movement 2a, R6.d) — no path search at all
// (R1): the path and each hex's entry time come straight from route via
// combat.RouteEnterMilli + anchor.WallAt. err is non-nil only for a
// Boundaries/PositionAt computation failure, which should not happen for a
// route that already passed combat.LoadActiveRoute's own validation.
func InterceptCourierTargetRoute(
	ctx context.Context, db province.Queryer, worldID uuid.UUID,
	courierOrigin province.MapPosition, route combat.StoredRoute, anchor tick.Anchor, now time.Time,
) (target province.MapPosition, ok bool, err error) {
	path := make([]province.MapPosition, len(route.Hexes))
	for i, h := range route.Hexes {
		path[i] = province.MapPosition{Q: h[0], R: h[1]}
	}
	enterMilli, err := combat.RouteEnterMilli(route)
	if err != nil {
		return province.MapPosition{}, false, err
	}
	enterAt := make([]time.Time, len(enterMilli))
	for i, m := range enterMilli {
		enterAt[i] = anchor.WallAt(m)
	}
	g, err := province.LoadTileGraph(ctx, db, worldID)
	if err != nil {
		return province.MapPosition{}, false, err
	}
	t, ok := InterceptAlongPathRoute(g, courierOrigin, path, enterAt, now)
	return t, ok, nil
}
