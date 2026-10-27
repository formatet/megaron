package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompletionDispatchCityAndPlayerNames(t *testing.T) {
	for _, tc := range []struct{ kind, body, want string }{
		{"BuildComplete", `{"building_type":"lumbermill","settlement_name":"Tylissos","hex_q":0,"hex_r":7}`, "Lumbermill built at (0,7) for Tylissos"},
		{"TrainComplete", `{"unit_type":"merchantman","settlement_name":"Tylissos","count":1}`, "Emporos ready in Tylissos"},
		{"TrainComplete", `{"unit_type":"war_galley","settlement_name":"Petras"}`, "War Galley ready in Petras"},
		{"TrainComplete", `{"unit_type":"elite_infantry","settlement_name":"Petras"}`, "Bronze Guard ready in Petras"},
	} {
		out, err := captureStdout(t, func() error {
			printNotificationSummary(nil, notificationItem{Kind: tc.kind, Body: json.RawMessage(tc.body)})
			return nil
		})
		if err != nil || !strings.Contains(out, tc.want) {
			t.Fatalf("%s got%q err%v", tc.kind, out, err)
		}
		for _, raw := range []string{"merchantman", "war_galley", "elite_infantry"} {
			if strings.Contains(out, raw) {
				t.Fatalf("raw type %s in %q", raw, out)
			}
		}
	}
}
