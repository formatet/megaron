package province

import (
	"errors"
	"math"
	"testing"
)

func TestTradeJourneyTerrainAndRounding(t *testing.T) {
	g := TileGraph{{0, 0}: "plains", {1, 0}: "river_ford", {2, 0}: "plains"}
	j, err := g.PlanTradeJourney(MapPosition{}, MapPosition{Q: 2}, "land")
	if err != nil {
		t.Fatal(err)
	}
	if j.TravelTicks != 5 || j.Distance != 2 || j.StepCosts[0] != 2500 || j.StepCosts[1] != 750 {
		t.Fatalf("journey %+v", j)
	}
	// 1.5*(2.5+.75)=4.875 rounds ONCE to5; reversing enters a different start.
	g[[2]int{0, 0}] = "river_ford"
	reverse, err := g.PlanTradeJourney(MapPosition{Q: 2}, MapPosition{}, "land")
	if err != nil {
		t.Fatal(err)
	}
	if reverse.TravelTicks == j.TravelTicks {
		t.Fatalf("reverse must price entering terrain: %+v", reverse)
	}
}

func TestTradeJourneyRejectsDisconnectedAndUsesWater(t *testing.T) {
	g := TileGraph{{0, 0}: "plains", {1, 0}: "coastal_sea", {2, 0}: "coastal_sea", {3, 0}: "plains"}
	if _, err := g.PlanTradeJourney(MapPosition{}, MapPosition{Q: 3}, "land"); !errors.Is(err, ErrNoTradePath) {
		t.Fatalf("land result %v", err)
	}
	j, err := g.PlanTradeJourney(MapPosition{}, MapPosition{Q: 3}, "naval")
	if err != nil {
		t.Fatal(err)
	}
	if j.Path[0] != (MapPosition{Q: 1}) || j.Path[len(j.Path)-1] != (MapPosition{Q: 2}) || j.TravelTicks != 1 {
		t.Fatalf("water journey %+v", j)
	}
	for _, p := range j.Path {
		if !IsPassable(g[[2]int{p.Q, p.R}], "naval") {
			t.Fatalf("naval path on land %+v", p)
		}
	}
}

func TestTradeJourneyMinimumAndInvalidSavedCosts(t *testing.T) {
	g := TileGraph{{0, 0}: "plains"}
	j, err := g.PlanTradeJourney(MapPosition{}, MapPosition{}, "land")
	if err != nil || j.TravelTicks != 1 || j.Validate() != nil {
		t.Fatalf("zero distance: %+v %v", j, err)
	}
	j = TradeJourney{Category: "land", Path: []MapPosition{{}, {Q: 1}}, StepCosts: []int64{math.MaxInt64}, Distance: 1, TravelTicks: 1}
	if j.Validate() == nil {
		t.Fatal("overflow saved costs accepted")
	}
}
