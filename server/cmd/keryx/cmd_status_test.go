package main

import (
	"reflect"
	"strings"
	"testing"
)

// TestUnusedCatchmentDeposits reproduces the P1a soak gap (2026-07-18): `status`
// showed Copper/Tin only as a produced good after a mine already existed, so a
// player who never built one never learned their own catchment held an ore.
func TestUnusedCatchmentDeposits(t *testing.T) {
	building := func(typ string) any { return map[string]any{"type": typ, "level": 1.0} }

	tests := []struct {
		name       string
		deposits   []any
		buildings  []any
		wantUnused []string
	}{
		{
			name:       "no deposits in catchment",
			deposits:   []any{},
			buildings:  nil,
			wantUnused: nil,
		},
		{
			name:       "copper and tin present, no mine built",
			deposits:   []any{"copper", "tin"},
			buildings:  nil,
			wantUnused: []string{"copper", "tin"},
		},
		{
			name:       "copper and tin present, mine already built",
			deposits:   []any{"copper", "tin"},
			buildings:  []any{building("mine")},
			wantUnused: nil,
		},
		{
			name:       "silver present, no silver_mine built",
			deposits:   []any{"silver"},
			buildings:  []any{building("mine")}, // a plain mine does not cover silver
			wantUnused: []string{"silver"},
		},
		{
			name:       "silver present, silver_mine built",
			deposits:   []any{"silver"},
			buildings:  []any{building("silver_mine")},
			wantUnused: nil,
		},
		{
			name:       "cedar deposit is never flagged (no mine-equivalent gate)",
			deposits:   []any{"cedar"},
			buildings:  nil,
			wantUnused: nil,
		},
		{
			name:       "mixed: tin unused, silver already mined",
			deposits:   []any{"tin", "silver"},
			buildings:  []any{building("silver_mine")},
			wantUnused: []string{"tin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unusedCatchmentDeposits(tt.deposits, tt.buildings)
			if !reflect.DeepEqual(got, tt.wantUnused) {
				t.Errorf("unusedCatchmentDeposits() = %#v, want %#v", got, tt.wantUnused)
			}
		})
	}
}

// TestMultiCityHint reproduces the recurring soak-round misreading of
// `status`'s hidden per-settlement scope (see multiCityHint's doc comment):
// with only one settlement the hint must stay silent (no noise in the normal
// case), and with more than one it must name the current city and point at
// `keryx settlements` / `keryx status --province <id>` for the rest.
func TestMultiCityHint(t *testing.T) {
	tests := []struct {
		name           string
		settlementName string
		used           float64
		wantEmpty      bool
	}{
		{name: "single settlement stays silent", settlementName: "Mycenae", used: 1, wantEmpty: true},
		{name: "zero (missing settlement_cap) stays silent", settlementName: "Mycenae", used: 0, wantEmpty: true},
		{name: "two settlements renders the hint", settlementName: "Mycenae", used: 2, wantEmpty: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := multiCityHint(tt.settlementName, tt.used)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("multiCityHint(%q, %v) = %q, want empty", tt.settlementName, tt.used, got)
				}
				return
			}
			if got == "" {
				t.Errorf("multiCityHint(%q, %v) = empty, want non-empty hint", tt.settlementName, tt.used)
			}
			if !strings.Contains(got, tt.settlementName) {
				t.Errorf("multiCityHint(%q, %v) = %q, want it to name the settlement", tt.settlementName, tt.used, got)
			}
			if !strings.Contains(got, "keryx settlements") || !strings.Contains(got, "keryx status --province") {
				t.Errorf("multiCityHint(%q, %v) = %q, want it to point at both escape hatches", tt.settlementName, tt.used, got)
			}
		})
	}
}
