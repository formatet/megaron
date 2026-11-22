package main

import "testing"

// Player text shows at most one decimal (Timothy 2026-11-18).
func TestTenths_AtMostOneDecimal(t *testing.T) {
	for _, c := range []struct {
		in   float64
		want string
	}{
		{0.333, "0.3"}, {-0.333, "-0.3"}, {0.04, "<0.1"}, {-0.04, "−<0.1"}, {0, "0"}, {12, "12"}, {2.54, "2.5"},
	} {
		if got := tenths(c.in); got != c.want {
			t.Errorf("tenths(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNotificationAmount_OneDecimal(t *testing.T) {
	if got := notificationAmount(0.333); got != "0.3" {
		t.Errorf("notificationAmount(0.333) = %q, want 0.3", got)
	}
}
