package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInboxAcknowledgesDisplayedLettersInTextAndJSON(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "json"}[jsonOutput], func(t *testing.T) {
			oldCfg, oldJSON := cfg, jsonMode
			defer func() { cfg, jsonMode = oldCfg, oldJSON }()
			var reads []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" && r.URL.Path == "/api/v1/worlds/w/messengers/inbox" {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`[{"id":"unread","from_name":"Remote","message":"Hello","read_at":null},{"id":"read","read_at":"already"}]`))
					return
				}
				if r.Method == "POST" && r.URL.Path == "/api/v1/worlds/w/messengers/unread/read" {
					reads = append(reads, r.URL.Path)
					w.Write([]byte(`{"read":true}`))
					return
				}
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
				http.NotFound(w, r)
			}))
			defer srv.Close()
			cfg = &Config{Server: srv.URL, WorldID: "w"}
			jsonMode = jsonOutput
			cmd := inboxCmd()
			if err := cmd.RunE(cmd, nil); err != nil {
				t.Fatal(err)
			}
			if len(reads) != 1 {
				t.Fatalf("read receipts%v want only displayed unread ID", reads)
			}
		})
	}
}

func TestInboxHistoryShowsPastLettersWithoutDeadReplyCommands(t *testing.T) {
	oldCfg, oldJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = oldCfg, oldJSON }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Query().Get("history") != "true" {
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`[{"id":"old","status":"arrived","from_name":"Kyme","message":"Old letter","reply_text":"My reply","read_at":"already"}]`))
	}))
	defer srv.Close()
	cfg = &Config{Server: srv.URL, WorldID: "w"}
	jsonMode = false
	var err error
	out := capturePrint(t, func() { cmd := inboxCmd(); cmd.SetArgs([]string{"--history"}); err = cmd.Execute() })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Old letter") || !strings.Contains(out, "My reply") || strings.Contains(out, "keryx reply") {
		t.Fatal(out)
	}
}
