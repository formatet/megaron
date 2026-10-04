package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgoraStatusNeverRequestsPassword(t *testing.T) {
	for _, tc := range []struct{ name, payload, text string }{
		{"disabled", `{"enabled":false}`, "not enabled"},
		{"pending", `{"enabled":true,"state":"pending"}`, "first city"},
		{"provisioning", `{"enabled":true,"state":"provisioning"}`, "being created"},
		{"ready", `{"enabled":true,"state":"ready","user_id":"@agamemnon:agora.test","homeserver":"agora.test"}`, "@agamemnon:agora.test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/agora" || r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("wrong account request: %s %s", r.Method, r.URL.Path)
				}
				_, _ = w.Write([]byte(tc.payload))
			}))
			defer server.Close()
			oldCfg, oldJSON := cfg, jsonMode
			defer func() { cfg, jsonMode = oldCfg, oldJSON }()
			cfg = &Config{Server: server.URL, Token: "test-token"}
			jsonMode = false
			cmd := agoraCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || !strings.Contains(out.String(), tc.text) {
				t.Fatalf("calls=%d output=%q", calls, out.String())
			}
			password, _, err := cmd.Find([]string{"password"})
			if err != nil {
				t.Fatal(err)
			}
			if commandNeedsWorld(cmd) || commandNeedsWorld(password) {
				t.Fatal("chat account must not depend on world selection")
			}
		})
	}
}

func TestAgoraPasswordExplicitOnlyAndNeverSaved(t *testing.T) {
	for _, asJSON := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "json"}[asJSON], func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/agora/password" || r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("wrong password request")
				}
				_, _ = w.Write([]byte(`{"password":"explicit-secret","user_id":"@agamemnon:agora.test","homeserver":"agora.test"}`))
			}))
			defer server.Close()
			oldCfg, oldJSON := cfg, jsonMode
			defer func() { cfg, jsonMode = oldCfg, oldJSON }()
			cfg = &Config{Server: server.URL, Token: "test-token"}
			jsonMode = asJSON
			configPath := filepath.Join(t.TempDir(), "config.json")
			t.Setenv("POLEIA_CONFIG", configPath)
			cmd := agoraCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{"password"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || !strings.Contains(out.String(), "explicit-secret") {
				t.Fatalf("calls=%d output=%q", calls, out.String())
			}
			if asJSON {
				var body map[string]string
				if err := json.Unmarshal(out.Bytes(), &body); err != nil || body["password"] != "explicit-secret" {
					t.Fatalf("invalid explicit JSON: %v", err)
				}
			}
			if _, err := os.Stat(configPath); !os.IsNotExist(err) {
				t.Fatalf("password command persisted configuration: %v", err)
			}
		})
	}
}

func TestAgoraPasswordFailureNeverPrintsUpstreamBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"error":"upstream-secret"}`))
	}))
	defer server.Close()
	oldCfg := cfg
	defer func() { cfg = oldCfg }()
	cfg = &Config{Server: server.URL}
	cmd := agoraCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"password"})
	err := cmd.Execute()
	if err == nil || strings.Contains(err.Error()+out.String(), "upstream-secret") {
		t.Fatalf("unsafe failure: %v %q", err, out.String())
	}
}

func TestAgoraNotificationOffersExplicitPasswordCommand(t *testing.T) {
	output := capturePrint(t, func() {
		printNotificationDetail(nil, notificationItem{Kind: "agora_ready", Body: json.RawMessage(`{"user_id":"@agamemnon:agora.test","password":"never-render-this"}`)})
	})
	if !strings.Contains(output, "keryx agora password") || strings.Contains(output, "never-render-this") {
		t.Fatalf("chat notice is not actionable or exposes a secret: %q", output)
	}
}
