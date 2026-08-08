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
