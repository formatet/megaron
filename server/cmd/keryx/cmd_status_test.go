package main

import (
	"reflect"
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
