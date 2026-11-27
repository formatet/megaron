package main

import (
	"strings"
	"testing"
)

// The growth surplus keeps one decimal above 10, as the web's food row does.
func TestPeopleFoodLineGrowthKeepsOneDecimal(t *testing.T) {
	line := peopleFoodLine(map[string]any{"food": map[string]any{
		"production_per_tick": 60.0, "consumption_per_tick": 47.6, "food_net_per_tick": 12.4,
	}})
	if !strings.Contains(line, "growth surplus +12.4 per game day") {
		t.Fatalf("growth line %q", line)
	}
}
