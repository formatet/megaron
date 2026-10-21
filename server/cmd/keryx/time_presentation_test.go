package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

var testTimeAnchor = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

func testTimeClient() *Client {
	zero := 0
	return &Client{tickSecondsFetched: true, tickSeconds: 60, worldTick: &zero, tickAnchor: testTimeAnchor}
}
func testTickTime(tick int) string {
	return clockTime(testTimeAnchor.Add(time.Duration(tick) * time.Minute))
}
func testWorldTimeResponse(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != "GET" || !strings.HasPrefix(r.URL.Path, "/api/v1/worlds/") || strings.Count(r.URL.Path, "/") != 4 {
		return false
	}
	json.NewEncoder(w).Encode(map[string]any{"current_tick": 0, "tick_seconds": 60, "tick_anchor_at": testTimeAnchor.Format(time.RFC3339)})
	return true
}

func TestWorldClockMidnightHistoryAndDates(t *testing.T) {
	now := time.Date(2026, 10, 21, 23, 50, 0, 0, keryxTZ)
	for _, tc := range []struct {
		at   time.Time
		want string
	}{
		{now, "today 23:50"},
		{now.Add(20 * time.Minute), "tomorrow 00:10"},
		{now.AddDate(0, 0, -1), "yesterday 23:50"},
		{now.AddDate(0, 0, 3), "Sat 23:50"},
		{now.AddDate(0, 0, -3), "Sun 23:50"},
		{now.AddDate(0, 0, 10), "31 Oct 23:50"},
		{now.AddDate(1, 0, 0), "21 Oct 2027 23:50"},
	} {
		if got := clockTimeAt(tc.at, now); got != tc.want {
			t.Fatalf("%v: %q want %q", tc.at, got, tc.want)
		}
	}
}

func TestAbsoluteWorldClockUsesAnchorNotTicksOrDelivery(t *testing.T) {
	current := 100
	anchor := time.Date(2026, 10, 21, 22, 50, 0, 0, keryxTZ)
	c := &Client{tickSecondsFetched: true, tickSeconds: 60, worldTick: &current, tickAnchor: anchor}
	for _, tc := range []struct {
		tick int
		at   time.Time
	}{
		{0, anchor.Add(-100 * time.Minute)}, {40, anchor.Add(-time.Hour)}, {110, anchor.Add(10 * time.Minute)}, {200, anchor.Add(100 * time.Minute)},
	} {
		if got := formatWorldTime(c, tc.tick); got != clockTime(tc.at) || strings.Contains(got, "arrived") {
			t.Fatalf("tick %d: %q", tc.tick, got)
		}
	}
	n := notificationItem{WorldTick: new(int), CreatedAt: "2026-01-01T00:00:00Z"}
	if got := notificationDay(n, c); got != clockTime(anchor.Add(-100*time.Minute)) {
		t.Fatalf("tick zero overwritten by delivery: %s", got)
	}
	if got := notificationDay(notificationItem{CreatedAt: n.CreatedAt}, c); got != "time unavailable" {
		t.Fatal("delivery is not event time", got)
	}
	for _, value := range []any{-1, 1.5, "bad", nil} {
		if got := formatWorldTime(c, value); got != "time unavailable" {
			t.Fatal(value, got)
		}
	}
	if got := formatWorldTime(nil, 200); got != "time unavailable" {
		t.Fatal(got)
	}
	c.tickSeconds = 3600
	if got := formatWorldTime(c, 110); got != clockTime(anchor.Add(10*time.Hour)) {
		t.Fatal("cadence ignored", got)
	}
	mission := expeditionMissionText(&expeditionMission{LengthTicks: 30, TurnTick: 115, HomeByTick: 130}, c)
	if !strings.Contains(mission, "30 days") || !strings.Contains(mission, "turns home by "+clockTime(anchor.Add(15*time.Hour))) || !strings.Contains(mission, "home by "+clockTime(anchor.Add(30*time.Hour))) || strings.Contains(mission, "day 115") {
		t.Fatal(mission)
	}
}
