package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBattleBegunArchiveAndWatchText(t *testing.T) {
	for _, joined := range []bool{false, true} {
		body, _ := json.Marshal(map[string]any{"opponent_name": "Idomeneus", "place": "Petras", "q": 1, "r": 0, "occurred_at": "2026-10-24T18:45:00Z", "joined": joined, "battle_id": "private-battle-id"})
		out := capturePrint(t, func() { printNotificationDetail(nil, notificationItem{Kind: "BattleBegun", Body: body}) })
		for _, want := range []string{"Idomeneus — ", "at Petras", "18:45:00Z"} {
			if !strings.Contains(out, want) {
				t.Fatalf("missing %q: %s", want, out)
			}
		}
		if strings.Contains(out, "private-battle-id") {
			t.Fatal(out)
		}
		if joined != strings.Contains(out, "joined the battle") {
			t.Fatal(out)
		}
	}
}
