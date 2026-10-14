package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func openingNotice(t *testing.T, title string, returning bool) notificationItem {
	t.Helper()
	body, err := json.Marshal(map[string]any{"title": title, "returning": returning, "host_id": "private-host-id", "paragraphs": []string{
		"Time. A day passes every 7 minutes.", "An army crosses a plains hex in 3 days.", "Your Host carries 987 grain and 654 silver.", "Food first. Below 321 people it falls.", "The gods. You are safe for 43 days.", "Orders travel with a Runner.", "Bronze needs copper and tin.", "Rules: ? (Codex). Problems: Report.",
	}})
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
		}
		if err := json.Unmarshal(n.Body, &b); err != nil {
			t.Fatal(err)
		}
		for _, p := range b.Paragraphs {
			if strings.Count(detail, p) != 1 {
				t.Errorf("%q must appear once in %q", p, detail)
			}
		}
		if strings.Contains(detail, "private-host-id") || strings.Contains(detail, "\"paragraphs\"") {
			t.Fatalf("raw JSON leaked: %s", detail)
		}
	}
}
