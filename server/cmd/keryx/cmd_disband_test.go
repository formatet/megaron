package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDisbandWholeUnitNameAndReceipt(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/units") {
			w.Write([]byte(`{"units":[{"id":"` + id + `","display_name":"1st Spearmen of Phaistos"}]}`))
			return
		}
		if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/provinces/province/disband") {
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			http.Error(w, "bad", 400)
			return
		}
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		ids, ok := req["unit_ids"].([]any)
		if !ok || len(req) != 1 || len(ids) != 1 || ids[0] != id {
			t.Errorf("not whole-unit request: %v", req)
		}
		posts++
		w.Write([]byte(`{"disbanded":[{"id":"` + id + `","display_name":"1st Spearmen of Phaistos","people_returned":41}],"pop_restored":41,"population":1041,"settlement_name":"Phaistos"}`))
	}))
	defer srv.Close()
	old, mode := cfg, jsonMode
	defer func() { cfg, jsonMode = old, mode }()
	cfg = &Config{Server: srv.URL, WorldID: "world", ProvinceID: "province"}
	jsonMode = false
	cmd := disbandCmd()
	cmd.SetArgs([]string{"--unit", "1st Spearmen of Phaistos"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil || posts != 1 || !strings.Contains(out, "41 people return to Phaistos.") || !strings.Contains(out, "1st Spearmen") {
		t.Fatalf("%v %s posts%d", err, out, posts)
	}
	for _, args := range [][]string{nil, {"--spearman", "20"}} {
		cmd = disbandCmd()
		cmd.SetArgs(args)
		if _, err = captureStdout(t, cmd.Execute); err == nil {
			t.Fatal("quantity/missing unit accepted")
		}
	}
	if posts != 1 {
		t.Fatal("invalid request sent")
	}
}
func TestDisbandAmbiguousNameNeverDispatches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Fatal("ambiguous dispatch")
		}
		w.Write([]byte(`{"units":[{"id":"aa","name":"Asterion"},{"id":"bb","name":"Asterion"}]}`))
	}))
	defer srv.Close()
	old := cfg
	defer func() { cfg = old }()
	cfg = &Config{Server: srv.URL, WorldID: "world"}
	if _, err := disbandUnitIDs(newClient(cfg), []string{"Asterion"}); err == nil || !strings.Contains(err.Error(), "matches 2") {
		t.Fatal(err)
	}
}

func TestDisbandReceiptNamesEachUnitBeforePeopleAndCity(t *testing.T) {
	got := formatDisbandResult(map[string]any{"settlement_name": "Phaistos", "pop_restored": float64(42), "disbanded": []any{
		map[string]any{"display_name": "2nd Spearmen of Phaistos", "people_returned": float64(41)},
		map[string]any{"display_name": "Asterion", "people_returned": float64(1)},
	}})
	want := "2nd Spearmen of Phaistos disbanded — 41 people return to Phaistos.\nAsterion disbanded — 1 person returns to Phaistos."
	if got != want {
		t.Fatalf("%q want %q", got, want)
	}
}
