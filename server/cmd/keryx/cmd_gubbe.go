package main

import (
	"encoding/json"
	"fmt"
)

// Shared types and helpers for the gubbe-placement command family (P5:
// megaron_plan_fysisk_gubbemodell.md — city/place/staff/idle/brief). All five
// read the same GET .../placement-options payload, so the wire shape lives
// here once instead of being redeclared per file.

type placementGood struct {
	GoodKey        string  `json:"good_key"`
	RatePerTick    float64 `json:"rate_per_tick"`
	Cap            *int    `json:"cap,omitempty"` // nil = uncapped (grain — placementYield's exemption)
	Placed         int     `json:"placed"`
	PlacedOrdinals []int   `json:"placed_ordinals,omitempty"`
	MarginalYield  float64 `json:"marginal_yield"`
}

type placementHex struct {
	HexQ       int             `json:"hex_q"`
	HexR       int             `json:"hex_r"`
	HexOrdinal int             `json:"hex_ordinal"`
	Terrain    string          `json:"terrain"`
	Goods      []placementGood `json:"goods"`
}

type placementBuilding struct {
	BuildingType string          `json:"building_type"`
	Level        int             `json:"level"`
	Goods        []placementGood `json:"goods"`
}

type placementOptionsResp struct {
	Hexes       []placementHex      `json:"hexes"`
	Buildings   []placementBuilding `json:"buildings"`
	TotalGubbar int                 `json:"total_gubbar"`
	PoolSize    int                 `json:"pool_size"`
}

// fetchPlacementOptions calls GET .../placement-options and decodes it.
func fetchPlacementOptions(c *Client, worldID, provinceID string) (*placementOptionsResp, error) {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/placement-options", worldID, provinceID))
	if err != nil {
		return nil, err
	}
	var resp placementOptionsResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode placement-options: %w", err)
	}
	return &resp, nil
}

// foodStatus is the food-placement subset of the /provinces/{id} settlement
// payload (api/handlers/province.go's food_gubbar_required/placed/
// self_sufficient) — the same numbers `keryx status`'s grain line already
// shows via foodGubbarLine. `city` and `idle` read /placement-options, a
// different endpoint that has no food_gubbar_* fields, so this is a second,
// small client-side fetch of data the server already computes — no new
// server code (megaron_plan_omfordelningsmatningen.md: "add zero server
// code" was the instruction, and both fields already exist).
type foodStatus struct {
	Required       int
	Placed         int
	SelfSufficient bool
}

// fetchFoodStatus calls GET .../provinces/{id} and pulls out the food_gubbar_*
// fields. Returns nil (no error) when the settlement has no food figures yet
// — e.g. an unfounded province — so callers can silently skip the food line
// rather than surface a spurious error for a case status.go treats as normal.
func fetchFoodStatus(c *Client, worldID, provinceID string) (*foodStatus, error) {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s", worldID, provinceID))
	if err != nil {
		return nil, err
	}
	var p map[string]any
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("decode province status: %w", err)
	}
	sett, _ := p["settlement"].(map[string]any)
	if sett == nil {
		return nil, nil
	}
	req, ok := sett["food_gubbar_required"].(float64)
	if !ok {
		return nil, nil
	}
	placed, _ := sett["food_gubbar_placed"].(float64)
	suff, _ := sett["food_self_sufficient"].(bool)
	return &foodStatus{Required: int(req), Placed: int(placed), SelfSufficient: suff}, nil
}

// foodSurplus reports how many placed food (grain/fish) citizens exceed what
// the catchment needs, if any — 0 when self-sufficiency isn't established or
// there is no surplus. Pure, shared by `city` and `idle` so the two surfaces
// can never report two different surplus numbers for the same settlement.
func foodSurplus(fs *foodStatus) int {
	if fs == nil || !fs.SelfSufficient {
		return 0
	}
	if s := fs.Placed - fs.Required; s > 0 {
		return s
	}
	return 0
}

// foodSurplusMarker flags a food (grain/fish) hex or building row in `keryx
// city` as part of a citywide food surplus (see foodSurplus) — shared so the
// marker symbol and its explanation in the "Food:" line always match.
const foodSurplusMarker = "★"

// isFoodGood reports whether good_key is one of the two goods
// food_gubbar_placed counts (api/handlers/province.go: grain, fish — never
// settlement_labor, never economy.FoodGoods' wider diet-variety set).
func isFoodGood(goodKey string) bool {
	return goodKey == "grain" || goodKey == "fish"
}

// goodCell renders one good's occupancy for a table row: "grain +12.0/tick
// (2 placed, next +12.0/tick)" for an uncapped good, or "fish 1/1 FULL" /
// "stone 1/4 (next +6.0/tick)" for a capped one.
func goodCell(g placementGood) string {
	if g.Cap == nil {
		return fmt.Sprintf("%-10s %-9s  (%d placed, next %s)", g.GoodKey, rate(g.RatePerTick), g.Placed, rate(g.MarginalYield))
	}
	if g.Placed >= *g.Cap {
		return fmt.Sprintf("%-10s %d/%d FULL", g.GoodKey, g.Placed, *g.Cap)
	}
	return fmt.Sprintf("%-10s %d/%-3d  (next %s)", g.GoodKey, g.Placed, *g.Cap, rate(g.MarginalYield))
}
