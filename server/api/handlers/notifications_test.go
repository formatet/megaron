package handlers

// splitKinds is pure (no DB) so it's covered here directly. The rest of
// NotificationsHandler.List needs a live pgxpool — skipped per plan
// (megaron_ekonomi_legibilitet_plan.md DEL B), verify manually against
// world c3c289e5 on CT 126 instead.

import (
	"reflect"
	"testing"
)

func TestSplitKinds(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single", "SitosIntervention", []string{"SitosIntervention"}},
		{"multiple", "TradeDelivery,UnitArrived", []string{"TradeDelivery", "UnitArrived"}},
		{"trims whitespace", " TradeDelivery , UnitArrived ", []string{"TradeDelivery", "UnitArrived"}},
		{"drops empty segments", "TradeDelivery,,UnitArrived", []string{"TradeDelivery", "UnitArrived"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitKinds(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("splitKinds(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}
