package province

import (
	"math"
	"testing"
)

func TestRO1TerrainCosts(t *testing.T) {
	for _, c := range []struct {
		terrain string
		want    float64
	}{
		{"plains", 1}, {"river_valley", 1}, {"river_delta", 1},
		{"forest_olive_grove", 1.5}, {"scrub_maquis", 1.5},
		{"hills", 2}, {"forest_cedar", 2}, {"semi_desert", 3},
		{"coastal_sea", 1.0 / 3}, {"deep_sea", 1.0 / 5}, {"river", 0.5},
	} {
		t.Run(c.terrain, func(t *testing.T) {
			if got := TerrainMoveTicks(c.terrain); math.Abs(got-c.want) > 1e-12 {
				t.Fatalf("RÖ1 %s cost = %v, want %v", c.terrain, got, c.want)
			}
		})
	}
	for _, terrain := range []string{"mountain_limestone", "mountain_red"} {
		for _, category := range []string{"land", "naval", CategoryCourier, CategoryCourierLand} {
			if IsPassable(terrain, category) {
				t.Fatalf("mountain %s became passable for %s", terrain, category)
			}
		}
	}
}

func TestRO1CourierLandAndRiver(t *testing.T) {
	for _, c := range []struct {
		terrain string
		want    float64
	}{{"plains", 0.5}, {"hills", 1}, {"semi_desert", 1.5}, {"river", 0.5}, {"river_ford", 1.25}} {
		if got := moveHoursFor(c.terrain, CategoryCourier); got != c.want {
			t.Fatalf("courier %s cost = %v, want %v", c.terrain, got, c.want)
		}
	}
}

func TestRO1CaravanFactorAndFrozenJourney(t *testing.T) {
	for _, category := range []string{"land", "naval"} {
		terrain := "plains"
		want := 6
		if category == "naval" {
			terrain = "river"
			want = 3
		}
		g := TileGraph{}
		for q := 0; q <= 4; q++ {
			g[[2]int{q, 0}] = terrain
		}
		j, err := g.PlanTradeJourney(MapPosition{}, MapPosition{Q: 4}, category, nil)
		if err != nil || j.TravelTicks != want {
			t.Fatalf("caravan %s duration = %d, want %d (1.5×); err %v", category, j.TravelTicks, want, err)
		}
	}
	// Old prices remain valid after RÖ1; no current terrain lookup is allowed.
	old := TradeJourney{Category: "land", Path: []MapPosition{{}, {Q: 1}, {Q: 2}}, StepCosts: []int64{2500, 750}, TravelTicks: 5, Distance: 2}
	if err := old.Validate(); err != nil {
		t.Fatalf("frozen old journey invalidated: %v", err)
	}
}
