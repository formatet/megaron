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
