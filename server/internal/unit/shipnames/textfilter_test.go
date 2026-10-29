package shipnames

import (
	"testing"

	"formatet/megaron/server/internal/textfilter"
)

// The filter must never touch a name the game itself suggests.
func TestNoSuggestedShipNameIsCoarse(t *testing.T) {
	for culture, styles := range pools {
		for style, names := range styles {
			for _, n := range names {
				if out, hits := textfilter.Name(n, func(int) int { return 0 }); hits != 0 {
					t.Errorf("%s/%s ship name %q filtered to %q", culture, style, n, out)
				}
			}
		}
	}
}
