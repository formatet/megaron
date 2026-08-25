package kharis

import (
	"testing"

	"formatet/megaron/server/internal/religion"
)

// TestDeriveMood_MatchesLowestPrayerTier verifies the mood table's lowest tier
// boundary equals religion.MoodSuspicious — the lowest MinKharis a prayer can
// gate on (megaron_plan_kultbrunnen.md §6). Before the fix, deriveMood's
// lowest tier was hardcoded to 10 while MinKharis was 5: a Wanax whose kharis
// cleared the gods' lowest prayer gate was still labelled "vredgad" (Wrathful).
// This compares against the prayer table's actual constant, not against the
// literal the mood table was hand-derived from — a test that only restated
// deriveMood's own number would pass under either bug.
func TestDeriveMood_MatchesLowestPrayerTier(t *testing.T) {
	got := deriveMood(religion.MoodSuspicious)
	if got == "vredgad" {
		t.Errorf("deriveMood(%v) = %q — the mood table's lowest tier disagrees with religion.MoodSuspicious (the lowest prayer gate); two threshold tables have drifted apart",
			religion.MoodSuspicious, got)
	}
}

// TestDeriveMood_0_100_Thresholds verifies the FAS 0 rescale (Timothy 2026-07-09
// kharis omdesign, temenos_kharis.md §"KANONISK OMDESIGN"): the four mood tiers on
// the new 0-100 scale (60/30/5, religion.MoodFavorable/MoodIndifferent/
// MoodSuspicious — lowered from 10 to 5 on 2026-08-25, megaron_plan_kultbrunnen.md
// §6, to match the lowest prayer gate).
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
		{10, "tveksam"},
		{5, "tveksam"}, // Suspicious, boundary, inclusive
		{4, "vredgad"},
		{0, "vredgad"}, // Wrathful
	}
	for _, c := range cases {
		if got := deriveMood(c.kharis); got != c.want {
			t.Errorf("deriveMood(%v) = %q, want %q", c.kharis, got, c.want)
		}
	}
}
