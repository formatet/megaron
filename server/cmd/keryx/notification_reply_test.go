package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Letter and trade-offer notices say how to answer (byggplan A.1).
func TestLetterNotificationsNameTheReplyCommand(t *testing.T) {
	for kind, body := range map[string]string{
		"MessengerArrival":  `{"sender_id":"p-writer","from":"Minos","name":"Knossos"}`,
		"MessengerReturned": `{"counterpart_wanax_id":"p-writer","replied":true,"reply":"yes"}`,
		"OfferDeclined":     `{"counterpart_wanax_id":"p-writer","kind":"buy","silver":5}`,
	} {
		out := capturePrint(t, func() {
			printNotificationSummary(nil, notificationItem{Kind: kind, Body: json.RawMessage(body)})
		})
		if !strings.Contains(out, "keryx letters p-writer") {
			t.Errorf("%s: no reply command in %q", kind, out)
		}
	}
	out := capturePrint(t, func() {
		printNotificationSummary(nil, notificationItem{Kind: "UnitArrived", Body: json.RawMessage(`{"sender_id":"p-x"}`)})
	})
	if strings.Contains(out, "keryx letters") {
		t.Errorf("UnitArrived offered a reply: %q", out)
	}
}

func TestRiteAnsweredSummary(t *testing.T) {
	out := capturePrint(t, func() {
		printNotificationSummary(nil, notificationItem{Kind: "RiteAnswered", Body: json.RawMessage(`{"prayer_name":"Potnia's Vision","name":"Knossos","message":"Potnia marks copper at (9,0)"}`)})
	})
	if !strings.Contains(out, "Potnia's Vision at Knossos: Potnia marks copper at (9,0)") {
		t.Fatalf("summary %q", out)
	}
}

func TestOfferFoodTextMatchesTheWeb(t *testing.T) {
	got := offerFoodText(map[string]float64{"grain": 28, "silver": 3}, map[string]float64{"grain": 29.6, "fish": 4}, 55.8, "Lato")
	if !strings.Contains(got, "takes 28 of Lato's 29 grain") || !strings.Contains(got, "leaves 5 food, less than Lato eats in a day (55.8)") {
		t.Fatalf("got %q", got)
	}
	if offerFoodText(map[string]float64{"silver": 3}, map[string]float64{"grain": 29}, 55.8, "Lato") != "" {
		t.Fatal("a gift without food must say nothing")
	}
}

func TestHiketeiaHintNamesConfirm(t *testing.T) {
	err := hiketeiaHint(fmt.Errorf("These are the survivors of a fallen Wanax, under the gods' protection until they found a city. Attacking them breaks hiketeia: your realm loses 70 kharis."))
	if !strings.Contains(err.Error(), "add --confirm") {
		t.Fatalf("%v", err)
	}
	if other := hiketeiaHint(fmt.Errorf("too far")); strings.Contains(other.Error(), "--confirm") {
		t.Fatalf("%v", other)
	}
}

func TestStormFishingLine(t *testing.T) {
	if got := stormFishingLine(map[string]any{"storm_no_fishing": 1.0}); got != "Storm over 1 fishing ground — no fishing there while it stands." {
		t.Fatalf("%q", got)
	}
	if stormFishingLine(map[string]any{"storm_no_fishing": 0.0}) != "" {
		t.Fatal("calm weather must say nothing")
	}
}
