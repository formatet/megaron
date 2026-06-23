package handlers

import (
	"testing"

	"github.com/poleia/server/internal/province"
)

// insufficientGoodsError must render a human/agent-readable list of exactly
// what is short and by how much — the keryx agents read this string to learn
// which good to build toward or trade for, replacing the old blind 422.
func TestInsufficientGoodsErrorMessage(t *testing.T) {
	err := &insufficientGoodsError{Short: []goodShortfall{
		{Good: "stone", Need: 200, Have: 50},
		{Good: "timber", Need: 100, Have: 0},
	}}
	got := err.Error()
	want := "insufficient resources: stone (need 200, have 50), timber (need 100, have 0)"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestInsufficientGoodsErrorSingle(t *testing.T) {
	err := &insufficientGoodsError{Short: []goodShortfall{
		{Good: "cedar", Need: 80, Have: 12},
	}}
	want := "insufficient resources: cedar (need 80, have 12)"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// insufficientUnitsMsg turns a blind "insufficient units" 422 into an actionable
// list: agents that try to outpost/march with more troops than their garrison
// holds need to see which units are short and by how much to scale down.
func TestInsufficientUnitsMsg(t *testing.T) {
	// Wants 30 infantry + 5 chariot, only holds 10 infantry + 5 chariot.
	want := province.ArmyComposition{Spearman: 30, WarChariot: 5}
	have := province.ArmyComposition{Spearman: 10, WarChariot: 5}
	got := insufficientUnitsMsg(want, have)
	exp := "insufficient units: spearman (need 30, have 10)"
	if got != exp {
		t.Errorf("insufficientUnitsMsg = %q, want %q", got, exp)
	}
}

func TestInsufficientUnitsMsgMultiple(t *testing.T) {
	want := province.ArmyComposition{Spearman: 20, Ship: 3, EliteInfantry: 4}
	have := province.ArmyComposition{Spearman: 5, Ship: 0, EliteInfantry: 1}
	got := insufficientUnitsMsg(want, have)
	exp := "insufficient units: spearman (need 20, have 5), ship (need 3, have 0), elite_infantry (need 4, have 1)"
	if got != exp {
		t.Errorf("insufficientUnitsMsg = %q, want %q", got, exp)
	}
}

// When nothing is actually short (defensive — caller only invokes this on a
// shortfall), fall back to the plain message rather than an empty list.
func TestInsufficientUnitsMsgNoShortfall(t *testing.T) {
	a := province.ArmyComposition{Spearman: 5}
	if got := insufficientUnitsMsg(a, a); got != "insufficient units" {
		t.Errorf("insufficientUnitsMsg = %q, want plain fallback", got)
	}
}

// A failed messenger trade must name the party, the good, and how much it holds
// so the agent can decline/restock/counter instead of re-accepting forever.
func TestInsufficientTradeMsg(t *testing.T) {
	if got := insufficientTradeMsg("seller", "cedar", 100, 0); got != "seller has insufficient cedar (need 100, have 0)" {
		t.Errorf("seller msg = %q", got)
	}
	if got := insufficientTradeMsg("buyer", "silver", 80, 12); got != "buyer has insufficient silver (need 80, have 12)" {
		t.Errorf("buyer msg = %q", got)
	}
}
