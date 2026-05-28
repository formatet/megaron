package ai

import (
	"math"
	"testing"
)

func TestDivineInterventionProbability(t *testing.T) {
	cases := []struct {
		kharis int
		want   float64
	}{
		{400, 0},
		{500, 0},
		{0, 0.30},
		{200, 0.15},
		{100, 0.225},
		{399, 0.30 / 400},
	}
	for _, c := range cases {
		got := DivineInterventionProbability(c.kharis)
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("DivineInterventionProbability(%d) = %f, want %f", c.kharis, got, c.want)
		}
	}
}

func TestVoteWeighting(t *testing.T) {
	cases := []struct {
		kharis    int
		alignment float64
		wantMin   float64
		wantMax   float64
	}{
		{800, 1.0, 0.79, 0.81},  // 0.5 + 0.3
		{800, 0.0, 0.49, 0.51},  // 0.5 + 0.0
		{400, 1.0, 0.59, 0.61},  // 0.5 + 0.1
		{300, 0.5, 0.49, 0.51},  // base 0.5, no modifier for 200–399
		{150, 0.5, 0.29, 0.31},  // 0.5 - 0.2
		{50, 0.5, 0.14, 0.16},   // 0.5 - 0.35
		{0, 0.0, 0.0, 0.16},     // clamped at 0
	}
	for _, c := range cases {
		got := VoteWeighting(c.kharis, c.alignment)
		if got < c.wantMin || got > c.wantMax {
			t.Errorf("VoteWeighting(%d, %.1f) = %f, want [%f, %f]",
				c.kharis, c.alignment, got, c.wantMin, c.wantMax)
		}
	}
}
