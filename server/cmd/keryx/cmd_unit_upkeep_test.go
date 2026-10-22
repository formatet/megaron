package main

import (
	"encoding/json"
	"testing"
)

func TestUnitRosterUpkeepShowsZeroSilverAndAuthoritativeRations(t *testing.T) {
	for _, tt := range []struct{ payload, want string }{
		{`{"type":"spearman","status":"garrison","upkeep_context":"active","upkeep_grain_per_tick":0.5,"upkeep_silver_per_tick":0}`, "Upkeep: 0.5 grain, 0 silver per day"},
		{`{"type":"spearman","status":"marching","upkeep_grain_per_tick":1,"upkeep_silver_per_tick":0}`, "Upkeep: 1 grain, 0 silver per day"},
		{`{"type":"elite_infantry","upkeep_grain_per_tick":0.6,"upkeep_silver_per_tick":0.75}`, "Upkeep: 0.6 grain, 0.75 silver per day"},
		{`{"type":"galley","status":"marching","crew":20,"upkeep_food_per_tick":0.2,"upkeep_silver_per_tick":0.2}`, "Upkeep: 0.2 food, 0.2 silver per day"},
		{`{"type":"spearman","upkeep_context":"inactive","upkeep_grain_per_tick":0.2,"upkeep_silver_per_tick":0}`, "Upkeep when active: 0.2 grain, 0 silver per day"},
		{`{"type":"spearman","upkeep_context":"subsistence","upkeep_grain_per_tick":1,"upkeep_silver_per_tick":0}`, "Upkeep after settling: 1 grain, 0 silver per day"},
		{`{"type":"nomadic_host","upkeep_context":"none","upkeep_grain_per_tick":0,"upkeep_silver_per_tick":0}`, ""},
		{`{"type":"spearman"}`, ""},
	} {
		var u unitRow
		if err := json.Unmarshal([]byte(tt.payload), &u); err != nil {
			t.Fatal(err)
		}
		if got := unitUpkeepLine(u); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.payload, got, tt.want)
		}
	}
}
