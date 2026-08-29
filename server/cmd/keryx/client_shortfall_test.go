package main

import "testing"

// TestShortfall_MirrorsServerFormat pins keryx's copy of the need/have renderer
// to the server's (api/handlers/helpers.go shortfall). The two are deliberately
// duplicated — cmd/keryx is a separate binary and cannot import the handlers
// package — so only a test keeps them from drifting. The cases are the same
// ones that pin the server side.
//
// The 4,97-against-5 row is the one that matters: the first version of this
// helper picked precision per value and rendered it "have 5.0", reintroducing
// exactly the lie cli-sanning row D removed.
func TestShortfall_MirrorsServerFormat(t *testing.T) {
	cases := []struct {
		need, have float64
		want       string
	}{
		{12, 11.7, "need 12, have 11.7, 0.3 short"},
		{5, 4.97, "need 5, have 4.97, 0.03 short"},
		{40, 12, "need 40, have 12, 28 short"},
		{200, 50, "need 200, have 50, 150 short"},
	}
	for _, c := range cases {
		if got := shortfall(c.need, c.have); got != c.want {
			t.Errorf("shortfall(%v, %v) = %q, want %q", c.need, c.have, got, c.want)
		}
	}
}
