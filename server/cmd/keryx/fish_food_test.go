package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestColonyFounded_FishFoodBalance(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`{"name":"Amnissos","grain_amount":0,"grain_net_per_tick":-5,"food_net_per_tick":0,"loyalty_warning":""}`, "food production covers consumption"},
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
	if err := json.Unmarshal([]byte(`{"grain":{"base_per_tick":0,"fish_per_tick":5,"est_net_per_tick":0,"consumption_per_tick":5,"food_net_per_tick":0,"seed":0,"loyalty_warning":""}}`), &p); err != nil {
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
