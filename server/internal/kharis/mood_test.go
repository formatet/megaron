package kharis

import "testing"

// TestDeriveMood_0_100_Thresholds verifies the FAS 0 rescale (Timothy 2026-07-09
// kharis omdesign, temenos_kharis.md §"KANONISK OMDESIGN"): the four mood tiers on
// the new 0-100 scale (60/30/10, strawman — temenos_balans_spakar.md §9).
func TestDeriveMood_0_100_Thresholds(t *testing.T) {
	cases := []struct {
		kharis float64
		want   string
	}{
		{100, "overdadig"}, // cap
		{65, "overdadig"},  // Favorable
		{60, "overdadig"},  // boundary, inclusive
		{59, "vardig"},
		{30, "vardig"}, // Indifferent, boundary
		{29, "tveksam"},
		{25, "tveksam"}, // Suspicious
		{10, "tveksam"}, // boundary, inclusive
		{9, "vredgad"},
		{0, "vredgad"}, // Wrathful
	}
	for _, c := range cases {
		if got := deriveMood(c.kharis); got != c.want {
			t.Errorf("deriveMood(%v) = %q, want %q", c.kharis, got, c.want)
		}
	}
}
