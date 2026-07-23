package main

import (
	"strings"
	"testing"
)

// A recruit blocked by upkeep must say WHICH resource binds. Soak 2026-07-22:
// two playtesters in a row read the bare "city can't carry this yet" as a
// population cap, then a unit cap, then a stock cap — one of them while holding
// 118k grain and short only silver. The string is read by humans and LLM agents
// alike, so the binding resource and the shortfall both have to be in it.
func TestUnsustainableReason_NamesTheBindingResource(t *testing.T) {
	cases := []struct {
		name                             string
		netGrain, netSilver              float64
		unitGrain, unitSilver            float64
		wantSubstrings, wantNotSubstring []string
	}{
		{
			name:     "silver binds, grain is plentiful — the live Antilokhos case",
			netGrain: 28434, netSilver: -7,
			unitGrain: 5, unitSilver: 2,
			wantSubstrings:   []string{"silver", "-7.0", "9.0"},
			wantNotSubstring: []string{"grain"},
		},
		{
			name:     "grain binds, silver is plentiful",
			netGrain: -3, netSilver: 500,
			unitGrain: 5, unitSilver: 2,
			wantSubstrings:   []string{"grain", "-3.0", "8.0"},
			wantNotSubstring: []string{"silver"},
		},
		{
			name:     "both bind — say both, do not pick one",
			netGrain: -1, netSilver: -1,
			unitGrain: 5, unitSilver: 2,
			wantSubstrings: []string{"grain", "silver", "6.0", "3.0"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := unsustainableReason(tc.netGrain, tc.netSilver, tc.unitGrain, tc.unitSilver)
			for _, want := range tc.wantSubstrings {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in %q", want, got)
				}
			}
			for _, unwanted := range tc.wantNotSubstring {
				if strings.Contains(got, unwanted) {
					t.Errorf("names %q though it is not short: %q", unwanted, got)
				}
			}
		})
	}
}

// The client's numbers can lag the server's verdict (the province GET and the
// sustainable flag are computed from the same read, but a tick can land between
// a cached view and a re-render). Inventing a shortfall that isn't there would
// be worse than the old vague string, so fall back rather than lie.
func TestUnsustainableReason_FallsBackWhenNeitherMarginIsNegative(t *testing.T) {
	got := unsustainableReason(100, 100, 5, 2)
	if !strings.Contains(got, "can't carry this yet") {
		t.Errorf("expected the neutral fallback, got %q", got)
	}
	if strings.Contains(got, "needs") {
		t.Errorf("fallback must not claim a shortfall: %q", got)
	}
}
