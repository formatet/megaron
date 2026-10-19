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
	if got := herdLine(map[string]any{}); got != "" {
		t.Fatal(got)
	}
}
