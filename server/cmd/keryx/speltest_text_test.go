package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"formatet/megaron/server/internal/province"
)

func TestPlaytestForecast_FullFoodDemandAndReserve(t *testing.T) {
	var p colonizePreview
	if err := json.Unmarshal([]byte(`{"grain":{"base_per_tick":0.5,"fish_per_tick":5,"est_net_per_tick":0,"consumption_per_tick":5,"food_net_per_tick":0.5,"seed":0,"loyalty_warning":"Food reserves are running down: improve food production"}}`), &p); err != nil {
		t.Fatal(err)
	}
	out := captureForecastOutput(t, "Coast", &p)
	for _, part := range []string{"grain ~0.5 + fish ~5.0", "consumption ~5.0", "net +0.5", "Food reserves are running down: improve food production"} {
		if !strings.Contains(out, part) {
			t.Errorf("missing %q: %s", part, out)
		}
	}
	if strings.Contains(out, "starves") {
		t.Fatal(out)
	}
}

func TestPlaytestTimberWarning_FollowsCatalogue(t *testing.T) {
	spec := province.BuildingSpecs[province.BuildingTemple]
	original := spec.Costs["timber"]
	spec.Costs["timber"] = 13.25
	defer func() { spec.Costs["timber"] = original }()
	if out := timberBottleneckWarning(0); !strings.Contains(out, "temple (13.25)") {
		t.Fatal(out)
	}
}

func TestPlaytestNotifications_UUIDGiftAndInternalDelivery(t *testing.T) {
	for _, gift := range []string{`true`, `"de291886-44cb-4f4e-8b52-6d488f8fd053"`} {
		n := notificationItem{Kind: "MetropolisFounded", Body: json.RawMessage(`{"name":"Knossos","poseidon_gift":` + gift + `}`)}
		out := capturePrint(t, func() { printNotificationSummary(nil, n) })
		if !strings.Contains(out, "Knossos") || !strings.Contains(out, "Poseidon grants a galley") {
			t.Fatalf("gift %s: %s", gift, out)
		}
	}
	n := notificationItem{Kind: "TradeDelivery", Body: json.RawMessage(`{"quantity":20,"good_key":"grain"}`)}
	out := capturePrint(t, func() { printNotificationSummary(nil, n) })
	if !strings.Contains(out, "Delivery arrived: 20 grain") || strings.Contains(out, "Trade") {
		t.Fatal(out)
	}
}

func TestPlaytestReadinessAndHelp(t *testing.T) {
	ready, _ := prayerReadiness(riteAvailablePrayer{UnavailableReason: "no temple"})
	if !strings.Contains(ready, "no temple") {
		t.Fatal(ready)
	}
	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "POLEIA_CONFIG") {
		t.Fatal(out.String())
	}
	root = newRootCommand()
	out.Reset()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--does-not-exist"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected flag error")
	}
	if strings.Contains(out.String(), "Usage:") {
		t.Fatal(out.String())
	}
}

func TestPlaytestKharisReading_DoesNotRoundUp(t *testing.T) {
	if got := prayerKharisReading(4.99); got != "4.9" {
		t.Fatalf("4.99 displayed as %s", got)
	}
	if got := prayerKharisReading(5); got != "5.0" {
		t.Fatal(got)
	}
	out := unsustainableReason(0, 0, 1, 2, 57, 1416)
	if !strings.Contains(out, "grain stock 57") || !strings.Contains(out, "silver stock 1416") || strings.Contains(out, "current stock") {
		t.Fatal(out)
	}
}
