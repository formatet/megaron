package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNotificationDayRecordedOnly(t *testing.T) {
	for _, tt := range []struct{ raw, want string }{
		{`{"world_tick":0,"body":{"world_tick":8}}`, testTickTime(0)},
		{`{"world_tick":12,"body":{"world_tick":8}}`, testTickTime(12)},
		{`{"world_tick":null,"body":{"world_tick":0}}`, testTickTime(0)},
		{`{"created_at":"2026-10-18T08:00:00Z","body":{}}`, "time unavailable"},
	} {
		var n notificationItem
		if err := json.Unmarshal([]byte(tt.raw), &n); err != nil {
			t.Fatal(err)
		}
		if got := notificationDay(n, testTimeClient()); got != tt.want {
			t.Fatalf("%s: %s want %s", tt.raw, got, tt.want)
		}
		text := capturePrint(t, func() { printNotificationRow(testTimeClient(), n) })
		if !strings.Contains(text, "["+tt.want+"]") {
			t.Fatal(text)
		}
	}
}
func TestNotificationGarrisonUrgencyClearedPerCity(t *testing.T) {
	item := func(kind string, tick int, city string) notificationItem {
		return notificationItem{Kind: kind, WorldTick: &tick, Body: json.RawMessage(`{"settlement_id":"` + city + `"}`)}
	}
	old := item("GarrisonStarvationStarted", 1, "a")
	ended := item("GarrisonStarvationEnded", 2, "a")
	other := item("GarrisonStarvationStarted", 1, "b")
	restart := item("GarrisonStarvationStarted", 3, "a")
	if urgent := urgentNotificationIndices([]notificationItem{old, ended, other}); len(urgent) != 1 || !urgent[2] {
		t.Fatalf("recovery left old city urgent: %v", urgent)
	}
	if urgent := urgentNotificationIndices([]notificationItem{old, ended, restart}); len(urgent) != 1 || !urgent[2] {
		t.Fatalf("restart not urgent: %v", urgent)
	}
}
func TestAbsenceNoticeTextAndWatchGameDay(t *testing.T) {
	for _, tt := range []struct{ kind, body, want string }{
		{"GarrisonStarvationStarted", `{"name":"Coast","food_net_per_tick":24}`, "Soldiers need grain even when the city has a food surplus"},
		{"GarrisonStarvationEnded", `{"name":"Coast"}`, "no longer losing men to hunger"},
		{"KharisBandChanged", `{"from_band":"Indifferent","to_band":"Wrathful","kharis":3}`, "Indifferent → Wrathful (3.0)"},
	} {
		body := json.RawMessage(tt.body)
		text := capturePrint(t, func() { printNotificationSummary(testTimeClient(), notificationItem{Kind: tt.kind, Body: body}) })
		if !strings.Contains(text, tt.want) {
			t.Fatal(text)
		}
		tick := 0
		text = capturePrint(t, func() { printWatchMessage(testTimeClient(), wsMsg{Kind: tt.kind, Payload: body, WorldTick: &tick}) })
		if !strings.Contains(text, "["+testTickTime(0)+"]") || !strings.Contains(text, tt.want) {
			t.Fatal(text)
		}
	}
	if !isNoisyNotificationKind("SitosGranaryRelease") || isNoisyNotificationKind("GarrisonStarvationStarted") {
		t.Fatal("archived granary release must be quiet, hunger must stay visible")
	}
}

func TestNotificationCivilianAndGarrisonHungerIndependent(t *testing.T) {
	item := func(kind string, tick int) notificationItem {
		return notificationItem{Kind: kind, WorldTick: &tick, Body: json.RawMessage(`{"settlement_id":"a"}`)}
	}
	civil := item("CivilianStarvationStarted", 1)
	army := item("GarrisonStarvationStarted", 2)
	civilEnd := item("CivilianStarvationEnded", 3)
	armyEnd := item("GarrisonStarvationEnded", 4)
	if u := urgentNotificationIndices([]notificationItem{civil, army, civilEnd}); len(u) != 1 || !u[1] {
		t.Fatalf("civilian recovery cleared army: %v", u)
	}
	if u := urgentNotificationIndices([]notificationItem{civil, army, armyEnd}); len(u) != 1 || !u[0] {
		t.Fatalf("army recovery cleared civilians: %v", u)
	}
	for _, tt := range []struct{ kind, body, want string }{
		{"CivilianStarvationStarted", `{"name":"Tylissos","unmet":0.03}`, "the meal fell short by <0.1 grain"},
		{"CivilianStarvationEnded", `{"name":"Tylissos"}`, "people were fed again"},
		{"CivilianStarvationEnded", `{"name":"Tylissos","reason":"no_longer_owned_or_active"}`, "warning closed"},
	} {
		tick := 7
		body := json.RawMessage(tt.body)
		row := capturePrint(t, func() {
			printNotificationRow(testTimeClient(), notificationItem{Kind: tt.kind, Body: body, WorldTick: &tick})
		})
		watch := capturePrint(t, func() { printWatchMessage(testTimeClient(), wsMsg{Kind: tt.kind, Payload: body, WorldTick: &tick}) })
		for _, text := range []string{row, watch} {
			if !strings.Contains(text, testTickTime(7)) || !strings.Contains(text, tt.want) {
				t.Fatal(text)
			}
		}
	}
}
