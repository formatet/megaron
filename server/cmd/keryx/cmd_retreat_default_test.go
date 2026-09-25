package main

import "testing"

// retreat_at_loss is the fraction of starting strength LEFT when the side
// breaks (combat.sideRouts: cur/start <= threshold) — the text must say the
// losses, not echo the fraction as if it were the losses.
func TestDescribeRetreatDefault(t *testing.T) {
	q := 0.25
	cases := []struct {
		at   *float64
		hold bool
		want string
	}{
		{&q, false, "retreat at 75% losses (when down to 25% of starting strength)"},
		{nil, true, "hold to the last man"},
		{nil, false, "by the troops' loyalty — the more loyal their city, the longer they hold"},
	}
	for _, c := range cases {
		if got := describeRetreatDefault(c.at, c.hold); got != c.want {
			t.Errorf("describeRetreatDefault(%v, %v) = %q, want %q", c.at, c.hold, got, c.want)
		}
	}
}
