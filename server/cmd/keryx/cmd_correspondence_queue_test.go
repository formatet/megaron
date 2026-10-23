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

func TestInboxQueueUsesServerActionsAndPreservesMissedOffersWithoutReading(t *testing.T) {
	const payload = `{"items":[{"id":"poor","from_name":"Kyme","to_name":"Nostos","message":"Open offer","queue_group":"waiting_you","offer_state":"pending","actions":["read","trade_decline"],"missing_payment":"Need 5 silver","trade_offer":{"kind":"sell","offer_good":"copper","offer_qty":2,"want_silver":5}},{"id":"expired","from_name":"Kyme","to_name":"Nostos","message":"Missed offer","queue_group":"completed","offer_state":"expired","actions":["read"],"expires_at":"2026-10-23T10:00:00Z"}],"next_cursor":"opaque+/=","future":42}`
	for _, mode := range []bool{false, true} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			saved, savedJSON := cfg, jsonMode
			defer func() { cfg, jsonMode = saved, savedJSON }()
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.URL.Path != "/api/v1/worlds/w/messengers/queue" || r.URL.Query().Get("limit") != "2" || r.URL.Query().Get("cursor") != "old+/=" {
					t.Errorf("unexpected %s %s", r.Method, r.URL)
				}
				fmt.Fprint(w, payload)
			}))
			defer srv.Close()
			cfg = &Config{Server: srv.URL, WorldID: "w"}
			jsonMode = mode
			cmd := inboxCmd()
			cmd.SetArgs([]string{"--queue", "--limit", "2", "--cursor", "old+/="})
			out, err := captureStdout(t, cmd.Execute)
			if err != nil || calls != 1 {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
			if mode {
				var got, want any
				json.Unmarshal([]byte(out), &got)
				json.Unmarshal([]byte(payload), &want)
				if !reflect.DeepEqual(got, want) {
					t.Fatal(out)
				}
				return
			}
			for _, text := range []string{"Waiting on you", "Past correspondence", "Expired without answer", "Need 5 silver", "trade-decline --id poor", "--cursor \"opaque+/=\"", "Answer deadline:", "Buy 2 copper"} {
				if !strings.Contains(out, text) {
					t.Fatalf("missing %q: %s", text, out)
				}
			}
			for _, absent := range []string{"trade-accept --id poor", "trade-decline --id expired", "reply --id expired"} {
				if strings.Contains(out, absent) {
					t.Fatalf("dead action %q: %s", absent, out)
				}
			}
		})
	}
}

func TestInboxQueueRejectsConflictingOrInvalidFlagsBeforeHTTP(t *testing.T) {
	for _, args := range [][]string{{"--queue", "--history"}, {"--queue", "--limit", "0"}, {"--queue", "--limit", "101"}, {"--cursor", "x"}, {"--limit", "10"}} {
		cmd := inboxCmd()
		cmd.SetArgs(args)
		if _, err := captureStdout(t, cmd.Execute); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
