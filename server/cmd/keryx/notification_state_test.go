package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNotificationDayRecordedOnly(t *testing.T) {
	for _, tt := range []struct{ raw, want string }{
		{`{"world_tick":0,"body":{"world_tick":8}}`, "Day #0"},
		{`{"world_tick":12,"body":{"world_tick":8}}`, "Day #12"},
		{`{"world_tick":null,"body":{"world_tick":0}}`, "Day #0"},
		{`{"created_at":"2026-10-18T08:00:00Z","body":{}}`, "Day unknown"},
	} {
		var n notificationItem
		if err := json.Unmarshal([]byte(tt.raw), &n); err != nil {
			t.Fatal(err)
		}
		if got := notificationDay(n); got != tt.want {
			t.Fatalf("%s: %s want %s", tt.raw, got, tt.want)
		}
		text := capturePrint(t, func() { printNotificationRow(nil, n) })
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
		text := capturePrint(t, func() { printNotificationSummary(nil, notificationItem{Kind: tt.kind, Body: body}) })
		if !strings.Contains(text, tt.want) {
			t.Fatal(text)
		}
		tick := 0
		text = capturePrint(t, func() { printWatchMessage(nil, wsMsg{Kind: tt.kind, Payload: body, WorldTick: &tick}) })
		if !strings.Contains(text, "[Day #0]") || !strings.Contains(text, tt.want) {
			t.Fatal(text)
		}
	}
	if !isNoisyNotificationKind("SitosGranaryRelease") || isNoisyNotificationKind("GarrisonStarvationStarted") {
		t.Fatal("archived granary release must be quiet, hunger must stay visible")
	}
}
