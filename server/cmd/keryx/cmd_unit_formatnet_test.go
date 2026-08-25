package main

import "testing"

// TestFormatNetPerTick locks the rounding rule directly: one decimal below
// 10 in magnitude (so a sub-integer netto never collapses to "+0"), zero
// decimals at or above it.
func TestFormatNetPerTick(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{-5, "-5.0"},
		{0.4, "+0.4"},
		{-0.4, "-0.4"},
		{900, "+900"},
		{9.99, "+10.0"},
		{10, "+10"},
	}
	for _, tt := range tests {
		if got := formatNetPerTick(tt.in); got != tt.want {
			t.Errorf("formatNetPerTick(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
