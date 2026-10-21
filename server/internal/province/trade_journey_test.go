package province

import (
	"errors"
	"math"
	"testing"
)

func TestTradeJourneyTerrainAndRounding(t *testing.T) {
	g := TileGraph{{0, 0}: "plains", {1, 0}: "river_ford", {2, 0}: "plains"}
	j, err := g.PlanTradeJourney(MapPosition{}, MapPosition{Q: 2}, "land", nil)
	if err != nil {
		t.Fatal(err)
	}
	if j.TravelTicks != 5 || j.Distance != 2 || j.StepCosts[0] != 2500 || j.StepCosts[1] != 1000 {
		t.Fatalf("journey %+v", j)
	}
	// 1.5*(2.5+1)=5.25 rounds ONCE to5; reversing enters a different start.
	g[[2]int{0, 0}] = "river_ford"
	reverse, err := g.PlanTradeJourney(MapPosition{Q: 2}, MapPosition{}, "land", nil)
	if err != nil {
		t.Fatal(err)
	}
	if reverse.TravelTicks == j.TravelTicks {
		t.Fatalf("reverse must price entering terrain: %+v", reverse)
	}
}

func TestTradeJourneyRejectsDisconnectedAndUsesWater(t *testing.T) {
	g := TileGraph{{0, 0}: "plains", {1, 0}: "coastal_sea", {2, 0}: "coastal_sea", {3, 0}: "plains"}
	if _, err := g.PlanTradeJourney(MapPosition{}, MapPosition{Q: 3}, "land", nil); !errors.Is(err, ErrNoTradePath) {
		t.Fatalf("land result %v", err)
	}
	j, err := g.PlanTradeJourney(MapPosition{}, MapPosition{Q: 3}, "naval", nil)
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
	j, err := g.PlanTradeJourney(MapPosition{}, MapPosition{}, "land", nil)
	if err != nil || j.TravelTicks != 1 || j.Validate() != nil {
		t.Fatalf("zero distance: %+v %v", j, err)
	}
	j = TradeJourney{Category: "land", Path: []MapPosition{{}, {Q: 1}}, StepCosts: []int64{math.MaxInt64}, Distance: 1, TravelTicks: 1}
	if j.Validate() == nil {
		t.Fatal("overflow saved costs accepted")
	}
}

func TestTradeJourneyUsesBestPortApproach(t *testing.T) {
	for _, blockedNorth := range []bool{false, true} {
		t.Run(map[bool]string{false: "shorter_south", true: "first_dock_isolated"}[blockedNorth], func(t *testing.T) {
			g := TileGraph{{0, 0}: "plains", {4, 0}: "plains"}
			for _, p := range [][2]int{{-1, 0}, {-1, -1}, {0, -2}, {1, -2}, {1, -1}, {2, -2}, {3, -2}, {4, -2}, {4, -1}, {0, 1}, {1, 1}, {2, 1}, {3, 1}} {
				g[p] = "coastal_sea"
			}
			if blockedNorth {
				delete(g, [2]int{2, -2})
			}
			j, err := g.PlanTradeJourney(MapPosition{}, MapPosition{Q: 4}, "naval", nil)
			if err != nil {
				t.Fatal(err)
			}
			if j.Distance != 3 || j.Path[0] != (MapPosition{R: 1}) || j.Path[len(j.Path)-1] != (MapPosition{Q: 3, R: 1}) {
				t.Fatalf("wanted short southern approach, got%+v", j)
			}
			for _, p := range j.Path {
				if !IsPassable(g[[2]int{p.Q, p.R}], "naval") {
					t.Fatalf("sailed over land:%v", p)
				}
			}
		})
	}
}
