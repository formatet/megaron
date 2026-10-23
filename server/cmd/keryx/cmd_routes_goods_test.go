package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutesListsConfiguredStocksAndReturnFloors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/worlds/world/standing-orders" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected route request %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"route","from_name":"Source","to_name":"Colony","status":"active","outbound":[{"good_key":"grain","threshold":200},{"good_key":"fish","threshold":50.5}],"return":[{"good_key":"silver","floor":0},{"good_key":"stone","floor":20}]},{"id":"empty","from_name":"Source","to_name":"Other","status":"paused","outbound":[{"good_key":"grain","threshold":0}],"return":[]}]`))
	}))
	defer ts.Close()
	oldCfg, oldJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = oldCfg, oldJSON }()
	cfg = &Config{Server: ts.URL, Token: "test-token", WorldID: "world"}
	jsonMode = false
	out, err := captureStdout(t, func() error { cmd := routesCmd(); return cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Keep at destination: grain 200, fish 50.5", "Bring home above: silver 0, stone 20", "Keep at destination: grain 0", "Return cargo: none"} {
		if !strings.Contains(out, want) {
			t.Errorf("routes missing %q: %s", want, out)
		}
	}
	jsonMode = true
	raw, err := captureStdout(t, func() error { cmd := routesCmd(); return cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	var orders []map[string]any
	if err := json.Unmarshal([]byte(raw), &orders); err != nil {
		t.Fatal(err)
	}
	if len(orders) != 2 || len(orders[0]["outbound"].([]any)) != 2 || orders[0]["return"].([]any)[0].(map[string]any)["floor"] != float64(0) {
		t.Fatalf("JSON contract changed: %s", raw)
	}

}
