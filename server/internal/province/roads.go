package province

import (
	"context"
	"math"

	"github.com/google/uuid"
)

// Roads (megaron_plan_vagar.md V1). A road is an EDGE between two neighbouring
// hexes, built by an army, free for everyone to use. It scales the cost of
// ENTERING the edge's target hex to one flat per-category time, whatever the terrain. There are no
// bridges: the road ends at a ford and fords are waded as before.

// CategoryCaravan is the price class of a land trade caravan. It behaves like
// "land" everywhere (passability, terrain cost) and differs only in its road
// factor. It is an internal pricing class; TradeJourney.Category stays "land".
const CategoryCaravan = "caravan"

// roadTicks is the ONE table of road speeds (plan §2, Timothy 2026-10-15): a
// road edge costs the same per entered hex on EVERY roadable terrain — the road
// is the great leveller — and the chariot is fastest on it. Ticks per hex; a
// category absent here (naval, anything unknown) gets no effect: roads are land.
// Foot 0.75 is what plains cost with the old ×0.75; the runner is half that
// again; the chariot is its own plains time (0.67) × 0.6.
var roadTicks = map[string]float64{
	"land":              0.75,
	CategoryCourier:     0.35,
	CategoryCourierLand: 0.35,
	CategoryCaravan:     0.60,
	CategoryWarChariot:  0.40,
}

