package movement

import (
	"encoding/json"
	"time"
)

// StoredRoute is units.march_route's JSON shape (R1). Costs[i] is the cost
// of entering Hexes[i+1] (movement.Move's own convention), in thousandths of
// a terrain-hour.
type StoredRoute struct {
	DepartureAt     time.Time `json:"departure_at,omitempty"`
	TickNanoseconds int64     `json:"tick_nanoseconds,omitempty"`
	Version         int       `json:"version,omitempty"`
	OffsetMilli     int64     `json:"offset_milli,omitempty"`
	StartTick       int       `json:"start_tick"`
	EndTick         int       `json:"end_tick"`
	Hexes           [][2]int  `json:"hexes"`
	Costs           []int64   `json:"costs"`
}

// Move converts a StoredRoute to movement's pure Move shape.
func (r StoredRoute) Move() Move {
	hexes := make([]Hex, len(r.Hexes))
	for i, h := range r.Hexes {
		hexes[i] = Hex{Q: h[0], R: h[1]}
	}
	return Move{StartTick: r.StartTick, EndTick: r.EndTick, Hexes: hexes, Costs: r.Costs, Version: r.Version, OffsetMilli: r.OffsetMilli}
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
	if status != "marching" || len(raw) == 0 || departTick == nil || arriveTick == nil {
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

// MilliAt is the saved route's clock, independent of the world worker's poll
// phase. The world anchor establishes this mapping once at dispatch.
func (r StoredRoute) MilliAt(now time.Time) int64 {
	return int64(r.StartTick)*1000 + r.OffsetMilli + int64(now.Sub(r.DepartureAt)/(time.Duration(r.TickNanoseconds)/1000))
}
func (r StoredRoute) WallAt(at int64) time.Time {
	return r.DepartureAt.Add(time.Duration(at-int64(r.StartTick)*1000-r.OffsetMilli) * (time.Duration(r.TickNanoseconds) / 1000))
}

// RouteClock is the movement-owned clock shared by frozen journeys. Geometry
// stays in its original catalogue; it is never copied into another JSON path.
type RouteClock struct {
	DepartureAt     time.Time `json:"departure_at"`
	TickNanoseconds int64     `json:"tick_nanoseconds"`
	Version         int       `json:"version"`
	OffsetMilli     int64     `json:"offset_milli"`
	StartTick       int       `json:"start_tick"`
	EndTick         int       `json:"end_tick"`
}

// NewRouteClock anchors the unchanged rounded duration at actual dispatch.
func NewRouteClock(dispatch int64, at time.Time, cadence time.Duration, ticks int) RouteClock {
	// PostgreSQL projections and deadlines store microsecond timestamps.
	// Freeze at that precision so reading an ETA cannot precede its route.
	at = at.Truncate(time.Microsecond)
	return RouteClock{DepartureAt: at, TickNanoseconds: int64(cadence), Version: 2, OffsetMilli: dispatch % 1000, StartTick: int(dispatch / 1000), EndTick: int(dispatch/1000) + ticks}
}
func (c RouteClock) Route(hexes [][2]int, costs []int64) StoredRoute {
	return StoredRoute{DepartureAt: c.DepartureAt, TickNanoseconds: c.TickNanoseconds, Version: c.Version, OffsetMilli: c.OffsetMilli, StartTick: c.StartTick, EndTick: c.EndTick, Hexes: hexes, Costs: costs}
}
func (r StoredRoute) Clock() RouteClock {
	return RouteClock{r.DepartureAt, r.TickNanoseconds, r.Version, r.OffsetMilli, r.StartTick, r.EndTick}
}
func (r StoredRoute) Position(now time.Time) (Hex, error) {
	if len(r.Hexes) == 1 {
		return Hex{Q: r.Hexes[0][0], R: r.Hexes[0][1]}, nil
	}
	return PositionAt(r.Move(), Milli(r.MilliAt(now)))
}
