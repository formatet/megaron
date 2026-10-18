package main

import (
	"strings"
	"testing"
)

func TestSitosArmy_StatusShowsOwnGrainReserve(t *testing.T) {
	s := map[string]any{"army_upkeep": map[string]any{"grain": 1.0}, "food_net_per_tick_after_upkeep": 23.0, "sitos": map[string]any{"granary_per_good": map[string]any{"grain": 442.0, "fish": 400.0}}}
	got := armyFoodLine(s)
	if !strings.Contains(got, "442.0 grain in this city's Sitos can feed the army") || !strings.Contains(got, "+23") {
		t.Fatal(got)
	}
	s["sitos"] = map[string]any{"granary_per_good": map[string]any{"fish": 442.0}}
	if strings.Contains(armyFoodLine(s), "Sitos can feed") {
		t.Fatal("fish-only reserve promised army food")
	}
}
