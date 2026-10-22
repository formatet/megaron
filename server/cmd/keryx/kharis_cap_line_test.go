package main

import (
	"strings"
	"testing"
)

// Haiku-8: "Kharis 50 · cap 50 · net +2.2" was followed by "climbing toward the cap 50".
func TestKharisCapLineAtCapDoesNotClimb(t *testing.T) {
	sett := func(kv float64) map[string]any {
		return map[string]any{"kharis": kv, "kharis_cap": 50.0, "max_temple_level": 1.0,
			"kharis_net_per_tick": 2.2, "kharis_net_known": true}
	}
	if got := kharisCapLine(sett(50)); !strings.Contains(got, "is at the cap 50") || strings.Contains(got, "climbing") {
		t.Fatalf("at cap: %q", got)
	}
	if got := kharisCapLine(sett(30)); !strings.Contains(got, "climbing toward the cap 50") {
		t.Fatalf("below cap: %q", got)
	}
}

func TestKharisStalledHintNamesPhysicalTempleStaffing(t *testing.T) {
	for _, net := range []float64{0, -.1} {
		out := kharisCapLine(map[string]any{"kharis": 40.0, "kharis_cap": 50.0, "max_temple_level": 1.0, "kharis_net_known": true, "kharis_net_per_tick": net})
		if strings.Contains(out, "allocate") || !strings.Contains(out, "keryx staff temple +1") {
			t.Fatal(out)
		}
	}
}
