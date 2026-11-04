package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func TestPlacementsLiftedNotificationShowsText(t *testing.T) {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	printNotificationSummary(nil, notificationItem{Kind: "PlacementsLifted", Body: json.RawMessage(`{"text":"3 citizens left their work in Gournia: grain at (30, 45) ×2, olive press (oil) ×1 (men were drafted)."}`)})
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "3 citizens left their work in Gournia") {
		t.Errorf("output = %q", out)
	}
}
