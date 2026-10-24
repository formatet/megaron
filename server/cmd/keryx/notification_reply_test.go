package main

import (
	"encoding/json"
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
