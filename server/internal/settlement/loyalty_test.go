package settlement

import "testing"

func TestColonyPenalty(t *testing.T) {
	cases := []struct {
		colonies int
		want     int
	}{
		{0, 0},
		{1, 0},
		{2, 0},
		{3, -1},
		{4, -3},
		{5, -5},
		{10, -5},
	}
	for _, c := range cases {
		if got := ColonyPenalty(c.colonies); got != c.want {
			t.Errorf("ColonyPenalty(%d) = %d, want %d", c.colonies, got, c.want)
		}
	}
}

func TestRevoltConditionsMet(t *testing.T) {
	cases := []struct {
		loyalty    int
		fraction   float64
		trigger    bool
		want       bool
	}{
		{1, 0.0, true, true},
		{1, 0.4, true, true},
		{1, 0.5, true, false},  // fraction not < 0.5
		{1, 0.0, false, false}, // no trigger
		{2, 0.0, true, false},  // loyalty too high
		{4, 1.0, false, false},
	}
	for _, c := range cases {
		got := RevoltConditionsMet(c.loyalty, c.fraction, c.trigger)
		if got != c.want {
			t.Errorf("RevoltConditionsMet(%d, %.1f, %v) = %v, want %v",
				c.loyalty, c.fraction, c.trigger, got, c.want)
		}
	}
}

func TestLoyaltyProjection(t *testing.T) {
	cases := []struct {
		base   int
		deltas []int
		want   int
	}{
		{2, nil, 2},
		{2, []int{1}, 3},
		{2, []int{1, 1}, 4},
		{2, []int{1, 1, 1}, 4},  // clamped at 4
		{2, []int{-1}, 1},
		{2, []int{-1, -1}, 1},   // clamped at 1
		{2, []int{-1, 1}, 2},
		{1, []int{3}, 4},
		{4, []int{-3}, 1},
	}
	for _, c := range cases {
		got := LoyaltyProjection(c.base, c.deltas)
		if got != c.want {
			t.Errorf("LoyaltyProjection(%d, %v) = %d, want %d", c.base, c.deltas, got, c.want)
		}
	}
}
