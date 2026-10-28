package economy

import (
	"math"
	"testing"
)

func TestFoodConsumptionProportionalGrainAndFish(t *testing.T) {
	grain, fish, _ := FoodConsumptionSplit(10, 100, 900, 0)
	if math.Abs(grain-99) > 1e-9 || math.Abs(fish-891) > 1e-9 {
		t.Fatalf("remaining grain/fish=%g/%g want99/891", grain, fish)
	}
}

func TestPlanFoodThreeEqualValuesAndCivilianEmergency(t *testing.T) {
	stock := map[string]float64{GoodGrain: 100, GoodFish: 800, GoodOil: 100, GoodWine: 500, GoodLivestock: 50}
	meal := PlanFood(20, stock, false)
	for good, want := range map[string]float64{GoodGrain: 2, GoodFish: 16, GoodOil: 2, GoodWine: 0} {
		if math.Abs(meal.Used[good]-want) > 1e-9 {
			t.Errorf("%s eaten=%g want%g", good, meal.Used[good], want)
		}
	}
	if meal.Unmet != 0 || meal.Used[GoodLivestock] != 0 {
		t.Fatal(meal)
	}
	// Wine is not food (Timothy 2026-10-28): a city with only wine goes hungry.
	if wineOnly := PlanFood(5, map[string]float64{GoodWine: 500}, false); wineOnly.Unmet != 5 || wineOnly.Used[GoodWine] != 0 {
		t.Fatalf("wine-only meal=%+v; want all unmet", wineOnly)
	}
	civilian := PlanFood(5, map[string]float64{GoodLivestock: 50}, true)
	other := PlanFood(5, map[string]float64{GoodLivestock: 50}, false)
	if civilian.Unmet != 0 || civilian.Used[GoodLivestock] != 2 || other.Unmet != 5 {
		t.Fatalf("civilian=%+v other=%+v", civilian, other)
	}
}
