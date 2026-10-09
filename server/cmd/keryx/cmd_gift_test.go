package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGiftTransferResolvesContactedForeignCity(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing auth")
		}
		switch r.Method {
		case "GET":
			if !strings.HasSuffix(r.URL.Path, "/provinces/source/trade/destinations") {
				t.Errorf("wrong discovery %s", r.URL.Path)
			}
			json.NewEncoder(w).Encode([]map[string]any{{"settlement_id": "dest", "name": "Nostos", "owner_name": "Another Wanax", "own": false}})
		case "POST":
			var req map[string]any
			json.NewDecoder(r.Body).Decode(&req)
			if req["destination_id"] != "dest" || req["good_key"] != "silver" {
				t.Errorf("wrong payload %v", req)
			}
			json.NewEncoder(w).Encode(map[string]any{"kind": "gift", "arrival_tick": 42, "travel_ticks": 7})
		}
	}))
	defer srv.Close()
	old, oldJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = old, oldJSON }()
	cfg = &Config{Server: srv.URL, Token: "token", WorldID: "world", ProvinceID: "source"}
	jsonMode = false
	cmd := transferCmd()
	cmd.SetArgs([]string{"--dest", "Nostos", "--good", "silver", "--qty", "3"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !strings.Contains(out, "Gift dispatched") || !strings.Contains(out, "arrives day 42") {
		t.Fatalf("calls=%d output=%s", calls, out)
	}
}
func TestGiftNoticeExactLossAndOwner(t *testing.T) {
	out, err := captureStdout(t, func() error {
		printNotificationDetail(nil, notificationItem{Kind: "GiftDelivered", Body: json.RawMessage(`{"origin_name":"Kyme","destination_name":"Nostos","good_key":"silver","credited_quantity":5.125,"lost_quantity":19.875,"returned_quantity":25,"reason":"storage_full","owner_changed":true,"actual_recipient_name":"New Wanax","recipient_name":"Old Wanax"}`)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"5.125 silver received", "19.875 lost", "25 sent home", "New Wanax", "Old Wanax", "storage full"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
}
