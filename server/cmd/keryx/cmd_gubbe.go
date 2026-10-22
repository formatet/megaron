package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Shared types and helpers for the gubbe-placement command family (P5:
// megaron_plan_fysisk_gubbemodell.md — city/place/staff/idle/brief). All five
// read the same GET .../placement-options payload, so the wire shape lives
// here once instead of being redeclared per file.

type placementGood struct {
	StaffingOnly   bool    `json:"staffing_only"`
	GoodKey        string  `json:"good_key"`
	RatePerTick    float64 `json:"rate_per_tick"`
	Cap            *int    `json:"cap,omitempty"` // nil = uncapped (grain — placementYield's exemption)
	Placed         int     `json:"placed"`
	PlacedOrdinals []int   `json:"placed_ordinals,omitempty"`
	MarginalYield  float64 `json:"marginal_yield"`
}

// placementHexBuilding is the ONE hex-bound production building (if any)
// actually standing on this hex — farm/mine/lumbermill/stonequarry
// (province.HexBoundBuildings, megaron_plan_byggnad_pa_hex.md §A). Nil when
// the hex is empty.
type placementHexBuilding struct {
	Type          string `json:"type"`
	Level         int    `json:"level"`
	UpgradeEffect string `json:"upgrade_effect,omitempty"` // server-formatted; absent at max level
}

// placementValidHex is one entry of valid_hexes_for_building[type]; Effect is
// the server-formatted line (megaron_plan_byggnadsregeln.md) — never computed here.
type placementValidHex struct {
	Q      int    `json:"q"`
	R      int    `json:"r"`
	Effect string `json:"effect"`
}

type placementHex struct {
	RiverName  string                `json:"river_name,omitempty"`
	HexQ       int                   `json:"hex_q"`
	HexR       int                   `json:"hex_r"`
	HexOrdinal int                   `json:"hex_ordinal"`
	Terrain    string                `json:"terrain"`
	Building   *placementHexBuilding `json:"building,omitempty"`
	Goods      []placementGood       `json:"goods"`

	// Shared catchment (megaron_plan_delad_catchment.md): set only on a hex
	// another city holds (has gubbar on). Takeable = the caller has a unit in
	// fortify/sentry there and the holder has none.
	HeldBy       string                `json:"held_by,omitempty"`
	Takeable     bool                  `json:"takeable,omitempty"`
	HeldWorkers  int                   `json:"held_workers,omitempty"`
	HeldBuilding *placementHexBuilding `json:"held_building,omitempty"`
}

// heldHexNote is the line `city` appends to a hex another city holds: who
// holds it and, when the caller can take it, what taking does. "" when the
// hex is not held.
func heldHexNote(h placementHex) string {
	if h.HeldBy == "" {
		return ""
	}
	note := "  held by " + h.HeldBy + " — already worked by another city"
	if h.Takeable {
		note += fmt.Sprintf("  TAKEABLE — place a worker to take it (%d workers go home", h.HeldWorkers)
		if h.HeldBuilding != nil {
			note += fmt.Sprintf(", its %s becomes yours", h.HeldBuilding.Type)
		}
		note += ")"
	}
	return note
}

type placementBuilding struct {
	BuildingType string          `json:"building_type"`
	Level        int             `json:"level"`
	Goods        []placementGood `json:"goods"`
	GodID        string          `json:"god_id,omitempty"` // a temple's god (one temple per god)
	Name         string          `json:"name,omitempty"`
}

type placementOptionsResp struct {
	Hexes       []placementHex      `json:"hexes"`
	Buildings   []placementBuilding `json:"buildings"`
	TotalGubbar int                 `json:"total_gubbar"`
	PoolSize    int                 `json:"pool_size"`

	ValidHexesForBuilding map[string][]placementValidHex `json:"valid_hexes_for_building"`
}

// buildEffectLine returns the server's effect text for building `typ` on hex
// (q,r): upgrade_effect when that type already stands there, else the hex's
// `effect`. Empty when the server gave none.
func buildEffectLine(resp *placementOptionsResp, typ string, q, r int) string {
	for _, h := range resp.Hexes {
		if h.HexQ == q && h.HexR == r && h.Building != nil && h.Building.Type == typ {
			return h.Building.UpgradeEffect
		}
	}
	for _, v := range resp.ValidHexesForBuilding[typ] {
		if v.Q == q && v.R == r {
			return v.Effect
		}
	}
	return ""
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
	Summary         string
	Automatic       string
	HarvestBlessing string
	Required        int
	Placed          int
	SelfSufficient  bool
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
	return &foodStatus{Summary: cityFoodLine(sett), Required: int(req), Placed: int(placed), SelfSufficient: suff, HarvestBlessing: harvestBlessingLine(sett), Automatic: automaticFoodLine(sett)}, nil
}

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
	if g.StaffingOnly {
		return fmt.Sprintf("%-10s %d/%d temple workers", "devotion", g.Placed, *g.Cap)
	}
	if g.Cap == nil {
		return fmt.Sprintf("%-10s %-9s  (%d placed, next %s)", g.GoodKey, rate(g.RatePerTick), g.Placed, rate(g.MarginalYield))
	}
	if g.Placed >= *g.Cap {
		return fmt.Sprintf("%-10s %d/%d FULL", g.GoodKey, g.Placed, *g.Cap)
	}
	return fmt.Sprintf("%-10s %d/%-3d  (next %s)", g.GoodKey, g.Placed, *g.Cap, rate(g.MarginalYield))
}

