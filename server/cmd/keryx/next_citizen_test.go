package main

import (
	"strings"
	"testing"
)

func TestNextCitizenLineUsesServerProgressAndWorldClock(t *testing.T) {
	sett := map[string]any{"next_citizen_progress": float64(64), "next_citizen_growth_per_tick": float64(9), "next_citizen_in_ticks": float64(4), "food": map[string]any{"growth_state": "growing"}}
	got := nextCitizenLine(testTimeClient(), sett)
	if got != "Next citizen 64 / 100 · in about 4 game days (about 4 minutes)" {
		t.Fatal(got)
	}
	for _, tc := range []struct{ state, want string }{{"holding", "not growing"}, {"capped", "the walls are full"}, {"shrinking", "shrinking"}} {
		sett["food"] = map[string]any{"growth_state": tc.state}
		sett["next_citizen_in_ticks"] = nil
		if got := nextCitizenLine(nil, sett); got != "Next citizen 64 / 100 · "+tc.want {
			t.Fatal(got)
		}
	}
	sett["food"] = map[string]any{"growth_state": "growing"}
	sett["next_citizen_in_ticks"] = float64(1)
	if got := nextCitizenLine(nil, sett); !strings.Contains(got, "next game day") {
		t.Fatal(got)
	}
	if got := nextCitizenLine(nil, map[string]any{}); got != "" {
		t.Fatal(got)
	}
}
