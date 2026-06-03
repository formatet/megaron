package handlers

import "testing"

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
