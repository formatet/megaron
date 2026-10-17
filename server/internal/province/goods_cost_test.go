package province

import (
	"math"
	"testing"
)

func TestGoodsAffordable(t *testing.T) {
	for _, tc := range []struct {
		have, need float64
		want       bool
	}{
		{23.999999999999996, 24, true}, {15.999999999999998, 16, true},
		{23.99999, 24, false}, {0, 24, false}, {25, 24, true},
		{math.NaN(), 24, false}, {24, math.NaN(), false},
	} {
		if got := GoodsAffordable(tc.have, tc.need); got != tc.want {
			t.Errorf("GoodsAffordable(%g,%g)=%v want %v", tc.have, tc.need, got, tc.want)
		}
	}
}
