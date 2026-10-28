package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCounterOfferUsesOneAtomicDispatch(t *testing.T) {
	posts := 0
	const original = "00000000-0000-0000-0000-000000001234"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if testWorldTimeResponse(w, r) {
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/provinces"):
			json.NewEncoder(w).Encode([]map[string]any{{"id": "source", "settlement_id": "home", "name": "Nostos", "own": true, "is_capital": true}, {"id": "target", "settlement_id": "dest", "name": "Petras", "knowledge": "known"}})
		case strings.HasSuffix(r.URL.Path, "/wanaxes"):
			w.Write([]byte(`[]`))
		case strings.HasSuffix(r.URL.Path, "/transport-options"):
			w.Write([]byte(`{"options":[{"by":"land","available":true}]}`))
		case r.Method == "POST":
			posts++
			if !strings.HasSuffix(r.URL.Path, "/messengers/"+original+"/trade-counter") {
				t.Errorf("unexpected independent command %s", r.URL.Path)
			}
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if _, ok := body["destination_id"]; ok {
				t.Errorf("counter geography must be server-bound: %v", body)
			}
			offer, _ := body["trade_offer"].(map[string]any)
			if offer["kind"] != "sell" || offer["offer_good"] != "tin" || offer["by"] != "land" {
				t.Errorf("wrong revised terms %v", body)
			}
			w.WriteHeader(201)
			w.Write([]byte(`{"id":"new","arrives_at":"2026-10-28T20:00:00Z"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old, j := cfg, jsonMode
	defer func() { cfg, jsonMode = old, j }()
	cfg = &Config{Server: srv.URL, Token: "token", WorldID: "world", ProvinceID: "source"}
	jsonMode = false
	cmd := tradeOfferCmd()
	cmd.SetArgs([]string{"--counter", original, "--offer-good", "tin", "--offer-qty", "20", "--want-silver", "12", "--by", "land"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatal(err)
	}
	if posts != 1 {
		t.Fatalf("made %d posts, want one atomic dispatch", posts)
	}
	if !strings.Contains(out, "Counter-offer") {
		t.Fatal("counter not identified in receipt", out)
	}
}
