package main

import "testing"

func TestFmtFlowsHasNoGainSign(t *testing.T) {
	if got := fmtFlows(map[string]float64{"grain": 31.67}); got != "grain 31.67" {
		t.Fatalf("fmtFlows = %q", got)
	}
}
