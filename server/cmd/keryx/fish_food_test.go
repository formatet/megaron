package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestColonyFounded_FishFoodBalance(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`{"name":"Amnissos","grain_amount":0,"grain_net_per_tick":-5,"food_net_per_tick":0}`, "food production covers consumption"},
		{`{"name":"Coast","food_net_per_tick":-2,"loyalty_warning":"Meals may become short"}`, "Stored food feeds people until it runs out"},
	} {
		out := capturePrint(t, func() { printColonyFoundedGrainLine(notificationItem{Body: json.RawMessage(tc.body)}) })
		if !strings.Contains(out, tc.want) || strings.Contains(out, "No grain reserve") || strings.Contains(out, "doesn't feed itself") {
			t.Fatal(out)
		}
	}
}
func TestFoundingForecast_FishNeedsNoGrainReserve(t *testing.T) {
	var p colonizePreview
	if err := json.Unmarshal([]byte(`{"grain":{"base_per_tick":0,"fish_per_tick":5,"est_net_per_tick":0,"consumption_per_tick":5,"food_net_per_tick":0,"seed":0}}`), &p); err != nil {
		t.Fatal(err)
	}
	out := captureForecastOutput(t, "Coast", &p)
	if strings.Contains(out, "loyalty can fall") || strings.Contains(out, "starves") {
		t.Fatal(out)
	}
	if !strings.Contains(out, "fish ~5.0") {
		t.Fatal(out)
	}
}

func TestFoundingNotificationRunway_ClientUsesFoodNet(t *testing.T) {
	for _, body := range []string{
		`{"name":"Fishcity","grain_net_per_tick":-5,"food_net_per_tick":0,"grain_ticks":240}`,
		`{"name":"Dustcity","grain_net_per_tick":-1e-15,"food_net_per_tick":-1e-15}`,
		`{"name":"Legacy","grain_net_per_tick":-1e-15,"grain_ticks":675539944105574400}`,
	} {
		out := capturePrint(t, func() { printColonyFoundedGrainLine(notificationItem{Body: json.RawMessage(body)}) })
		if strings.Contains(out, "675539944105574400") || strings.Contains(out, "falls short") || strings.Contains(out, "-0.0") || strings.Contains(out, "doesn't feed itself") {
			t.Fatal(out)
		}
	}
	out := capturePrint(t, func() {
		printColonyFoundedGrainLine(notificationItem{Body: json.RawMessage(`{"grain_net_per_tick":-5,"food_net_per_tick":-0.3}`)})
	})
	if !strings.Contains(out, "falls short by 0.3") || strings.Contains(out, "5.0") {
		t.Fatal(out)
	}
}
