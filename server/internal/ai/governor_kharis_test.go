package ai

import "testing"

func TestDivineInterventionProbability_Thresholds(t *testing.T) {
	cases := []struct {
		kharis int
		wantGt float64
		wantLt float64
	}{
		{0, 0.29, 0.31},    // kharis=0 → P ≈ 0.30
		{200, 0.14, 0.16},  // kharis=200 → P ≈ 0.15
		{400, -1, 0.001},   // kharis=400 → P = 0 (no override)
		{800, -1, 0.001},   // kharis=800 → P = 0
	}
	for _, tc := range cases {
		p := DivineInterventionProbability(tc.kharis)
		if tc.wantGt >= 0 && p <= tc.wantGt {
			t.Errorf("kharis=%d: P=%.4f want > %.4f", tc.kharis, p, tc.wantGt)
		}
		if p >= tc.wantLt {
			t.Errorf("kharis=%d: P=%.4f want < %.4f", tc.kharis, p, tc.wantLt)
		}
	}
}

func TestDivineInterventionProbability_NeverNegative(t *testing.T) {
	for kharis := 0; kharis <= 1000; kharis += 50 {
		if p := DivineInterventionProbability(kharis); p < 0 {
			t.Errorf("kharis=%d: P=%.4f must not be negative", kharis, p)
		}
	}
}

func TestVoteWeighting_Bounds(t *testing.T) {
	for _, k := range []int{0, 100, 200, 400, 800, 1000} {
		w := VoteWeighting(k, 1.0)
		if w < 0 || w > 1 {
			t.Errorf("VoteWeighting(kharis=%d) = %.3f: must be in [0,1]", k, w)
		}
	}
}

func TestVoteWeighting_HighKharisHelps(t *testing.T) {
	low := VoteWeighting(50, 1.0)
	high := VoteWeighting(800, 1.0)
	if high <= low {
		t.Errorf("high kharis should give better vote weight: low=%.3f high=%.3f", low, high)
	}
}
