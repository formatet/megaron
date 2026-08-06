package province

import "testing"

// NormaliseCulture is the MVP culture gate (EK1 = B, Timothy 2026-08-05):
// every culture forced to minoan — empty, the active choice itself, one of
// the five deactivated cultures, or garbage input a client never should have
// sent (`keryx found --culture hatti` went straight through before this).
func TestNormaliseCulture(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"already minoan", "minoan"},
		{"deactivated akhaier", "akhaier"},
		{"deactivated khemetiu", "khemetiu"},
		{"deactivated knaani", "knaani"},
		{"deactivated thrakes", "thrakes"},
		{"deactivated hatti", "hatti"},
		{"unknown garbage", "atlantean"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormaliseCulture(tc.input); got != string(MVPCulture) {
				t.Errorf("NormaliseCulture(%q) = %q, want %q", tc.input, got, MVPCulture)
			}
		})
	}
}

func TestMVPCultureIsMinoan(t *testing.T) {
	if MVPCulture != CultureMinoan {
		t.Errorf("MVPCulture = %q, want %q", MVPCulture, CultureMinoan)
	}
}
