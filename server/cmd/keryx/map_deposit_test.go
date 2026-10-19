package main

import "testing"

func TestDepositReach_OwnCatchmentBeatsColonizeHint(t *testing.T) {
	own := []mapCity{{Q: 0, R: 0, Name: "Phaistos"}}
	if got := depositReach(2, 0, true, own); got != "(in Phaistos's catchment)" {
		t.Errorf("mountain tin two hexes from Phaistos = %q, want the catchment hint", got)
	}
	if got := depositReach(3, 0, true, own); got != "(colonize adjacent hex)" {
		t.Errorf("mountain tin outside every catchment = %q, want the colonize hint", got)
	}
	if got := depositReach(5, 0, false, own); got != "" {
		t.Errorf("passable deposit outside every catchment = %q, want no hint", got)
	}
}
