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
	"encoding/json"
	"math"

	"formatet/megaron/server/internal/movement"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
)

// StoredRoute is units.march_route's JSON shape (R1). Costs[i] is the cost
// of entering Hexes[i+1] (movement.Move's own convention), in thousandths of
// a terrain-hour.
type StoredRoute struct {
	StartTick int      `json:"start_tick"`
	EndTick   int      `json:"end_tick"`
	Hexes     [][2]int `json:"hexes"`
	Costs     []int64  `json:"costs"`
}

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

// Move converts a StoredRoute to movement's pure Move shape.
func (r StoredRoute) Move() movement.Move {
	hexes := make([]movement.Hex, len(r.Hexes))
	for i, h := range r.Hexes {
		hexes[i] = movement.Hex{Q: h[0], R: h[1]}
	}
	return movement.Move{StartTick: r.StartTick, EndTick: r.EndTick, Hexes: hexes, Costs: r.Costs}
}

// LoadActiveRoute parses raw (units.march_route) and reports whether it is
// the saved route for the unit's CURRENT march (invariant 2): status must be
// "marching" and the route's own StartTick/EndTick must equal the unit's
// current depart_tick/arrive_tick — a route from an earlier march never
// matches a later one, so no write site needs to null the column out when a
// march ends. Any mismatch, nil/empty raw, unparseable JSON or a structurally
// invalid route returns ok=false: the caller falls back to the pre-existing
// code path, unchanged.
func LoadActiveRoute(raw []byte, status string, departTick, arriveTick *int) (StoredRoute, bool) {
	if status != string(unit.StatusMarching) || len(raw) == 0 || departTick == nil || arriveTick == nil {
		return StoredRoute{}, false
	}
	var r StoredRoute
	if err := json.Unmarshal(raw, &r); err != nil {
		return StoredRoute{}, false
	}
	if r.StartTick != *departTick || r.EndTick != *arriveTick {
		return StoredRoute{}, false
	}
	if err := r.Move().Validate(); err != nil {
		return StoredRoute{}, false
	}
	return r, true
}

// RoutePositionAt is where the unit is at Milli m along its saved route.
func RoutePositionAt(r StoredRoute, m int64) (province.MapPosition, error) {
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
	out[0] = int64(r.StartTick) * 1000
	for i := 1; i < len(out); i++ {
		out[i] = int64(bounds[i-1])
	}
	return out, nil
}
