package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGodsAngeredNoticeMatchesTheWeb(t *testing.T) {
	out, _ := captureStdout(t, func() error {
		printNotificationSummary(nil, notificationItem{Kind: "GodsAngered", Body: json.RawMessage(`{"words":2,"kharis_lost":10}`)})
		return nil
	})
	if !strings.Contains(out, "The gods heard what was written in your name. Your realm loses 10 kharis.") {
		t.Fatalf("keryx line = %q", out)
	}
}
