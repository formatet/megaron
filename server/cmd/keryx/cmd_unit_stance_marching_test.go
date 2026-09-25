package main

import (
	"strings"
	"testing"
)

// A stance order to a marching unit: the receipt must say the Runner has to
// catch up, where, and when (megaron_styrande_beslut §11) — and a plain field
// unit's receipt stays as it was.
func TestStanceDispatchLine(t *testing.T) {
	cases := []struct {
		name string
		resp map[string]any
		want []string
	}{
		{"catch up on the march",
			map[string]any{"catch_up": "on_the_march", "intercept_q": 2.0, "intercept_r": 0.0},
			[]string{"must catch up with it at (2,0) in 2 game-days", "bites where the unit stops"}},
		{"only after it stops",
			map[string]any{"catch_up": "at_destination", "intercept_q": 8.0, "intercept_r": 0.0},
			[]string{"no runner can overtake it", "destination at (8,0), reaching it in 2 game-days", "applies where it stopped"}},
		{"standing field unit",
			map[string]any{},
			[]string{"the runner reaches it in 2 game-days — it applies on delivery."}},
	}
	for _, tc := range cases {
		got := stanceDispatchLine(tc.resp, "sentry", "abcd1234", "in 2 game-days")
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: %q lacks %q", tc.name, got, w)
			}
		}
	}
}
