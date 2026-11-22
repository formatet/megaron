package main

import (
	"strings"
	"testing"
)

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

// Army upkeep printed "0.07 food" and a gods-angered loss used full precision
// (found 2026-11-22 in `keryx status`): both go through tenths now.
func TestTrimFloatAndGodsAngeredOneDecimal(t *testing.T) {
	for in, want := range map[float64]string{0.07: "0.1", 0.04: "<0.1", 2.25: "2.3", 3: "3", 120: "120"} {
		if got := trimFloat(in); got != want {
			t.Errorf("trimFloat(%v) = %q, want %q", in, got, want)
		}
	}
	if got := godsAngeredText(1.234); !strings.Contains(got, "loses 1.2 kharis") {
		t.Errorf("godsAngeredText(1.234) = %q, want 'loses 1.2 kharis'", got)
	}
}
