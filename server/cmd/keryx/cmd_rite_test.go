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
		{"god still waiting", 12, "awaiting next departure; 12 game days remaining wait"},
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
	if ready != "cooldown" || detail != "7 game days remaining" {
		t.Fatalf("consumed gift waiting for god: got %q/%q", ready, detail)
	}
	ready, detail = prayerReadiness(riteAvailablePrayer{Affordable: true})
	if ready != "yes" || detail != "—" {
		t.Fatalf("consumed gift after wait: got %q/%q", ready, detail)
	}
}

func TestPrayerPaymentDistinguishesEverydayFromLegacy(t *testing.T) {
	everyday := prayerPaymentText(riteAvailablePrayer{GodID: "potnia"})
	if !strings.Contains(everyday, "not spent") || !strings.Contains(everyday, "god answers") || strings.Contains(everyday, "silent") {
		t.Fatal(everyday)
	}
	legacy := prayerPaymentText(riteAvailablePrayer{})
	if !strings.Contains(legacy, "silent") || !strings.Contains(legacy, "not spent") {
		t.Fatal(legacy)
	}
}

func TestRiteReadinessConcreteBlockerPrecedesWait(t *testing.T) {
	for _, reason := range []string{"no Temple of Potnia", "insufficient kharis"} {
		status, _ := prayerReadiness(riteAvailablePrayer{UnavailableReason: reason, CooldownRemainingDays: 24})
		if status != "no: "+reason {
			t.Fatalf("known blocker hidden by waiting: %q", status)
		}
	}
}

func TestRiteReadinessCanPrepareAlternativeGift(t *testing.T) {
	var p riteAvailablePrayer
	if err := json.Unmarshal([]byte(`{"affordable":false,"can_prepare_offering":true,"unavailable_reason":"insufficient offering"}`), &p); err != nil {
		t.Fatal(err)
	}
	ready, detail := prayerReadiness(p)
	if ready != "compose" || detail != "traditional gift unavailable; use --offer" {
		t.Fatalf("existing alternative gift hidden: %q/%q", ready, detail)
	}
}

func TestOfferingPreviewPreservesChosenDecimalAmounts(t *testing.T) {
	gift := map[string]float64{"incense": 5.25, "wine": 0.04}
	got := formatOffering(gift)
	if got != "incense×5.25 wine×0.04" {
		t.Fatalf("preview changes the chosen gift: %q", got)
	}
	if gift["incense"] != 5.25 || gift["wine"] != 0.04 {
		t.Fatal("preview mutated the gift")
	}
	if got := formatOffering(map[string]float64{"grain": 10, "silver": 3}); got != "grain×10 silver×3" {
		t.Fatalf("traditional display changed: %s", got)
	}
}

// rite takes --province like status/place/build: city name, province ID or
// settlement ID, own cities only (GPT-player logs 2026-10-24).
func TestRiteSettlementByProvince(t *testing.T) {
	markers := []map[string]any{
		{"id": "p-own", "settlement_id": "s-own", "name": "Knossos", "own": true},
		{"id": "p-far", "settlement_id": "s-far", "name": "Pylos", "own": false},
	}
	for arg, want := range map[string]string{"knossos": "s-own", "p-own": "s-own", "s-own": "s-own", "Pylos": "", "p-far": ""} {
		if got := riteSettlementByProvince(markers, arg); got != want {
			t.Errorf("riteSettlementByProvince(%q) = %q, want %q", arg, got, want)
		}
	}
}
