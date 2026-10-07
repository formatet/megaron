package province

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/google/uuid"
)

// ErrNoTradePath means the chosen mode has no traversable route. Callers must
// reject before committing goods, ships, offers or arrival jobs.
var ErrNoTradePath = errors.New("no traversable trade route")

// TradeJourney freezes a goods journey at dispatch. Costs are thousandths of
// unrounded march ticks for the entered hexes; only the total caravan duration
// is rounded. Reverse legs are planned separately because entering costs differ.
type TradeJourney struct {
	Category    string        `json:"category"`
	Path        []MapPosition `json:"path"`
	StepCosts   []int64       `json:"step_costs"`
	TravelTicks int           `json:"travel_ticks"`
	Distance    int           `json:"distance"`
}

// PlanTradeJourney uses one terrain snapshot for A* and its entering-hex costs.
// Naval endpoints can be water positions or settlements whose dock is the first
// adjacent navigable hex in the same deterministic order as NearestSeaNeighbor.
func PlanTradeJourney(ctx context.Context, db Queryer, worldID uuid.UUID, origin, dest MapPosition, category string) (TradeJourney, error) {
	g, err := LoadTileGraph(ctx, db, worldID)
	if err != nil {
		return TradeJourney{}, err
	}
	return g.PlanTradeJourney(origin, dest, category)
}

// PlanTradeJourney is the pure planner on an already loaded terrain snapshot.
func (g TileGraph) PlanTradeJourney(origin, dest MapPosition, category string) (TradeJourney, error) {
	if category != "land" && category != "naval" {
		return TradeJourney{}, fmt.Errorf("invalid trade category %q", category)
	}
	if category == "naval" {
		var ok bool
		origin, ok = g.tradeDock(origin)
		if !ok {
			return TradeJourney{}, ErrNoTradePath
		}
		dest, ok = g.tradeDock(dest)
		if !ok {
			return TradeJourney{}, ErrNoTradePath
		}
	}
	path, _, ok := g.FindPath(origin, dest, category)
	if !ok {
		return TradeJourney{}, ErrNoTradePath
	}
	costs := make([]int64, 0, len(path)-1)
	var total int64
	for _, cost := range g.StepHours(path, category) {
		milli := int64(math.Round(cost * 1000))
		if milli <= 0 {
			return TradeJourney{}, fmt.Errorf("invalid terrain cost")
		}
		costs = append(costs, milli)
		total += milli
	}
	// Caravan calibration: 1.5 times raw march ticks, round half up ONCE.
	ticks := int((total*3 + 1000) / 2000)
	if ticks < 1 {
		ticks = 1
	}
	return TradeJourney{Category: category, Path: path, StepCosts: costs, TravelTicks: ticks, Distance: len(path) - 1}, nil
}

func (g TileGraph) tradeDock(p MapPosition) (MapPosition, bool) {
	if terrain, exists := g[[2]int{p.Q, p.R}]; exists && isPassable(terrain, "naval") {
		return p, true
	}
	for _, d := range axialDirs {
		next := MapPosition{Q: p.Q + d[0], R: p.R + d[1]}
		if terrain, exists := g[[2]int{next.Q, next.R}]; exists && isPassable(terrain, "naval") {
			return next, true
		}
	}
	return MapPosition{}, false
}

// Validate checks frozen data without querying terrain or finding a new route.
func (j TradeJourney) Validate() error {
	if j.Category != "land" && j.Category != "naval" {
		return fmt.Errorf("invalid journey category")
	}
	if len(j.Path) < 1 || len(j.StepCosts) != len(j.Path)-1 || j.Distance != len(j.Path)-1 || j.TravelTicks < 1 {
		return fmt.Errorf("invalid saved trade journey")
	}
	var total int64
	for i, c := range j.StepCosts {
		if c <= 0 || c > (math.MaxInt64-1000)/3-total || HexDistance(j.Path[i], j.Path[i+1]) != 1 {
			return fmt.Errorf("invalid saved trade step %d", i)
		}
		total += c
	}
	ticks := int((total*3 + 1000) / 2000)
	if ticks < 1 {
		ticks = 1
	}
	if ticks != j.TravelTicks {
		return fmt.Errorf("saved trade duration does not match costs")
	}
	return nil
}
