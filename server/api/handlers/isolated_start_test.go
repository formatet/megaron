package handlers

import "testing"

// P8 (soak 2026-07-18): a Nomadic Host could found on a plains-only site with
// no hills, no ore, and no neighbours in reach — a structural dead end for
// bronze (copper+tin), cult expansion, and trade — and the founding forecast
// said nothing about it. isolationWarningText is the pure heuristic
// `ColonizePreview` uses to surface that risk (a heads-up, never a gate —
// CLAUDE.md: "Startstaden: prognos + spelarval, INTE garanti").

func TestIsolationWarningText_FlagsOnlyWhenAllThreeMissing(t *testing.T) {
	cases := []struct {
		name                            string
		hasHills, hasMetal, hasNeighbor bool
		wantWarning                     bool
	}{
		{"nothing at all — the dead-end case", false, false, false, true},
		{"has hills — cleared", true, false, false, false},
		{"has metal — cleared", false, true, false, false},
		{"has a neighbour in reach — cleared", false, false, true, false},
		{"has everything — cleared", true, true, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isolationWarningText(tc.hasHills, tc.hasMetal, tc.hasNeighbor, 10)
			if tc.wantWarning && got == "" {
				t.Fatal("expected a warning, got none")
			}
			if !tc.wantWarning && got != "" {
				t.Fatalf("expected no warning, got %q", got)
			}
		})
	}
}

func TestIsolationWarningText_MentionsRadiusAndIsNotAGate(t *testing.T) {
	got := isolationWarningText(false, false, false, 10)
	if !contains(got, "10 hexes") {
		t.Errorf("expected the message to name the radius, got %q", got)
	}
	if !contains(got, "still your choice") {
		t.Errorf("expected the message to make clear this doesn't block founding, got %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
