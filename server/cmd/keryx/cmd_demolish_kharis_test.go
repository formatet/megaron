package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestDemolishTempleRequiresExplicitConfirmation(t *testing.T) {
	prevStdin := os.Stdin
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	writer.Close()
	os.Stdin = input
	defer func() { os.Stdin = prevStdin; input.Close() }()
	for _, tc := range []struct {
		name, kind               string
		yes, machine, wantDelete bool
	}{
		{"temple guarded", "temple", false, false, false}, {"json guarded", "temple", false, true, false}, {"temple confirmed", "temple", true, false, true}, {"machine confirmed", "temple", true, true, true}, {"market unchanged", "market", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deleted := false
			quote := map[string]any{"god_id": "potnia", "level": 3, "penalty_per_level": 10, "kharis_loss": 4, "kharis_before": 5, "kharis_after": 1, "cap_before": 100, "cap_after": 50}
			round3Client(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					deleted = true
					json.NewEncoder(w).Encode(map[string]any{"building_type": tc.kind, "level": 3, "citizens_lifted": 2, "demolition_kharis": quote})
					return
				}
				b := map[string]any{"id": "temple-id", "type": tc.kind}
				if tc.kind == "temple" {
					b["god_id"] = "potnia"
					b["demolition_kharis"] = quote
				}
				json.NewEncoder(w).Encode([]any{b})
			})
			old := jsonMode
			jsonMode = tc.machine
			defer func() { jsonMode = old }()
			cmd := demolishCmd()
			args := []string{"--type", tc.kind}
			if tc.yes {
				args = append(args, "--yes")
			}
			cmd.SetArgs(args)
			out, err := captureStdout(t, cmd.Execute)
			if deleted != tc.wantDelete {
				t.Fatalf("deleted=%v want %v; err=%v", deleted, tc.wantDelete, err)
			}
			if !tc.wantDelete {
				if err != nil || !strings.Contains(out, "Not demolished. Run again with --yes to confirm.") {
					t.Fatalf("missing confirmation error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.machine {
				var got map[string]any
				if json.Unmarshal([]byte(out), &got) != nil {
					t.Fatalf("polluted machine output: %s", out)
				}
			} else if tc.kind == "temple" {
				for _, s := range []string{"potnia is angered", "−4 kharis now (5 → 1; floor 1)", "100 → 50", "The lower ceiling takes no additional kharis"} {
					if !strings.Contains(out, s) {
						t.Fatalf("missing %q: %s", s, out)
					}
				}
			}
		})
	}
}
func TestTempleDemolitionNotificationSummary(t *testing.T) {
	out := capturePrint(t, func() {
		printNotificationSummary(nil, notificationItem{Kind: "TempleDemolitionWrath", Body: json.RawMessage(`{"god_id":"velchanos","kharis_loss":10,"kharis_before":80,"kharis_after":70,"cap_before":75,"cap_after":50}`)})
	})
	for _, s := range []string{"velchanos is angered", "80 → 70", "75 → 50", "The lower ceiling takes no additional kharis"} {
		if !strings.Contains(out, s) {
			t.Fatalf("missing %q: %s", s, out)
		}
	}
}

func TestTempleDemolitionUsesGodDisplayName(t *testing.T) {
	text := demolitionKharisText(demolitionKharis{GodID: "sea_daimons_stillness", GodName: "Sea Daimons of Stillness"})
	if !strings.Contains(text, "Sea Daimons of Stillness is angered") || strings.Contains(text, "sea_daimons_stillness") {
		t.Fatal(text)
	}
}
