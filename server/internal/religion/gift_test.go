package religion

import "testing"

func TestGiftFactorDurationAndRest(t *testing.T) {
	if GiftFactor(10, 10) != 1 || GiftFactor(5, 10) != 1 || GiftFactor(10, 0) != 1 {
		t.Fatal("traditional, short or unreckoned gift must be 1")
	}
	if GiftFactor(20, 10) != 2 || GiftFactor(1000, 10) != MaxGiftFactor {
		t.Fatal("gift factor must be worth/baseline, capped")
	}
	for _, tc := range []struct {
		base int
		r    float64
		want int
	}{{6, 1, 6}, {6, 2, 12}, {6, 1.25, 8}, {6, 99, 30}, {25, 2, 50}} {
		if got := DurationTicks(tc.base, tc.r); got != tc.want {
			t.Errorf("DurationTicks(%d,%v)=%d want %d", tc.base, tc.r, got, tc.want)
		}
	}
	for _, tc := range []struct {
		cd    int
		r     float64
		level int
		want  int
	}{{24, 1, 1, 24}, {24, 1, 2, 12}, {24, 1, 3, 8}, {25, 1, 2, 13}, {24, 2, 2, 24}, {24, 1, 0, 0}, {0, 1, 1, 0}} {
		if got := RestTicks(tc.cd, tc.r, tc.level); got != tc.want {
			t.Errorf("RestTicks(%d,%v,%d)=%d want %d", tc.cd, tc.r, tc.level, got, tc.want)
		}
	}
}