const citizenScaleNote = "1 citizen = 100 people"

func occupiedGoodHint(opts *placementOptionsResp, good string) string {
	var rows []string
	for _, h := range opts.Hexes {
		if h.HeldBy != "" {
			continue
		}
		for _, g := range h.Goods {
			if g.GoodKey == good && g.Placed > 0 {
				label := "citizens"
				if g.Placed == 1 {
					label = "citizen"
				}
				rows = append(rows, fmt.Sprintf("#%d %s (%d %s)", h.HexOrdinal, good, g.Placed, label))
			}
		}
	}
	if len(rows) == 0 {
		return "No citizens placed on " + good + " in this city's known catchment; see keryx city."
	}
	return "Currently working " + good + ": " + strings.Join(rows, ", ") + "; use those hex numbers to remove citizens."
}

func citizenCount(n int) string {
	if n == 1 {
		return "1 citizen"
	}
	return fmt.Sprintf("%d citizens", n)
}
func automaticFoodLine(sett map[string]any) string {
	a, ok := sett["automatic_food"].(map[string]any)
	if !ok {
		return ""
	}
	base, _ := a["nearjord_grain_per_tick"].(float64)
	people, _ := a["remainder_people"].(float64)
	grain, _ := a["remainder_grain_per_tick"].(float64)
	fish, _ := a["remainder_fish_per_tick"].(float64)
	return fmt.Sprintf("Automatic food: %.2f grain + %.2f fish per day without placed citizens (nearby ground %.2f grain; %.0f people outside whole citizens: %.2f grain + %.2f fish, a fraction of the best food worker)", base+grain, fish, base, people, grain, fish)
}

func hexOrdinal(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(s), "#"))
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid hex ordinal %q", s)
	}
	return n, nil
}

// Ordinals resolve only through the server's known catchment menu; no client
// ring geometry or unseen coordinates are invented.
func resolveHexFlag(c *Client, world, prov, value string) (q, r, ordinal int, err error) {
	if strings.Contains(value, ",") {
		q, r, err = parseHexFlag(value)
		return
	}
	ordinal, err = hexOrdinal(value)
	if err != nil {
		return
	}
	opts, e := fetchPlacementOptions(c, world, prov)
	if e != nil {
		err = e
		return
	}
	for _, h := range opts.Hexes {
		if h.HexOrdinal == ordinal {
			return h.HexQ, h.HexR, ordinal, nil
		}
	}
	return 0, 0, 0, fmt.Errorf("hex #%d is not in this city's known catchment; see keryx city", ordinal)
}
