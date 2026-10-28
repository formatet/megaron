package main

import (
	"encoding/json"
	"github.com/spf13/cobra"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTransportChoiceFlagsRejectInvalidBeforeNetwork(t *testing.T) {
	for _, makeCmd := range []func() *cobra.Command{transferCmd, routeCreateCmd, tradeOfferCmd} {
		cmd := makeCmd()
		if cmd.Flags().Lookup("by") == nil {
			t.Errorf("%s missing --by land|sea", cmd.Name())
			continue
		}
		args := []string{"--by", "air"}
		switch cmd.Name() {
		case "transfer":
			args = append(args, "--good", "grain", "--qty", "1", "--dest", "x")
		case "route":
			args = append(args, "--to", "x", "--out", "grain:2")
		case "trade-offer":
			args = append(args, "--to", "x", "--want-good", "grain", "--want-qty", "1", "--offer-silver", "2")
		}
		cmd.SetArgs(args)
		_, err := captureStdout(t, cmd.Execute)
		if err == nil || !strings.Contains(err.Error(), "--by") {
			t.Errorf("%s invalid mode: %v", cmd.Name(), err)
		}
	}
}

func TestTransferCarriesExplicitModeToPreviewAndDispatch(t *testing.T) {
	for _, by := range []string{"land", "sea"} {
		t.Run(by, func(t *testing.T) {
			const dest = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
			preview, posted := false, false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if testWorldTimeResponse(w, r) {
					return
				}
				if strings.HasSuffix(r.URL.Path, "/trade/preview") {
					preview = true
					if r.URL.Query().Get("by") != by {
						t.Errorf("preview mode %s", r.URL.RawQuery)
					}
					json.NewEncoder(w).Encode(map[string]any{"category": "land"})
					return
				}
				posted = true
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				if body["by"] != by {
					t.Errorf("dispatch by=%v", body["by"])
				}
				json.NewEncoder(w).Encode(map[string]any{})
			}))
			defer srv.Close()
			oldCfg, oldJSON := cfg, jsonMode
			defer func() { cfg, jsonMode = oldCfg, oldJSON }()
			cfg = &Config{Server: srv.URL, Token: "token", WorldID: "world", ProvinceID: "source"}
			jsonMode = true
			cmd := transferCmd()
			cmd.SetArgs([]string{"--by", by, "--dest", dest, "--good", "grain", "--qty", "1"})
			if _, err := captureStdout(t, cmd.Execute); err != nil {
				t.Fatal(err)
			}
			if !preview || !posted {
				t.Fatal("missing preview or dispatch")
			}
		})
	}
}

func TestRouteAndTradeOfferPersistSelectedMode(t *testing.T) {
	for _, verb := range []string{"route", "trade-offer"} {
		t.Run(verb, func(t *testing.T) {
			posted := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if testWorldTimeResponse(w, r) {
					return
				}
				switch {
				case strings.HasSuffix(r.URL.Path, "/provinces"):
					json.NewEncoder(w).Encode([]map[string]any{{"id": "source", "settlement_id": "home", "name": "Nostos", "own": true, "is_capital": true}, {"id": "target", "settlement_id": "dest", "name": "Petras", "knowledge": "known"}})
				case strings.HasSuffix(r.URL.Path, "/wanaxes"):
					json.NewEncoder(w).Encode([]any{})
				case strings.HasSuffix(r.URL.Path, "/transport-options"):
					if r.URL.Query().Get("destination_id") != "dest" {
						t.Errorf("wrong destination %s", r.URL.RawQuery)
					}
					json.NewEncoder(w).Encode(map[string]any{"options": []map[string]any{{"by": "land", "available": true, "travel_ticks": 3, "travel_seconds": 180, "risk": "Interception"}, {"by": "sea", "available": false, "reason": "No free ship"}}})
				case r.Method == "POST":
					posted = true
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					if verb == "trade-offer" {
						body = body["trade_offer"].(map[string]any)
					}
					if body["by"] != "land" {
						t.Errorf("wrong by %v", body)
					}
					json.NewEncoder(w).Encode(map[string]any{"id": "new", "arrives_at": "2026-10-28T12:00:00Z"})
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
				}
			}))
			defer srv.Close()
			oldCfg, oldJSON := cfg, jsonMode
			defer func() { cfg, jsonMode = oldCfg, oldJSON }()
			cfg = &Config{Server: srv.URL, Token: "token", WorldID: "world", ProvinceID: "source"}
			jsonMode = false
			cmd := routeCreateCmd()
			args := []string{"--by", "land", "--to", "Petras", "--out", "grain:200"}
			if verb == "trade-offer" {
				cmd = tradeOfferCmd()
				args = []string{"--by", "land", "--to", "Petras", "--want-good", "grain", "--want-qty", "2", "--offer-silver", "3"}
			}
			cmd.SetArgs(args)
			out, err := captureStdout(t, cmd.Execute)
			if err != nil {
				t.Fatal(err)
			}
			if !posted || !strings.Contains(out, "No free ship") || !strings.Contains(out, "3 game days") {
				t.Fatalf("posted=%v output=%s", posted, out)
			}
		})
	}
}
