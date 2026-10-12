package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRiteDeparturePendingStatus(t *testing.T) {
	for _, test := range []struct {
		name string
		wait int
		want string
	}{
		{"god still waiting", 12, "awaiting next departure; 12 days remaining wait"},
		{"god ready but gift unused", 0, "awaiting next departure"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var prayer riteAvailablePrayer
			if err := json.Unmarshal([]byte(`{"departure_pending":true,"affordable":false}`), &prayer); err != nil {
				t.Fatal(err)
			}
			prayer.CooldownRemainingDays = test.wait
			ready, detail := prayerReadiness(prayer)
			if ready != "pending" || detail != test.want {
				t.Fatalf("pending gift: got %q/%q, want pending/%q", ready, detail, test.want)
			}
			if strings.Contains(detail, "active") {
				t.Fatalf("departure gift must not invent a duration: %q", detail)
			}
		})
	}
}

func TestRiteReadinessAfterDeparture(t *testing.T) {
	ready, detail := prayerReadiness(riteAvailablePrayer{CooldownRemainingDays: 7})
	if ready != "cooldown" || detail != "7 days remaining" {
		t.Fatalf("consumed gift waiting for god: got %q/%q", ready, detail)
	}
	ready, detail = prayerReadiness(riteAvailablePrayer{Affordable: true})
	if ready != "yes" || detail != "—" {
		t.Fatalf("consumed gift after wait: got %q/%q", ready, detail)
	}
}
