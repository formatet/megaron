package movement

import "encoding/json"

// StoredRoute is units.march_route's JSON shape (R1). Costs[i] is the cost
// of entering Hexes[i+1] (movement.Move's own convention), in thousandths of
// a terrain-hour.
type StoredRoute struct {
	Version     int      `json:"version,omitempty"`
	OffsetMilli int64    `json:"offset_milli,omitempty"`
	StartTick   int      `json:"start_tick"`
	EndTick     int      `json:"end_tick"`
	Hexes       [][2]int `json:"hexes"`
	Costs       []int64  `json:"costs"`
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
