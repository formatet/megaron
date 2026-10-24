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

func TestLettersListAndWanaxThreadUsePersonEndpoint(t *testing.T) {
	for _, detail := range []bool{false, true} {
		for _, raw := range []bool{false, true} {
			t.Run(fmt.Sprintf("detail=%v/json=%v", detail, raw), func(t *testing.T) {
				saved, savedJSON := cfg, jsonMode
				defer func() { cfg, jsonMode = saved, savedJSON }()
				calls := []string{}
				list := `{"items":[{"counterpart_id":"person","counterpart_name":"Myrine","preview":"Last line","state":"waiting_you","reply_before":"2099-10-24T23:18:00Z"}],"future":42}`
				thread := `{"counterpart_name":"Myrine","items":[{"id":"letter","sender_name":"Myrine","direction":"incoming","message":"Greetings","goods":{"silver":5.125},"sent_at":"2026-10-24T08:00:00Z","offer_state":"accepted","trade_offer":{"kind":"sell","offer_qty":2.125,"offer_good":"copper","want_silver":5},"actions":["reply"],"reply_before":"2099-10-24T23:18:00Z"}]}`
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls = append(calls, r.Method+" "+r.URL.Path)
					if r.URL.Path == "/api/v1/worlds/w/letters/person" {
						fmt.Fprint(w, thread)
					} else if r.URL.Path == "/api/v1/worlds/w/letters" {
						fmt.Fprint(w, list)
					} else {
						t.Errorf("unexpected %s", r.URL)
						w.WriteHeader(404)
					}
				}))
				defer srv.Close()
				cfg = &Config{Server: srv.URL, WorldID: "w"}
				jsonMode = raw
				cmd := lettersCmd()
				if detail {
					cmd.SetArgs([]string{"myrine"})
				} else {
					cmd.SetArgs([]string{})
				}
				out, err := captureStdout(t, cmd.Execute)
				if err != nil {
					t.Fatal(err)
				}
				wantCalls := 1
				if detail {
					wantCalls = 2
				}
				if len(calls) != wantCalls {
					t.Fatal(calls)
				}
				if raw {
					var got, want any
					payload := list
					if detail {
						payload = thread
					}
					json.Unmarshal([]byte(out), &got)
					json.Unmarshal([]byte(payload), &want)
					if !reflect.DeepEqual(got, want) {
						t.Fatal(out)
					}
				} else if detail {
					for _, x := range []string{"Myrine", "Greetings", "5.125 silver", "Agreed: 2.125 copper for 5 silver", "Send with their runner (leaves", "reply --id letter"} {
						if !strings.Contains(out, x) {
							t.Fatalf("missing %q: %s", x, out)
						}
					}
				} else {
					if !strings.Contains(out, "Myrine · Waiting on you") || !strings.Contains(out, "Last line") {
						t.Fatal(out)
					}
				}
			})
		}
	}
}
func TestLettersStatesAndRemovedFlags(t *testing.T) {
	for key, want := range map[string]string{"waiting_you": "Waiting on you", "waiting_them": "Waiting on them", "done": "Done"} {
		if lettersState(key) != want {
			t.Fatal(key)
		}
	}
	for _, flag := range []string{"case", "subject"} {
		if messageCmd().Flags().Lookup(flag) != nil {
			t.Fatal(flag)
		}
	}
	if inboxCmd().Flags().Lookup("queue") != nil {
		t.Fatal("old queue exposed")
	}
}
