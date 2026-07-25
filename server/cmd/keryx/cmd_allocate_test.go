package main

import "testing"

// TestGrainBreakevenNote guards the weight-vs-percent unit trap: the server
// reports breakeven_grain_weight as a WEIGHT (0..1, see province.go's
// CatchmentBasePotential-derived breakevenGrainWeight), but printCurrentAllocation
// renders everything else — including the grain row's own percent — as a
// PERCENT (0..100). Mixing the two units up makes the comparison fire 100x too
// eagerly (any weight > 0 reads as "below" a raw percent) or never (any
// percent < 1 reads as "above" a raw weight). This test pins the *100 scaling
// so a future edit that "simplifies" the arithmetic gets caught immediately.
func TestGrainBreakevenNote(t *testing.T) {
	weight := func(w float64) *float64 { return &w }

	tests := []struct {
		name           string
		pct            float64
		breakeven      *float64
		wantEmpty      bool
		wantBelow      bool
		wantThresholds string // substring that must appear (the scaled percent)
	}{
		{
			name:      "nil weight (catchment cannot grow grain) stays silent",
			pct:       0,
			breakeven: nil,
			wantEmpty: true,
		},
		{
			name:           "45% weight, 30% allocated — below break-even",
			pct:            30,
			breakeven:      weight(0.45),
			wantBelow:      true,
			wantThresholds: "45%",
		},
		{
			name:           "45% weight, 50% allocated — above break-even",
			pct:            50,
			breakeven:      weight(0.45),
			wantBelow:      false,
			wantThresholds: "45%",
		},
		{
			name:           "exactly at break-even counts as meeting it, not below",
			pct:            45,
			breakeven:      weight(0.45),
			wantBelow:      false,
			wantThresholds: "45%",
		},
		{
			name: "the trap itself: a small weight (0.05) must not be compared " +
				"raw against a percent — 5% allocated must read as ABOVE a 0.05 " +
				"(=5%) break-even, not below",
			pct:            5,
			breakeven:      weight(0.05),
			wantBelow:      false,
			wantThresholds: "5%",
		},
		{
			name: "the trap inverted: a low percent (2%) against a real 15% " +
				"weight floor must still read as BELOW",
			pct:            2,
			breakeven:      weight(0.15),
			wantBelow:      true,
			wantThresholds: "15%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := grainBreakevenNote(tt.pct, tt.breakeven)
			if tt.wantEmpty {
				if got != "" {
					t.Fatalf("grainBreakevenNote(%v, weight) = %q, want empty", tt.pct, got)
				}
				return
			}
			if got == "" {
				t.Fatalf("grainBreakevenNote(%v, %v) = empty, want a note", tt.pct, *tt.breakeven)
			}
			if !contains(got, tt.wantThresholds) {
				t.Errorf("grainBreakevenNote(%v, %v) = %q, want it to contain %q",
					tt.pct, *tt.breakeven, got, tt.wantThresholds)
			}
			gotBelow := contains(got, "BELOW")
			if gotBelow != tt.wantBelow {
				t.Errorf("grainBreakevenNote(%v, %v) = %q, wantBelow=%v", tt.pct, *tt.breakeven, got, tt.wantBelow)
			}
		})
	}
}
