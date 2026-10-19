package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEqualFoodCityUsesServerTotalsAndGoods(t *testing.T) {
	var sett map[string]any
	err := json.Unmarshal([]byte(`{"grain_prod_rate":999,"grain_consum_rate":999,"army_upkeep":{"grain":999,"food":2,"silver":0},"food_net_per_tick_after_upkeep":5,"food":{"coverage_ticks":12.5,"production_per_tick":10,"consumption_per_tick":3,"food_net_per_tick":7,"goods":{"grain":100,"fish":900,"oil":40,"wine":60}}}`), &sett)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Food for 12.5 days", "100 grain, 900 fish, 40 oil, 60 wine"} {
		if !strings.Contains(cityFoodLine(sett), text) {
			t.Fatal(cityFoodLine(sett))
		}
	}
	if line := peopleFoodLine(sett); !strings.Contains(line, "production 10.0 − people eat 3.0 = net +7.0") || strings.Contains(line, "999") {
		t.Fatal(line)
	}
	if line := armyFoodLine(sett); !strings.Contains(line, "2.0 food") || !strings.Contains(line, "+5.0") || strings.Contains(line, "grain") {
		t.Fatal(line)
	}
	food := sett["food"].(map[string]any)
	food["goods"] = map[string]any{"grain": 0.0, "fish": 0.0000001, "oil": 0.008, "wine": 0.0001}
	if got := cityFoodLine(sett); !strings.Contains(got, "0 grain, <0.0001 fish, 0.008 oil, 0.0001 wine") {
		t.Fatal(got)
	}
	if got := cityFoodLine(map[string]any{}); got != "Food stores not reported." {
		t.Fatal(got)
	}
}

func TestEqualFoodRosterPrefersAuthoritativeFoodAndKeepsManifest(t *testing.T) {
	var u unitRow
	if err := json.Unmarshal([]byte(`{"category":"naval","status":"marching","crew":40,"upkeep_food_per_tick":0.5,"upkeep_grain_per_tick":999,"upkeep_silver_per_tick":0,"provision_days":7,"provisions_aboard":true,"provisions_by_good":{"grain":1,"fish":9,"oil":2,"wine":3}}`), &u); err != nil {
		t.Fatal(err)
	}
	if got := unitUpkeepLine(u); got != "Upkeep: 0.5 food, 0 silver per day" {
		t.Fatal(got)
	}
	got := formatSize(nil, u)
	for _, want := range []string{"food for 7 days", "1 grain, 9 fish, 2 oil, 3 wine"} {
		if !strings.Contains(got, want) {
			t.Fatal(got)
		}
	}
	u.ProvisionsByGood = map[string]float64{"grain": 0, "fish": 0.0000001, "oil": 0.008, "wine": 0.0001}
	if got := formatSize(nil, u); !strings.Contains(got, "0 grain, <0.0001 fish, 0.008 oil, 0.0001 wine") {
		t.Fatal(got)
	}
}

func TestEqualFoodNoticesPreserveHistoricalGrainMeaning(t *testing.T) {
	for _, tc := range []struct{ kind, body, want string }{
		{"GarrisonStarvationStarted", `{"name":"Coast","food_category":true}`, "Soldiers share"},
		{"GarrisonStarvationStarted", `{"name":"Coast"}`, "Soldiers need grain even when"},
		{"CivilianStarvationStarted", `{"unmet":0.03,"food_category":true}`, "<0.1 food"},
		{"CivilianStarvationStarted", `{"unmet":0.03}`, "<0.1 grain"},
		{"UnitAttrition", `{"reason":"food_shortage","disbanded":true}`, "no food"},
		{"UnitAttrition", `{"reason":"grain_shortage","disbanded":true}`, "no grain"},
	} {
		got := capturePrint(t, func() { printNotificationSummary(nil, notificationItem{Kind: tc.kind, Body: json.RawMessage(tc.body)}) })
		if !strings.Contains(got, tc.want) {
			t.Fatalf("%s: %s", tc.kind, got)
		}
	}
}
