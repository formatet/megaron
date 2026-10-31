package main

import (
	"strings"
	"testing"
)

// A herd at its pen cap adds its overflow to the food net (Timothy 2026-10-31);
// the line must show the term so production + herd - eat = net adds up.
func TestPeopleFoodLine_ShowsHerdSurplus(t *testing.T) {
	sett := func(surplus float64) map[string]any {
		return map[string]any{"food": map[string]any{
			"production_per_tick": 5.0, "consumption_per_tick": 5.0, "food_net_per_tick": 0.25,
			"herd": map[string]any{"count": 50.0, "cap": 50.0, "surplus_rate": surplus},
		}}
	}
	if got := peopleFoodLine(sett(0.25)); !strings.Contains(got, "production 5.0 + herd 0.25") {
		t.Errorf("line = %q, want the herd term", got)
	}
	if got := peopleFoodLine(sett(0)); strings.Contains(got, "herd") {
		t.Errorf("line = %q, no herd term without surplus", got)
	}
}
