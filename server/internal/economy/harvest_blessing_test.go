package economy

import "testing"

// TestHarvestBlessedGoods_EveryFoodButFish: the blessing is the fields', not
// the sea's (Timothy 2026-10-10: "all matproduktion förutom fisk").
func TestHarvestBlessedGoods_EveryFoodButFish(t *testing.T) {
	got := map[string]bool{}
	for _, g := range HarvestBlessedGoods() {
		got[g] = true
	}
	if got[GoodFish] {
		t.Error("fish is blessed")
	}
	for _, g := range FoodGoods {
		if g != GoodFish && !got[g] {
			t.Errorf("%s is food but not blessed", g)
		}
	}
}
