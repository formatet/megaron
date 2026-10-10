package economy

import "testing"

func TestGrowthState(t *testing.T) {
	need := GrainConsumptionPerTick(1000)
	cases := []struct {
		name  string
		unmet float64
		rate  float64
		want  string
	}{
		{"negative net holds", 0, need - 1.7, GrowthHolding},
		{"zero net holds", 0, need, GrowthHolding},
		{"positive net grows", 0, need + 30.8, GrowthGrowing},
		{"hunger shrinks even with positive net", 5, need + 30.8, GrowthShrinking},
	}
	for _, tc := range cases {
		if got := GrowthState(tc.unmet, FoodNet(tc.rate, 1000)); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}
