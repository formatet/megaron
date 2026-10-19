package main

import "testing"

func TestHerdLine(t *testing.T) {
	mk := func(c, k float64) map[string]any {
		return map[string]any{"food": map[string]any{"herd": map[string]any{"count": c, "cap": k}}}
	}
	if got := herdLine(mk(50, 50)); got != "Herd 50 / 50 — full: new animals are eaten, then lost. A pen raises the cap." {
		t.Fatal(got)
	}
	if got := herdLine(mk(40, 150)); got != "Herd 40 / 150" {
		t.Fatal(got)
	}
	calves := map[string]any{"food": map[string]any{"herd": map[string]any{"count": 40.0, "cap": 50.0, "births_next": 4.0, "births_in": 3.0, "births_grain": 4.0}}}
	if got := herdLine(calves); got != "Herd 40 / 50 — 4 calves in 3 days (4 grain)" {
		t.Fatal(got)
	}
	if got := herdLine(map[string]any{}); got != "" {
		t.Fatal(got)
	}
}