// RoadTicks is the category's cost per hex entered over a road edge, and
// whether it has one at all.
func RoadTicks(category string) (float64, bool) {
	t, ok := roadTicks[category]
	return t, ok
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

// RoadSet is a world's edge modifiers: finished road edges under their
// canonical key, and the river current on directed water steps. A nil set has
// neither, so callers that pass nil price plain terrain.
type RoadSet = *EdgeSet

// EdgeSet is what RoadSet points to. Build it with NewRoadSet or LoadWorldGraph.
type EdgeSet struct {
	roads map[[4]int]struct{}
	flow  map[[4]int]int8 // directed from→to: +1 downstream, −1 upstream
}

// NewRoadSet returns an empty edge set.
func NewRoadSet() RoadSet {
	return &EdgeSet{roads: map[[4]int]struct{}{}, flow: map[[4]int]int8{}}
}

// roadKey is the canonical (sorted by q, then r) key of the unordered edge a-b (F6).
func roadKey(a, b MapPosition) [4]int {
	if a.Q > b.Q || (a.Q == b.Q && a.R > b.R) {
		a, b = b, a
	}
	return [4]int{a.Q, a.R, b.Q, b.R}
}

// RoadKey exposes the canonical key for writers (the INSERT must store it sorted).
func RoadKey(a, b MapPosition) [4]int { return roadKey(a, b) }

// AddRoad records a finished road edge a-b.
func (r *EdgeSet) AddRoad(a, b MapPosition) { r.roads[roadKey(a, b)] = struct{}{} }

// Has reports whether the edge a-b is a finished road. A nil set has none.
func (r *EdgeSet) Has(a, b MapPosition) bool {
	if r == nil || len(r.roads) == 0 {
		return false
	}
	_, ok := r.roads[roadKey(a, b)]
	return ok
}

// RoadCount is the number of finished road edges.
func (r *EdgeSet) RoadCount() int {
	if r == nil {
		return 0
	}
	return len(r.roads)
}

// Current reports the river current on the step from→to: +1 with it
// (downstream), −1 against it (upstream), 0 across or unknown.
func (r *EdgeSet) Current(from, to MapPosition) int8 {
	if r == nil {
		return 0
	}
	return r.flow[[4]int{from.Q, from.R, to.Q, to.R}]
}

// addCurrent records that water at `at` flows out through edge i (dir +1,
// downstream) or comes in through it (dir −1, upstream): stepping from `at`
// across edge i goes with the current when dir is +1, and the reverse step
// goes the other way. Edge indices follow axialDirs (= world.RiverEdgeDelta).
func (r *EdgeSet) addCurrent(at MapPosition, mask int16, dir int8) {
	for i, d := range axialDirs {
		if mask&(1<<i) == 0 {
			continue
		}
		n := MapPosition{Q: at.Q + d[0], R: at.R + d[1]}
		r.flow[[4]int{at.Q, at.R, n.Q, n.R}] = dir
		r.flow[[4]int{n.Q, n.R, at.Q, at.R}] = -dir
	}
}

// LoadRoadSet loads every finished road edge of a world.
func LoadRoadSet(ctx context.Context, db Queryer, worldID uuid.UUID) (RoadSet, error) {
	rows, err := db.Query(ctx,
		`SELECT q1, r1, q2, r2 FROM road_edges WHERE world_id = $1`, worldID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roads := NewRoadSet()
	for rows.Next() {
		var k [4]int
		if scanErr := rows.Scan(&k[0], &k[1], &k[2], &k[3]); scanErr != nil {
			return nil, scanErr
		}
		roads.roads[k] = struct{}{}
	}
	return roads, rows.Err()
}

// loadRiverCurrent adds the saved river direction (mig 221) to roads. Only
// tiles whose flow is known carry it; the rest stay across/unknown.
func loadRiverCurrent(ctx context.Context, db Queryer, worldID uuid.UUID, roads RoadSet) error {
	rows, err := db.Query(ctx,
		`SELECT q, r, river_upstream_edges, river_downstream_edges FROM map_tiles
		 WHERE world_id = $1 AND river_flow_known`, worldID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var at MapPosition
		var up, down int16
		if scanErr := rows.Scan(&at.Q, &at.R, &up, &down); scanErr != nil {
			return scanErr
		}
		roads.addCurrent(at, up, -1)
		roads.addCurrent(at, down, +1)
	}
	return rows.Err()
}

// LoadWorldGraph loads terrain, roads and river current together — what every pricing caller needs.
func LoadWorldGraph(ctx context.Context, db Queryer, worldID uuid.UUID) (TileGraph, RoadSet, error) {
	g, err := LoadTileGraph(ctx, db, worldID)
	if err != nil {
		return nil, nil, err
	}
	roads, err := LoadRoadSet(ctx, db, worldID)
	if err != nil {
		return nil, nil, err
	}
	if err := loadRiverCurrent(ctx, db, worldID, roads); err != nil {
		return nil, nil, err
	}
	return g, roads, nil
}

// Ships go with a river's current in half the time and against it in twice
// the time (Timothy 2026-10-28). Runners, armies and caravans are unaffected.
const (
	downstreamFactor = 0.5
	upstreamFactor   = 2.0
)

// EdgeCost is the cost in ticks of stepping from -> to (entering `to`): the
// category's terrain cost for the target hex, or — when a finished road edge
// joins the two hexes, both ends are roadable terrain and the category uses
// roads — the category's flat road time instead. A ship entering a river or
// ford hex is scaled by the current on that step. TestRoadIsNeverSlowerThanTerrain
// pins that every road time beats every roadable terrain for its category.
// It is the ONE place movement is priced (F2): A*, StepHours and every frozen
// route go through it.
func EdgeCost(from, to MapPosition, fromTerrain, toTerrain, category string, roads RoadSet) float64 {
	cost := moveHoursFor(toTerrain, category)
	if roads.RoadCount() != 0 && roadableTerrain(fromTerrain) && roadableTerrain(toTerrain) && roads.Has(from, to) {
		if rt, ok := RoadTicks(category); ok {
			cost = rt
		}
	}
	if category == "naval" && (toTerrain == "river" || toTerrain == "river_ford") {
		switch roads.Current(from, to) {
		case +1:
			cost *= downstreamFactor
		case -1:
			cost *= upstreamFactor
		}
	}
	return cost
}

// RoadStoneCost is the stone one road edge into toTerrain costs (plan §2, B2):
// a stone wall (WallLevelSpecs[2]) is worth three plains edges, so one edge is
// a third of its stone, times the target hex's terrain cost. It used to read the
// wall table; B1 (murtak, 2026-10-21) raised the Stone Wall to 40 stone for the
// population ceiling and that must not silently reprice every road, so the basis
// is frozen at the 24 stone the wall cost when the road prices were set.
const roadStoneWallBasis = 24.0

func RoadStoneCost(toTerrain string) float64 {
	return roadStoneWallBasis / 3 * TerrainMoveTicks(toTerrain)
}

// RoadBuildTicks is how long an army of armySize needs for one edge into
// toTerrain: ceil(100 × terrain cost / size), at least 1 tick (plan §2: ~100
// men build a plains edge in 1 tick and a hill edge in 2).
func RoadBuildTicks(toTerrain string, armySize int) int {
	if armySize < 1 {
		armySize = 1
	}
	t := int(math.Ceil(100*TerrainMoveTicks(toTerrain)/float64(armySize) - 1e-9))
	if t < 1 {
		t = 1
	}
	return t
}
