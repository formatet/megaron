package economy

import "testing"

func TestFoodRunwayAndLowBoundary(t *testing.T) {
	if FoodRunway(100, 0) != nil || FoodRunway(100, 3) != nil {
		t.Fatal("a net that is not negative has no runway")
	}
	if d := FoodRunway(120, -5); d == nil || *d != 24 {
		t.Fatalf("120 food at -5 lasts 24 game days, got %v", d)
	}
	if d := FoodRunway(0, -5); d == nil || *d != 0 {
		t.Fatalf("no food lasts 0 game days, got %v", d)
	}
	low := func(stock, net float64) bool {
		f := FoodForecast{Stock: stock, Net: net, Days: FoodRunway(stock, net)}
		return FoodLow(f)
	}
	for _, c := range []struct {
		stock, net float64
		want       bool
	}{
		{119.9, -5, true}, // 23.98 days
		{120, -5, false},  // exactly 24: not low
		{500, -5, false},  // plenty
		{10, 0, false},    // net not negative: silent whatever the stock
		{10, 2, false},    // surplus
		{0, -0.001, true}, // empty and draining
	} {
		if got := low(c.stock, c.net); got != c.want {
			t.Errorf("FoodLow(stock %v, net %v) = %v, want %v", c.stock, c.net, got, c.want)
		}
	}
}
