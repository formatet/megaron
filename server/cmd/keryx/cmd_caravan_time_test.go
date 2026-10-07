package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTransferDisplaysServerTicksAndShip(t *testing.T) {
	const dest = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/v1/worlds/world/provinces/source/trade" || r.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"arrival_tick": 42, "travel_ticks": 7, "travel_min": 99999, "ship_name": "Naia"})
	}))
	defer srv.Close()
	oldCfg, oldJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = oldCfg, oldJSON }()
	cfg = &Config{Server: srv.URL, Token: "token", WorldID: "world", ProvinceID: "source"}
	jsonMode = false
	cmd := transferCmd()
	cmd.SetArgs([]string{"--dest", dest, "--good", "grain", "--qty", "3"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"arrives tick 42 (journey: 7 ticks)", "Naia", "sails home empty"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
	if calls != 1 || strings.Contains(out, "99999") {
		t.Errorf("calls=%d output=%s", calls, out)
	}
}

func TestTradeAcceptShowsBothLegsFromRecipientPerspective(t *testing.T) {
	for _, kind := range []string{"buy", "sell"} {
		t.Run(kind, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/trade-accept") {
					t.Errorf("unexpected %s", r.Method)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"kind": kind, "goods_arrival_tick": 42, "silver_arrival_tick": 49, "quantity": 3, "good_key": "grain", "silver_paid": 2})
			}))
			defer srv.Close()
			oldCfg, oldJSON := cfg, jsonMode
			defer func() { cfg, jsonMode = oldCfg, oldJSON }()
			cfg = &Config{Server: srv.URL, WorldID: "world"}
			jsonMode = false
			cmd := tradeAcceptCmd()
			cmd.SetArgs([]string{"--id", "offer"})
			out, err := captureStdout(t, cmd.Execute)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"goods arrive tick 42", "silver arrives tick 49"} {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q in %s", want, out)
				}
			}
			direction := "outgoing"
			if kind == "sell" {
				direction = "incoming"
			}
			// A sell offer's recipient sends silver and receives the offered goods;
			// a buy offer's recipient sends goods and receives silver.
			if !strings.Contains(out, direction) {
				t.Errorf("wrong direction: %s", out)
			}
		})
	}
}

func TestTradeOutboxDoesNotInferDeliveryFromWallTime(t *testing.T) {
	got := deliveryETALine(nil, map[string]any{"goods_arrival_tick": float64(42), "silver_arrival_tick": float64(49), "goods_arrives_at": "2000-01-01T00:00:00Z"})
	if got != "  goods scheduled tick 42 · silver scheduled tick 49" {
		t.Fatal(got)
	}
}
