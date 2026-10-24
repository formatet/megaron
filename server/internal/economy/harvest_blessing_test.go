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
	// The herd breeds by itself (herd_births.go); its rate is 0 since mig 192,
	// so a livestock blessing promised nothing (Timothy 2026-10-24 "stryk det").
	if got[GoodLivestock] {
		t.Error("livestock is blessed")
	}
	for _, g := range FoodGoods {
		if g != GoodFish && !got[g] {
			t.Errorf("%s is food but not blessed", g)
		}
	}
}
