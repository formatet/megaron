package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExploreTicksOrderAndPreview(t *testing.T) {
	for _, preview := range []bool{false, true} {
		for _, alias := range []bool{false, true} {
			t.Run(map[bool]string{true: "preview", false: "dispatch"}[preview]+map[bool]string{true: "alias", false: "unit"}[alias], func(t *testing.T) {
				calls := 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if preview {
						if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/march-preview") || r.URL.Query().Get("ticks") != "12" || r.URL.Query().Get("intent") != "explore" {
							t.Errorf("wrong preview %s %s", r.Method, r.URL)
						}
						w.Write([]byte(`{"available":false,"reason":"unknown_terrain"}`))
					} else {
						var body map[string]any
						json.NewDecoder(r.Body).Decode(&body)
						if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/march") || body["ticks"] != float64(12) || body["intent"] != "explore" {
							t.Errorf("wrong dispatch %s %+v", r.Method, body)
						}
						w.Write([]byte(`{"expedition":{"area_q":5,"area_r":-3,"length_ticks":12,"turn_tick":106,"home_by_tick":112}}`))
					}
				}))
				defer srv.Close()
				oldCfg, oldJSON := cfg, jsonMode
				defer func() { cfg, jsonMode = oldCfg, oldJSON }()
				cfg = &Config{Server: srv.URL, WorldID: "world"}
				jsonMode = false
				cmd := unitMarchCmd()
				if alias {
					cmd = marchAliasCmd()
				}
				args := []string{"--unit", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "--target", "5,-3", "--intent", "explore", "--ticks", "12"}
				if preview {
					args = append(args, "--preview")
				}
				cmd.SetArgs(args)
				out, err := captureStdout(t, cmd.Execute)
				if err != nil {
					t.Fatal(err)
				}
				if calls != 1 {
					t.Fatalf("calls=%d", calls)
				}
				if !preview && (!strings.Contains(out, "12 days") || !strings.Contains(out, "home by day 112")) {
					t.Fatalf("missing mission: %s", out)
				}
			})
		}
	}
}
func TestExploreTicksRejectOtherIntents(t *testing.T) {
	for _, intent := range []string{"", "colonize", "land"} {
		cmd := unitMarchCmd()
		cmd.SetArgs([]string{"--unit", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "--target", "0,0", "--intent", intent, "--ticks", "12"})
		if _, err := captureStdout(t, cmd.Execute); err == nil || !strings.Contains(err.Error(), "--ticks requires") {
			t.Fatalf("intent=%q err=%v", intent, err)
		}
	}
}
func TestExpeditionMissionLifecycleText(t *testing.T) {
	var u unitRow
	if err := json.Unmarshal([]byte(`{"expedition":{"area_q":9,"area_r":2,"length_ticks":12,"turn_tick":8,"home_by_tick":14,"homeward":true,"turn_reason":"area_known"}}`), &u); err != nil {
		t.Fatal(err)
	}
	text := expeditionMissionText(u.Expedition)
	for _, want := range []string{"(9,2)", "12 days", "returning home", "area explored", "home by day 14"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s: %s", want, text)
		}
	}
}
