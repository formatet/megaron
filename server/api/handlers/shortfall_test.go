package handlers

import (
	"strings"
	"testing"
)

// TestShortfall_NeverHidesItsOwnDifference pins cli-sanning row D's bug class
// one layer down: the "need X, have Y" pair is rendered by ONE helper with nine
// call sites (every build/recruit/trade gate), and with %.0f on both numbers a
// Wanax holding 11,7 of a required 12 read "need 12, have 12" — a refusal that
// contradicts its own reason. The gate is honest only if the reader can see the
// gap without subtracting two rounded numbers.
func TestShortfall_NeverHidesItsOwnDifference(t *testing.T) {
	tests := []struct {
		name       string
		need, have float64
		wantParts  []string
		notEqual   bool // the two rendered figures must not read as the same number
	}{
		{
			name: "fractional stock just below a whole requirement",
			need: 12, have: 11.7,
			wantParts: []string{"need 12", "have 11.7", "0.3 short"},
			notEqual:  true,
		},
		{
			name: "sub-tenth gap falls back to two decimals rather than '0.0 short'",
			need: 5, have: 4.97,
			wantParts: []string{"need 5", "have 4.97", "0.03 short"},
			notEqual:  true,
		},
		{
			name: "whole numbers still read cleanly",
			need: 40, have: 12,
			wantParts: []string{"need 40", "have 12", "28 short"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shortfall(tc.need, tc.have)
			for _, want := range tc.wantParts {
				if !strings.Contains(got, want) {
					t.Errorf("shortfall(%v, %v) = %q, missing %q", tc.need, tc.have, got, want)
				}
			}
			if tc.notEqual && strings.Contains(got, "have 12,") {
				t.Errorf("shortfall(%v, %v) = %q — stock rounded up to equal the requirement, "+
					"which is the exact lie this helper exists to remove", tc.need, tc.have, got)
			}
		})
	}
}

// TestInsufficientGoodsError_CarriesTheGap proves the shared error type — not
// just the helper — reaches the player with the difference named. This is the
// string nine call sites return on a failed build/recruit/trade.
func TestInsufficientGoodsError_CarriesTheGap(t *testing.T) {
	e := &insufficientGoodsError{Short: []goodShortfall{
		{Good: "timber", Need: 12, Have: 11.7},
		{Good: "stone", Need: 40, Have: 39.9},
	}}
	got := e.Error()
	for _, want := range []string{"timber (need 12, have 11.7, 0.3 short)", "stone (need 40, have 39.9"} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, missing %q", got, want)
		}
	}
}

// TestInsufficientTradeMsg_CarriesTheGap — the trade path shares the helper, so
// a seller 0,3 short no longer reads as holding exactly what it owes.
func TestInsufficientTradeMsg_CarriesTheGap(t *testing.T) {
	got := insufficientTradeMsg("seller", "tin", 12, 11.7)
	if !strings.Contains(got, "0.3 short") {
		t.Errorf("insufficientTradeMsg = %q, want the shortfall named", got)
	}
	if strings.Contains(got, "have 12") {
		t.Errorf("insufficientTradeMsg = %q — stock rounds up to the requirement", got)
	}
}
