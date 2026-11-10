package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const dupDest = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

// runDupTransfer runs `keryx transfer` against a server whose own caravans in
// transit are `trades`, returning POST count, output and error.
func runDupTransfer(t *testing.T, trades []map[string]any, args ...string) (int, string, error) {
	t.Helper()
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case testWorldTimeResponse(w, r):
		case strings.HasSuffix(r.URL.Path, "/trade/destinations"):
			json.NewEncoder(w).Encode([]map[string]any{{"settlement_id": dupDest, "name": "Korinth"}})
		case strings.HasSuffix(r.URL.Path, "/provinces"):
			json.NewEncoder(w).Encode([]map[string]any{{"id": "source", "name": "Myrine", "own": true}})
		case strings.HasSuffix(r.URL.Path, "/trades"):
			json.NewEncoder(w).Encode(trades)
		case strings.HasSuffix(r.URL.Path, "/preview"):
			json.NewEncoder(w).Encode(map[string]any{"category": "land"})
		case r.Method == "POST":
			posts++
			json.NewEncoder(w).Encode(map[string]any{"kind": "transfer", "arrival_tick": 42, "travel_ticks": 7})
		}
	}))
	defer srv.Close()
	old, mode := cfg, jsonMode
	defer func() { cfg, jsonMode = old, mode }()
	cfg = &Config{Server: srv.URL, WorldID: "world", ProvinceID: "source"}
	jsonMode = false
	cmd := transferCmd()
	cmd.SetArgs(append([]string{"--dest", "Korinth"}, args...))
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	out, err := captureStdout(t, cmd.Execute)
	return posts, out, err
}

func dupTrade(tick int, dest string, items ...[2]any) map[string]any {
	var list []map[string]any
	for _, it := range items {
		list = append(list, map[string]any{"good_key": it[0], "quantity": it[1]})
	}
	return map[string]any{"role": "sender", "kind": "transfer", "origin_name": "Myrine", "dest_name": dest,
		"departure_tick": tick, "arrival_tick": 42, "items": list}
}

func TestTransferDuplicateSameTickRefused(t *testing.T) {
	trades := []map[string]any{dupTrade(0, "Korinth", [2]any{"grain", 50.0})}
	posts, _, err := runDupTransfer(t, trades, "--good", "grain", "--qty", "50")
	if err == nil || posts != 0 || !strings.Contains(err.Error(), "--again") || !strings.Contains(err.Error(), "identical caravan") {
		t.Fatalf("posts %d err %v", posts, err)
	}
}

func TestTransferDuplicateAgainFlagSends(t *testing.T) {
	trades := []map[string]any{dupTrade(0, "Korinth", [2]any{"grain", 50.0})}
	posts, _, err := runDupTransfer(t, trades, "--good", "grain", "--qty", "50", "--again")
	if err != nil || posts != 1 {
		t.Fatalf("posts %d err %v", posts, err)
	}
}

func TestTransferNotIdenticalSends(t *testing.T) {
	for name, tc := range map[string]struct {
		trade map[string]any
		args  []string
	}{
		"qty":     {dupTrade(0, "Korinth", [2]any{"grain", 40.0}), []string{"--good", "grain", "--qty", "50"}},
		"good":    {dupTrade(0, "Korinth", [2]any{"timber", 50.0}), []string{"--good", "grain", "--qty", "50"}},
		"dest":    {dupTrade(0, "Athenai", [2]any{"grain", 50.0}), []string{"--good", "grain", "--qty", "50"}},
		"earlier": {dupTrade(-1, "Korinth", [2]any{"grain", 50.0}), []string{"--good", "grain", "--qty", "50"}},
		"extra":   {dupTrade(0, "Korinth", [2]any{"grain", 50.0}, [2]any{"timber", 5.0}), []string{"--good", "grain", "--qty", "50"}},
	} {
		posts, _, err := runDupTransfer(t, []map[string]any{tc.trade}, tc.args...)
		if err != nil || posts != 1 {
			t.Fatalf("%s: posts %d err %v", name, posts, err)
		}
	}
}

func TestTransferDuplicateItemOrderIrrelevant(t *testing.T) {
	trades := []map[string]any{dupTrade(0, "Korinth", [2]any{"timber", 8.0}, [2]any{"grain", 12.0})}
	posts, _, err := runDupTransfer(t, trades, "--good", "grain", "--qty", "12", "--good", "timber", "--qty", "8")
	if err == nil || posts != 0 {
		t.Fatalf("posts %d err %v", posts, err)
	}
}
