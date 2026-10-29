package province

import (
	"testing"

	"formatet/megaron/server/internal/textfilter"
)

// The filter must never touch a name the game itself gives a city or a Wanax.
func TestNoGeneratedNameIsCoarse(t *testing.T) {
	all := append([]string(nil), WanaxNamePool...)
	for _, names := range CultureSettlementNames {
		all = append(all, names...)
	}
	for _, n := range all {
		if out, hits := textfilter.Name(n, func(int) int { return 0 }); hits != 0 {
			t.Errorf("generated name %q filtered to %q", n, out)
		}
	}
}
