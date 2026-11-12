package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func openingNotice(t *testing.T, title string, returning bool) notificationItem {
	t.Helper()
	body, err := json.Marshal(map[string]any{"title": title, "returning": returning, "host_id": "private-host-id", "paragraphs": []string{
		"You are a Wanax, a ruler.", "A game day passes every 7 minutes.",
	}, "steps": []string{"Look at the land.", "Click your Host on the map.", "Found your city.", "Put your citizens to work.", "Read the Codex (?)."}})
	if err != nil {
		t.Fatal(err)
	}
	return notificationItem{Kind: "OpeningDispatch", Level: 4, Body: body}
}

func TestOpeningDispatchSummaryAndArchiveWatchDetails(t *testing.T) {
	for _, title := range []string{"Your people await you", "Begin again"} {
		n := openingNotice(t, title, title == "Begin again")
		summary := capturePrint(t, func() { printNotificationSummary(nil, n) })
		if strings.TrimSpace(summary) != title {
			t.Fatalf("summary = %q, want archived title %q", summary, title)
		}
		// Archive row and watch both call this shared detail formatter.
		detail := capturePrint(t, func() { printNotificationDetail(nil, n) })
		var b struct {
			Paragraphs []string `json:"paragraphs"`
			Steps      []string `json:"steps"`
		}
		if err := json.Unmarshal(n.Body, &b); err != nil {
			t.Fatal(err)
		}
		for _, p := range append(b.Paragraphs, b.Steps...) {
			if strings.Count(detail, p) != 1 {
				t.Errorf("%q must appear once in %q", p, detail)
			}
		}
		if !strings.Contains(detail, "1. Look at the land.") || !strings.Contains(detail, "5. Read the Codex (?).") {
			t.Fatalf("steps must print as a numbered list: %q", detail)
		}
		if strings.Contains(detail, "private-host-id") || strings.Contains(detail, "\"paragraphs\"") {
			t.Fatalf("raw JSON leaked: %s", detail)
		}
	}
}

func TestWanaxEnteredSaysWhoAndWhatTheySayNeverWhere(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"wanax_id": "x", "wanax_name": "Minos", "message": "Peace."})
	out := capturePrint(t, func() { printNotificationDetail(nil, notificationItem{Kind: "WanaxEntered", Level: 3, Body: raw}) })
	if !strings.Contains(out, "A new Wanax has entered the Thalassa: Minos. They say: “Peace.”") {
		t.Fatalf("got %q", out)
	}
	raw, _ = json.Marshal(map[string]any{"wanax_name": "Minos", "message": ""})
	out = capturePrint(t, func() { printNotificationDetail(nil, notificationItem{Kind: "WanaxEntered", Level: 3, Body: raw}) })
	if strings.Contains(out, "They say") || !strings.Contains(out, "Minos.") {
		t.Fatalf("empty message: %q", out)
	}
}
