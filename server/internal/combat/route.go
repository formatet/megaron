package combat

// route.go: the sparad väg (megaron_plan_rorelse_sparad_vag.md, slice 2a).
// Every march-dispatching write site builds a StoredRoute from the path and
// per-hex costs it ALREADY has from province.FindPath/StepHoursDB and saves
// it in the same UPDATE as the march itself (R5). Every reader that wants
// "where is this unit right now" checks LoadActiveRoute first and only falls
// back to the old re-search (province.InterpolatePosition/InterpolateAlongPath)
// when no route applies (invariant 2: an absent/stale route is never a bug,
// just the old code, unchanged).

import (
	"context"
	"fmt"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"math"
	"time"

	"formatet/megaron/server/internal/movement"
	"formatet/megaron/server/internal/province"
)

// StoredRoute is the shared saved physical march representation.
type StoredRoute = movement.StoredRoute

// BuildRoute turns a freshly-found path and its per-step costs (from
// province.TileGraph.StepHours/StepHoursDB, over that SAME path — never a
// re-search) into a StoredRoute. ok=false when there is nothing worth
// storing (fewer than two hexes — colonize-in-place, origin==target) or the
// result fails movement.Move's own structural/overflow validation; callers
// save NULL in either case, and readers fall back to the old code (R5).
func BuildRoute(path []province.MapPosition, stepHours []float64, startTick, endTick int) (StoredRoute, bool) {
	if len(path) < 2 || len(stepHours) != len(path)-1 {
		return StoredRoute{}, false
	}
	hexes := make([][2]int, len(path))
	for i, p := range path {
		hexes[i] = [2]int{p.Q, p.R}
	}
	costs := make([]int64, len(stepHours))
	for i, h := range stepHours {
		c := int64(math.Round(h * 1000))
		if c < 1 {
			c = 1
		}
		costs[i] = c
	}
	r := StoredRoute{StartTick: startTick, EndTick: endTick, Hexes: hexes, Costs: costs}
	if err := r.Move().Validate(); err != nil {
		return StoredRoute{}, false
	}
	return r, true
}

// LoadActiveRoute forwards to the shared decoder used by live vision.
func LoadActiveRoute(raw []byte, status string, departTick, arriveTick *int) (StoredRoute, bool) {
	return movement.LoadActiveRoute(raw, status, departTick, arriveTick)
}

// RoutePositionAt is where the unit is at Milli m along its saved route.
func RoutePositionAt(r StoredRoute, m int64, now ...time.Time) (province.MapPosition, error) {
	if r.Version == 2 && !r.DepartureAt.IsZero() && r.TickNanoseconds > 0 && len(now) > 0 {
		m = r.MilliAt(now[0])
	}
	h, err := movement.PositionAt(r.Move(), movement.Milli(m))
	if err != nil {
		return province.MapPosition{}, err
	}
	return province.MapPosition{Q: h.Q, R: h.R}, nil
}

// RouteEnterMilli returns, for each hex in r.Hexes, the Milli at which the
// unit enters it (R1, "val A"). For i>=1 that is b_{i-1} from
// movement.Boundaries. Hexes[0] (the start hex) is left INSTANTLY at b_0
// (val A: the start hex costs nothing to be in, it is left at departure) —
// there is no later instant at which the unit is still meaningfully "in" it,
// so its own entry time is reported as the march's StartTick*1000 (b_0
// itself): the earliest instant anything about this route is defined, and
// exactly the value the pre-existing frac-based code already used for the
// start hex (frac=0 → departsAt). Used by messenger.InterceptAlongPathRoute
// (R6.d).
func RouteEnterMilli(r StoredRoute) ([]int64, error) {
	bounds, err := movement.Boundaries(r.Move())
	if err != nil {
		return nil, err
	}
	out := make([]int64, len(r.Hexes))
	out[0] = int64(bounds[0])
	for i := 1; i < len(out); i++ {
		if r.Version == 2 {
			out[i] = int64(bounds[i])
		} else {
			out[i] = int64(bounds[i-1])
		}
	}
	return out, nil
}

// BuildRouteAt is the clocked route writer. Legacy BuildRoute remains for old
// stored journeys and fixtures; all new physical unit voyages use version 2.
func BuildRouteAt(ctx context.Context, db province.Queryer, worldID uuid.UUID, path []province.MapPosition, costs []float64, start, end int, now time.Time) (StoredRoute, bool) {
	r, ok := BuildRoute(path, costs, start, end)
	if !ok {
		return r, false
	}
	rows, err := db.Query(ctx, `SELECT current_tick,last_tick_at FROM worlds WHERE id=$1`, worldID)
	if err != nil {
		return StoredRoute{}, false
	}
	defer rows.Close()
	var a tick.Anchor
	if !rows.Next() || rows.Scan(&a.Tick, &a.At) != nil {
		return StoredRoute{}, false
	}
	dispatch := a.ExactMilliAt(now)
	if dispatch < int64(start)*1000 {
		dispatch = int64(start) * 1000
	}
	c := movement.NewRouteClock(dispatch, a.WallAt(dispatch), time.Duration(tick.TickSeconds)*time.Second, end-start)
	r = c.Route(r.Hexes, r.Costs)
	return r, true
}

// RouteWallTimes converts saved millitick endpoints using the world's anchor.
func RouteWallTimes(ctx context.Context, db province.Queryer, worldID uuid.UUID, r StoredRoute) (time.Time, time.Time, error) {
	if r.Version == 2 && !r.DepartureAt.IsZero() && r.TickNanoseconds > 0 {
		return r.DepartureAt, r.WallAt(int64(r.EndTick)*1000 + r.OffsetMilli), nil
	}
	rows, err := db.Query(ctx, `SELECT current_tick,last_tick_at FROM worlds WHERE id=$1`, worldID)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	defer rows.Close()
	var a tick.Anchor
	if !rows.Next() {
		return time.Time{}, time.Time{}, fmt.Errorf("missing route anchor")
	}
	if err = rows.Scan(&a.Tick, &a.At); err != nil {
		return time.Time{}, time.Time{}, err
	}
	return a.WallAt(int64(r.StartTick)*1000 + r.OffsetMilli), a.WallAt(int64(r.EndTick)*1000 + r.OffsetMilli), nil
}
