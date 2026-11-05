package main

import (
	"strings"
	"testing"
)

func TestWallsHoldLine_OnlyTheCeilingNoCountdown(t *testing.T) {
	got := wallsHoldLine(10000, 2, 1)
	if !strings.Contains(got, "Walls hold") || !strings.Contains(got, "Granary of level 3") {
		t.Fatalf("wallsHoldLine = %q", got)
	}
	if strings.Contains(strings.ToLower(got), "room for") {
		t.Fatalf("the surface must show the ceiling only, got %q", got)
	}
	if strings.Contains(wallsHoldLine(20000, 3, 3), "Granary") {
		t.Fatalf("no next wall after Bronze Wall")
	}
	if wallsHoldLine(0, 0, 0) != "" {
		t.Fatalf("no cap reported -> no line")
	}
}

func TestStorehouseLine(t *testing.T) {
	got := storehouseLine(map[string]any{"storehouse": map[string]any{"amount": 120.0, "capacity": 500.0, "level": 0.0}})
	if !strings.HasPrefix(got, "Storehouse: 120 / 500") || !strings.Contains(got, "keryx build --type storehouse") {
		t.Fatalf("no building: %q", got)
	}
	got = storehouseLine(map[string]any{"storehouse": map[string]any{"amount": 1200.0, "capacity": 2500.0, "level": 2.0}})
	if got != "Storehouse: 1.2k / 2.5k (level 2)" {
		t.Fatalf("level 2: %q", got)
	}
	if storehouseLine(map[string]any{}) != "" {
		t.Fatal("a foreign city has no Storehouse line")
	}
}
