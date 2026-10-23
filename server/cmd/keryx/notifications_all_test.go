package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestNotificationsAllIncludesEveryKindWithoutGrouping(t *testing.T) {
	for _, unread := range []bool{false, true} {
		t.Run(fmt.Sprint(unread), func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/notifications") {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}
				if r.URL.Query().Get("exclude") != "" || r.URL.Query().Get("kind") != "" {
					t.Errorf("all still filters kinds: %s", r.URL)
				}
				if (r.URL.Query().Get("unread") == "true") != unread {
					t.Errorf("unread filter: %s", r.URL)
				}
				fmt.Fprint(w, `{"notifications":[{"kind":"SitosIntervention","body":{}},{"kind":"SitosIntervention","body":{}}],"unread":2}`)
			}))
			defer srv.Close()
			before, beforeJSON := cfg, jsonMode
			defer func() { cfg, jsonMode = before, beforeJSON }()
			cfg = &Config{Server: srv.URL, WorldID: "test-world"}
			jsonMode = false
			cmd := notificationsCmd()
			args := []string{"--all"}
			if unread {
				args = append(args, "--unread")
			}
			cmd.SetArgs(args)
			out, err := captureStdout(t, cmd.Execute)
			if err != nil {
				t.Fatal(err)
			}
			if requests != 1 {
				t.Fatalf("requests=%d want1 without hidden-kind probes", requests)
			}
			if strings.Count(out, "Food support was given") != 2 || strings.Contains(out, "×2") {
				t.Fatalf("all grouped or hid archive rows: %s", out)
			}
		})
	}
}

func TestNotificationsAllRejectsConflictingKindFilters(t *testing.T) {
	for _, flag := range []string{"--kind", "--exclude", "--mark-read"} {
		t.Run(flag, func(t *testing.T) {
			cmd := notificationsCmd()
			args := []string{"--all", flag}
			if flag != "--mark-read" {
				args = append(args, "BuildComplete")
			}
			cmd.SetArgs(args)
			_, err := captureStdout(t, cmd.Execute)
			if err == nil || !strings.Contains(err.Error(), "all") {
				t.Fatalf("expected incompatible-filter error, got%v", err)
			}
		})
	}
}

func TestNotificationsAllJSONPreservesArchive(t *testing.T) {
	const raw = `{"notifications":[{"kind":"SitosIntervention","body":{"historical":true}}],"unread":1,"future_field":42}`
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.RawQuery != "" {
			t.Errorf("filtered all JSON: %s", r.URL)
		}
		fmt.Fprint(w, raw)
	}))
	defer srv.Close()
	before, beforeJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = before, beforeJSON }()
	cfg = &Config{Server: srv.URL, WorldID: "w"}
	jsonMode = true
	cmd := notificationsCmd()
	cmd.SetArgs([]string{"--all"})
	out, err := captureStdout(t, cmd.Execute)
	var actual, expected any
	json.Unmarshal([]byte(raw), &expected)
	decodeErr := json.Unmarshal([]byte(out), &actual)
	if err != nil || decodeErr != nil || requests != 1 || !reflect.DeepEqual(actual, expected) {
		t.Fatalf("requests=%d err=%v output=%s", requests, err, out)
	}
}
