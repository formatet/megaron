package economy

import (
	"math"
	"testing"
)

// The tithe must only ever bite where silver is actually moving — the whole
// reason it was chosen over a standing cult upkeep (Timothy 2026-07-22). A city
// with no trade income pays nothing, which is exactly what a city with no silver
// (both tin holders were measured at zero the same evening) needs.
func TestTithe_OnlyOnReligiousTradeWithATemple(t *testing.T) {
	cases := []struct {
		name         string
		silver       float64
		religious    bool
		hasTemple    bool
		wantToTemple float64
		wantToSeller float64
	}{
		{"wine sold by a temple city — tithed", 100, true, true, 10, 90},
		{"grain sold by a temple city — untouched", 100, false, true, 0, 100},
		{"wine sold where no temple stands — no priests to collect", 100, true, false, 0, 100},
		{"neither religious nor templed", 100, false, false, 0, 100},
		{"a trivial sale is waved through", 5, true, true, 0, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			temple, seller := Tithe(tc.silver, tc.religious, tc.hasTemple)
			if math.Abs(temple-tc.wantToTemple) > 1e-9 {
				t.Errorf("temple got %.2f, want %.2f", temple, tc.wantToTemple)
			}
			if math.Abs(seller-tc.wantToSeller) > 1e-9 {
				t.Errorf("seller got %.2f, want %.2f", seller, tc.wantToSeller)
			}
		})
	}
}

// Nothing may be created or lost in the split — the tithe is a transfer out of
// the seller's income, not a mint.
func TestTithe_ConservesSilver(t *testing.T) {
	for _, silver := range []float64{10, 37.5, 1000, 999999} {
		temple, seller := Tithe(silver, true, true)
		if math.Abs((temple+seller)-silver) > 1e-9 {
			t.Errorf("silver %.2f split into %.2f + %.2f", silver, temple, seller)
		}
	}
}
