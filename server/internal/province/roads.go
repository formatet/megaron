package province

import (
	"context"

	"github.com/google/uuid"
)

// Roads (megaron_plan_vagar.md V1). A road is an EDGE between two neighbouring
// hexes, built by an army, free for everyone to use. It scales the cost of
// ENTERING the edge's target hex by a per-category factor. There are no
// bridges: the road ends at a ford and fords are waded as before.

// CategoryCaravan is the price class of a land trade caravan. It behaves like
// "land" everywhere (passability, terrain cost) and differs only in its road
// factor. It is an internal pricing class; TradeJourney.Category stays "land".
const CategoryCaravan = "caravan"

// roadFactors is the ONE table of road speed-ups per category (plan §2). A
// category absent here (naval, anything unknown) gets no effect: roads are land.
var roadFactors = map[string]float64{
	"land":              0.75,
	CategoryCourier:     0.70,
	CategoryCourierLand: 0.70,
	CategoryCaravan:     0.60,
}

// RoadFactor is the category's multiplier on an entered road edge (1 = none).
func RoadFactor(category string) float64 {
	if f, ok := roadFactors[category]; ok {
		return f
	}
	return 1
}

// roadableTerrain is the terrain a road edge may touch at BOTH ends (plan §2).
// Rivers, fords, sea and mountains never carry a road, even if a row exists.
func roadableTerrain(terrain string) bool {
	switch terrain {
	case "plains", "river_valley", "river_delta", "forest_olive_grove",
		"scrub_maquis", "hills", "forest_cedar", "semi_desert":
		return true
	}
	return false
}

// TradePricingCategory maps a stored trade-journey category ("land"/"naval") to
// the class its steps are priced in: a land caravan has its own road factor.
func TradePricingCategory(category string) string {
	if category == "land" {
		return CategoryCaravan
	}
	return category
}

// IsRoadableTerrain is the exported form for the order validator.
func IsRoadableTerrain(terrain string) bool { return roadableTerrain(terrain) }

// RoadSet holds a world's finished road edges under their canonical key.
type RoadSet map[[4]int]struct{}

// roadKey is the canonical (sorted by q, then r) key of the unordered edge a-b (F6).
func roadKey(a, b MapPosition) [4]int {
	if a.Q > b.Q || (a.Q == b.Q && a.R > b.R) {
		a, b = b, a
	}
	return [4]int{a.Q, a.R, b.Q, b.R}
}

// RoadKey exposes the canonical key for writers (the INSERT must store it sorted).
func RoadKey(a, b MapPosition) [4]int { return roadKey(a, b) }

// Has reports whether the edge a-b is a finished road. A nil set has none.
func (r RoadSet) Has(a, b MapPosition) bool {
	if len(r) == 0 {
		return false
	}
	_, ok := r[roadKey(a, b)]
	return ok
}

// LoadRoadSet loads every finished road edge of a world.
func LoadRoadSet(ctx context.Context, db Queryer, worldID uuid.UUID) (RoadSet, error) {
	rows, err := db.Query(ctx,
		`SELECT q1, r1, q2, r2 FROM road_edges WHERE world_id = $1`, worldID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roads := RoadSet{}
	for rows.Next() {
		var k [4]int
		if scanErr := rows.Scan(&k[0], &k[1], &k[2], &k[3]); scanErr != nil {
			return nil, scanErr
		}
		roads[k] = struct{}{}
	}
	return roads, rows.Err()
}

// LoadWorldGraph loads terrain and roads together — what every pricing caller needs.
func LoadWorldGraph(ctx context.Context, db Queryer, worldID uuid.UUID) (TileGraph, RoadSet, error) {
	g, err := LoadTileGraph(ctx, db, worldID)
	if err != nil {
		return nil, nil, err
	}
	roads, err := LoadRoadSet(ctx, db, worldID)
	if err != nil {
		return nil, nil, err
	}
	return g, roads, nil
}

// EdgeCost is the cost in ticks of stepping from -> to (entering `to`): the
// category's terrain cost for the target hex, times the category's road
// factor when a finished road edge joins the two hexes and both ends are
// roadable terrain. It is the ONE place movement is priced (F2): A*,
// StepHours and every frozen route go through it.
func EdgeCost(from, to MapPosition, fromTerrain, toTerrain, category string, roads RoadSet) float64 {
	cost := moveHoursFor(toTerrain, category)
	if len(roads) != 0 && roadableTerrain(fromTerrain) && roadableTerrain(toTerrain) && roads.Has(from, to) {
		cost *= RoadFactor(category)
	}
	return cost
}
