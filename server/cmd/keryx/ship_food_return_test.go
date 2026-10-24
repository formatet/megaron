package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestShipFoodReturnNamesHomecomingRatherThanRemoteDeparture(t *testing.T) {
	out, err := captureStdout(t, func() error {
		printNotificationDetail(nil, notificationItem{Kind: "ShipCameHomeForFood", Body: json.RawMessage(`{"unit_id":"ship","name":"Nike","crew_after":20,"reason":"food_for_way_home"}`)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Nike came home early", "food for the way back only", "20 crew home"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
	for _, wrong := range []string{"turning home", "sailing slower", "half strength", "arrives"} {
		if strings.Contains(out, wrong) {
			t.Fatal(out)
		}
	}
	old, err := captureStdout(t, func() error {
		printNotificationDetail(nil, notificationItem{Kind: "UnitReturnedStarving", Body: json.RawMessage(`{"name":"Nike","crew_after":4}`)})
		return nil
	})
	if err != nil || !strings.Contains(old, "starved to half strength") || !strings.Contains(old, "turning home") {
		t.Fatalf("historical notice changed: %v %s", err, old)
	}
}
