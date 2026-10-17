package handlers

import (
	"strings"
	"testing"
)

func TestFoundingTimberWarning_WoodFogAndCedar(t *testing.T) {
	for _, tc := range []struct {
		goods   map[string]float64
		unknown int
		wanted  string
	}{
		{map[string]float64{}, 0, "No timber within reach"},
		{map[string]float64{}, 4, "No timber on known ground"},
		{map[string]float64{"timber": 1}, 0, ""},
		{map[string]float64{"cedar": 1}, 0, ""},
	} {
		got := foundingTimberWarning(tc.goods, tc.unknown)
		if tc.wanted == "" {
			if got != "" {
				t.Fatal(got)
			}
		} else if !strings.HasPrefix(got, tc.wanted) {
			t.Fatalf("got %q want %q", got, tc.wanted)
		}
	}
}
