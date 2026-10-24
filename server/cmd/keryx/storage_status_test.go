package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// Order 29 (GPT20: ~90 silver lost without a word). The overflow names what
// was lost and why; the warning names the good and the days left.
func TestStorageNotificationsNameTheLoss(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"name": "MalliaWest", "good_key": "silver", "cap": 60, "lost_quantity": 90, "reason": "transfer_delivery"})
	out := capturePrint(t, func() { printStorageNotification(notificationItem{Kind: "StorageOverflow", Body: raw}) })
	for _, want := range []string{"MalliaWest", "lost 90 silver", "an internal transfer", "capacity 60"} {
		if !strings.Contains(out, want) {
			t.Fatalf("overflow line %q lacks %q", strings.TrimSpace(out), want)
		}
	}
	raw, _ = json.Marshal(map[string]any{"name": "MalliaWest", "good_key": "silver", "cap": 960, "full_in_ticks": 3})
	out = capturePrint(t, func() { printStorageNotification(notificationItem{Kind: "StorageCapacityWarning", Body: raw}) })
	if !strings.Contains(out, "silver storage full in 3") {
		t.Fatalf("warning line %q lacks the game days left", strings.TrimSpace(out))
	}
	lines := storageWarningLines(map[string]any{"storage": []any{
		map[string]any{"good_key": "silver", "amount": 900.0, "cap": 960.0, "warning": true, "full_in_ticks": 2.0},
		map[string]any{"good_key": "grain", "amount": 10.0, "cap": 1e6, "warning": false, "full_in_ticks": 1e5},
	}})
	if len(lines) != 1 || !strings.Contains(lines[0], "silver") {
		t.Fatalf("status storage lines = %v, want one silver warning", lines)
	}
}
